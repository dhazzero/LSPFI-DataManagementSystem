package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

//go:embed web/* schema.sql
var assets embed.FS

const databaseName = "lspfi_dms"

type Config struct {
	Address  string `json:"address"`
	Host     string `json:"mysql_host"`
	User     string `json:"mysql_user"`
	Password string `json:"mysql_password"`
	Database string `json:"database"`
	Storage  string `json:"storage_dir"`
}

type App struct {
	db       *sql.DB
	cfg      Config
	mu       sync.Mutex
	sessions map[string]Session
	previews map[string]Preview
	attempts map[string][]time.Time
	writes   sync.Mutex
}

func token() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func readConfig(path string) (Config, error) {
	c := Config{Address: "127.0.0.1:4080", Host: "127.0.0.1:3306", Database: databaseName, Storage: "data"}
	b, e := os.ReadFile(path)
	if e != nil {
		return c, e
	}
	e = json.Unmarshal(b, &c)
	if c.Database != databaseName {
		return c, errors.New("database harus lspfi_dms; database proyek lama tidak boleh digunakan")
	}
	host, _, err := net.SplitHostPort(c.Address)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		return c, errors.New("versi lokal hanya menerima address loopback (127.0.0.1)")
	}
	if !filepath.IsAbs(c.Storage) {
		c.Storage = filepath.Join(filepath.Dir(path), c.Storage)
	}
	return c, e
}

func connect(c Config, dbName string) (*sql.DB, error) {
	d := mysql.NewConfig()
	d.User = c.User
	d.Passwd = c.Password
	d.Net = "tcp"
	d.Addr = c.Host
	d.DBName = dbName
	d.ParseTime = true
	d.Timeout = 5 * time.Second
	d.ReadTimeout = 30 * time.Second
	d.WriteTimeout = 30 * time.Second
	db, e := sql.Open("mysql", d.FormatDSN())
	if e != nil {
		return nil, e
	}
	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)
	db.SetConnMaxLifetime(3 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	if e = db.PingContext(ctx); e != nil {
		db.Close()
		return nil, fmt.Errorf("MySQL tidak terhubung: %w", e)
	}
	return db, nil
}

func migrate(db *sql.DB) error {
	b, _ := assets.ReadFile("schema.sql")
	for _, stmt := range strings.Split(string(b), ";") {
		if strings.TrimSpace(stmt) != "" {
			if _, e := db.Exec(stmt); e != nil {
				return e
			}
		}
	}
	return nil
}

func sourceConfig(path string) (Config, string, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return Config{}, "", e
	}
	var raw string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "DATABASE_URL=") {
			raw = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "DATABASE_URL=")), "\"'")
			break
		}
	}
	u, e := url.Parse(raw)
	if e != nil || u == nil || u.Scheme != "mysql" || u.User == nil {
		return Config{}, "", errors.New("DATABASE_URL MySQL tidak ditemukan")
	}
	p, _ := u.User.Password()
	host := u.Host
	if u.Port() == "" {
		host = net.JoinHostPort(u.Hostname(), "3306")
	}
	source := strings.TrimPrefix(u.Path, "/")
	if source == databaseName || source == "" {
		return Config{}, "", errors.New("database sumber harus berbeda dengan lspfi_dms")
	}
	return Config{Address: "127.0.0.1:4080", Host: host, User: u.User.Username(), Password: p, Database: databaseName, Storage: "data"}, source, nil
}

