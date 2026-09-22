//go:build ignore

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	configPath := flag.String("config", "config.json", "Konfigurasi DMS lokal")
	flag.Parse()
	b, err := os.ReadFile(*configPath)
	if err != nil {
		log.Fatal("Gagal membaca konfigurasi: ", err)
	}

	var c struct {
		Host     string `json:"mysql_host"`
		User     string `json:"mysql_user"`
		Password string `json:"mysql_password"`
		DB       string `json:"database"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		log.Fatal("Konfigurasi tidak valid: ", err)
	}

	if c.DB != "lspfi_dms" {
		log.Fatal("Database harus lspfi_dms; reset tidak boleh menyentuh database sumber.")
	}
	password := os.Getenv("LSPFI_ADMIN_PASSWORD")
	if len(password) < 12 || len(password) > 72 || strings.TrimSpace(password) == "" {
		log.Fatal("Isi LSPFI_ADMIN_PASSWORD dengan password 12–72 byte.")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.DBName = c.User, c.Password, "tcp", c.Host, c.DB
	cfg.Timeout, cfg.ReadTimeout, cfg.WriteTimeout = 5*time.Second, 10*time.Second, 10*time.Second
	dsn := cfg.FormatDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatal("Gagal konek ke database: ", err)
	}
	defer db.Close()

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	res, err := db.ExecContext(ctx, "UPDATE users SET password_hash = ? WHERE username = 'admin' AND role = 'admin'", string(hash))
	if err != nil {
		log.Fatal("Gagal memperbarui pengguna: ", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		log.Fatal(err)
	}
	if rows > 0 {
		fmt.Println("SUKSES: Password admin berhasil diubah.")
	} else {
		log.Fatal("Akun admin dengan role admin tidak ditemukan.")
	}
}
