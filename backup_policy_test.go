package main

import (
	"archive/zip"
	"context"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReferenceBackupMySQL(t *testing.T) {
	env := os.Getenv("LSPFI_TEST_SOURCE_ENV")
	if env == "" {
		t.Skip("set LSPFI_TEST_SOURCE_ENV for isolated MySQL test")
	}
	c, _, e := sourceConfig(env)
	if e != nil {
		t.Fatal(e)
	}
	root, e := connect(c, "")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	source := legacySourceDB(t, root, c)
	queries := []string{
		"CREATE TABLE user (id INT PRIMARY KEY, name VARCHAR(100), role VARCHAR(30)) ENGINE=InnoDB",
		"INSERT INTO user VALUES (1,'PENGELOLA','admin'),(2,'ASESI-RAHASIA','asesi'),(3,'ASESOR','asesor'),(4,'PROFIL-RAHASIA','admin'),(5,'ROLE-BARU-RAHASIA','unknown')",
		"CREATE TABLE asesiprofile (id INT PRIMARY KEY, user_id INT, nik VARCHAR(30), FOREIGN KEY (user_id) REFERENCES user(id)) ENGINE=InnoDB",
		"INSERT INTO asesiprofile VALUES (1,2,'NIK-RAHASIA'),(2,4,'NIK-PROFIL-RAHASIA')",
		"CREATE TABLE skema (id INT PRIMARY KEY, nama VARCHAR(100)) ENGINE=InnoDB",
		"INSERT INTO skema VALUES (1,'SKEMA TETAP')",
		"CREATE TABLE banksoal (id INT PRIMARY KEY, pertanyaan TEXT, created_by_user_id INT NULL) ENGINE=InnoDB",
		"INSERT INTO banksoal VALUES (1,'SOAL TETAP',1),(2,'SOAL LAIN',2)",
		"CREATE TABLE activitylog (id INT PRIMARY KEY, description TEXT) ENGINE=InnoDB",
		"INSERT INTO activitylog VALUES (1,'NIK-RAHASIA')",
		"CREATE TABLE unexpected (id INT PRIMARY KEY, content TEXT) ENGINE=InnoDB",
		"INSERT INTO unexpected VALUES (1,'DATA-BARU-RAHASIA')",
	}
	for _, q := range queries {
		if _, e = source.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	target := testApp(t, root, c)
	if _, e = target.importRegisterWeb(context.Background(), source); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(filepath.Join(target.cfg.Storage, "imports", strings.Repeat("a", 32)+".csv"), []byte("NIK-RAHASIA"), 0600); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "reference.zip")
	if e = target.writeBackup(path); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(path)
	if e != nil {
		t.Fatal(e)
	}
	defer z.Close()
	if len(z.File) != 1 || z.File[0].Name != "manifest.json" {
		t.Fatal("private files included")
	}
	r, e := z.File[0].Open()
	if e != nil {
		t.Fatal(e)
	}
	var manifest Backup
	e = json.NewDecoder(r).Decode(&manifest)
	r.Close()
	if e != nil {
		t.Fatal(e)
	}
	if e = validateReferenceBackup(manifest); e != nil {
		t.Fatal(e)
	}
	for name, rows := range manifest.RegisterWeb.Rows {
		for _, row := range rows {
			for _, value := range row {
				if value != nil {
					plain, _ := hex.DecodeString(*value)
					if strings.Contains(string(plain), "RAHASIA") {
						t.Fatalf("private information leaked in %s", name)
					}
				}
			}
		}
	}
	if len(manifest.RegisterWeb.Rows["user"]) != 2 || len(manifest.RegisterWeb.Rows["skema"]) != 1 || len(manifest.RegisterWeb.Rows["banksoal"]) != 2 {
		t.Fatal("other data lost")
	}
	restored := testApp(t, root, c)
	if e = restored.restore(path); e != nil {
		t.Fatal(e)
	}
	for name, want := range map[string]int{"user": 2, "skema": 1, "banksoal": 2, "asesiprofile": 0, "activitylog": 0, "unexpected": 0} {
		var count int
		if e = restored.db.QueryRow("SELECT COUNT(*) FROM " + quoteID("rw_"+name)).Scan(&count); e != nil || count != want {
			t.Fatalf("restore count %s: %d %v", name, count, e)
		}
	}
	var count int
	if e = target.db.QueryRow("SELECT COUNT(*) FROM rw_asesiprofile").Scan(&count); e != nil || count != 2 {
		t.Fatal("live candidate data changed")
	}
	if e = restored.db.QueryRow("SELECT COUNT(*) FROM rw_banksoal WHERE created_by_user_id IS NULL").Scan(&count); e != nil || count != 1 {
		t.Fatal("excluded creator ID retained")
	}
	// A forged v2 file must not be allowed to reintroduce excluded records.
	manifest.RegisterWeb.Rows["asesiprofile"] = [][]*string{{nil, nil, nil}}
	if e = validateReferenceBackup(manifest); e == nil {
		t.Fatal("invalid reference backup accepted")
	}
}