func initialize(configPath, sourceEnv string) error {
	var c Config
	var source string
	var e error
	if sourceEnv != "" {
		if _, err := os.Stat(configPath); !os.IsNotExist(err) {
			return errors.New("config sudah ada; inisialisasi tidak menimpa konfigurasi")
		}
		c, source, e = sourceConfig(sourceEnv)
		if e != nil {
			return e
		}
	} else {
		c, e = readConfig(configPath)
		if e != nil {
			return e
		}
	}
	admin, e := connect(c, "")
	if e != nil {
		return e
	}
	defer admin.Close()
	if _, e = admin.Exec("CREATE DATABASE IF NOT EXISTS lspfi_dms CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
		return e
	}
	db, e := connect(c, databaseName)
	if e != nil {
		return e
	}
	defer db.Close()
	if e = migrate(db); e != nil {
		return e
	}
	var count int
	if e = db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return errors.New("database sudah memiliki pengguna; inisialisasi dihentikan")
	}
	tx, e := db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if source != "" {
		old, e := connect(c, source)
		if e != nil {
			return e
		}
		defer old.Close()
		rows, e := old.Query("SELECT kategori,kode,label,COALESCE(parent_kode,'') FROM ParameterBnsp")
		if e != nil {
			return e
		}
		for rows.Next() {
			var m Master
			if e = rows.Scan(&m.Category, &m.Code, &m.Label, &m.Parent); e != nil {
				rows.Close()
				return e
			}
			if _, e = tx.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?) ON DUPLICATE KEY UPDATE label=VALUES(label),parent_code=VALUES(parent_code)", m.Category, m.Code, m.Label, m.Parent); e != nil {
				rows.Close()
				return e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		rows, e = old.Query("SELECT id,nama_skema,jenjang FROM Skema")
		if e != nil {
			return e
		}
		for rows.Next() {
			var id, level int
			var name string
			if e = rows.Scan(&id, &name, &level); e != nil {
				rows.Close()
				return e
			}
			if _, e = tx.Exec("INSERT INTO master(category,code,label) VALUES('SKEMA',?,?) ON DUPLICATE KEY UPDATE label=VALUES(label)", fmt.Sprint(id), fmt.Sprintf("%d %s", level, name)); e != nil {
				rows.Close()
				return e
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
	}
	password := token()[:20]
	hash, e := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if e != nil {
		return e
	}
	if _, e = tx.Exec("INSERT INTO users(username,password_hash,role) VALUES('admin',?,'admin')", string(hash)); e != nil {
		return e
	}
	if sourceEnv != "" {
		// Give the running application access exclusively to its own database.
		appPassword := token()
		appUser := "lspfi_dms_" + token()[:8]
		// CREATE USER does not support placeholders on all MySQL versions.
		// Both interpolated values are generated hexadecimal tokens, never user input.
		if _, e = admin.Exec("CREATE USER '" + appUser + "'@'localhost' IDENTIFIED BY '" + appPassword + "'"); e != nil {
			return fmt.Errorf("buat akun aplikasi: %w", e)
		}
		if _, e = admin.Exec("GRANT SELECT,INSERT,UPDATE,DELETE,CREATE,INDEX,REFERENCES,ALTER ON lspfi_dms.* TO '" + appUser + "'@'localhost'"); e != nil {
			return e
		}
		c.User = appUser
		c.Password = appPassword
		b, _ := json.MarshalIndent(c, "", "  ")
		if e = os.WriteFile(configPath, b, 0600); e != nil {
			return e
		}
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	if e = os.MkdirAll(".local", 0700); e != nil {
		return e
	}
	if e = os.WriteFile(".local/LOGIN-AWAL.txt", []byte("LSPFI Arsip Digital\nAlamat: http://"+c.Address+"\nPengguna: admin\nKata sandi: "+password+"\nGanti kata sandi dari menu Pengaturan setelah login.\n"), 0600); e != nil {
		return e
	}
	fmt.Println("Database lspfi_dms siap. Kredensial awal tersimpan di .local/LOGIN-AWAL.txt. Database sumber hanya dibaca.")
	return nil
}

func main() {
	config := flag.String("config", "config.json", "Konfigurasi lokal")
	initDB := flag.Bool("init", false, "Buat database aplikasi")
	source := flag.String("source-env", "", "Salin master dari DATABASE_URL proyek lama (read-only)")
	restore := flag.String("restore", "", "Pulihkan backup ZIP ke database arsip kosong")
	inspect := flag.Bool("inspect-registerweb", false, "Inventaris seluruh tabel MySQL sumber tanpa mengubahnya")
	importLegacy := flag.Bool("import-registerweb", false, "Salin seluruh struktur dan data tabel RegisterWeb ke tabel rw_ lokal")
	flag.Parse()
	if *inspect {
		c, _, e := sourceConfig(*source)
		if e != nil {
			log.Fatal(e)
		}
		_, sourceDB, _ := sourceConfig(*source)
		db, e := connect(c, sourceDB)
		if e != nil {
			log.Fatal(e)
		}
		defer db.Close()
		catalog, e := inspectLegacy(context.Background(), db)
		if e != nil {
			log.Fatal(e)
		}
		if e = writeLegacyDocs(catalog); e != nil {
			log.Fatal(e)
		}
		for _, table := range catalog.Tables {
			fmt.Printf("%s: %d baris, %d kolom, %d relasi\n", table.Name, table.Count, len(table.Columns), len(table.Relations))
		}
		return
	}
	if *initDB {
		if e := initialize(*config, *source); e != nil {
			log.Fatal(e)
		}
		return
	}
	c, e := readConfig(*config)
	if e != nil {
		log.Fatal(e)
	}
	db, e := connect(c, c.Database)
	if e != nil {
		log.Fatal(e)
	}
	defer db.Close()
	if e = migrate(db); e != nil {
		log.Fatal(e)
	}
	for _, folder := range []string{"documents", "imports"} {
		if e = os.MkdirAll(filepath.Join(c.Storage, folder), 0700); e != nil {
			log.Fatal(e)
		}
	}
	a := &App{db: db, cfg: c, sessions: map[string]Session{}, previews: map[string]Preview{}, attempts: map[string][]time.Time{}}
	if *importLegacy {
		sourceCfg, sourceDB, e := sourceConfig(*source)
		if e != nil {
			log.Fatal(e)
		}
		old, e := connect(sourceCfg, sourceDB)
		if e != nil {
			log.Fatal(e)
		}
		defer old.Close()
		backupPath := filepath.Join(c.Storage, "before-registerweb-"+time.Now().Format("20060102-150405")+".zip")
		if e = a.writeBackup(backupPath); e != nil {
			log.Fatal(e)
		}
		catalog, e := a.importRegisterWeb(context.Background(), old)
		if e != nil {
			log.Fatal(e)
		}
		if e = writeLegacyDocs(catalog); e != nil {
			log.Fatal(e)
		}
		for _, table := range catalog.Tables {
			fmt.Printf("%s → %s: %d baris terverifikasi\n", table.Name, table.Target, table.Count)
		}
		return
	}
	if *restore != "" {
		if e = a.restore(*restore); e != nil {
			log.Fatal(e)
		}
		fmt.Println("Pemulihan selesai.")
		return
	}
	server := &http.Server{Addr: c.Address, Handler: a.routes(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 5 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		server.Shutdown(shutdown)
	}()
	fmt.Printf("LSPFI Arsip Digital siap: http://%s\n", c.Address)
	if e = server.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		log.Fatal(e)
	}
}
