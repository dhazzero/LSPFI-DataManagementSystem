package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func legacySourceDB(t *testing.T, root *sql.DB, c Config) *sql.DB {
	t.Helper()
	name := "lspfi_dms_test_rw_" + token()[:8]
	if _, e := root.Exec("CREATE DATABASE " + quoteID(name) + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
		t.Fatal(e)
	}
	db, e := connect(c, name)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Close()
		if _, e := root.Exec("DROP DATABASE " + quoteID(name)); e != nil {
			t.Error(e)
		}
	})
	return db
}
func TestRegisterWebSnapshotMySQL(t *testing.T) {
	env := os.Getenv("LSPFI_TEST_SOURCE_ENV")
	if env == "" {
		t.Skip("set LSPFI_TEST_SOURCE_ENV to test MySQL migration")
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
	stmts := []string{
		"CREATE TABLE parent (id INT PRIMARY KEY AUTO_INCREMENT,nik CHAR(16) NOT NULL UNIQUE,password VARCHAR(100) NOT NULL,settings JSON NULL,amount DECIMAL(20,4) NOT NULL,raw_bytes VARBINARY(20) NULL) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
		"CREATE TABLE child (id INT PRIMARY KEY AUTO_INCREMENT,parent_id INT NOT NULL,result ENUM('K','BK') NOT NULL,notes TEXT NULL,at_time DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),CONSTRAINT child_parent FOREIGN KEY(parent_id) REFERENCES parent(id) ON DELETE CASCADE ON UPDATE CASCADE) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
		"CREATE TABLE empty_table (id INT PRIMARY KEY) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci",
	}
	for _, q := range stmts {
		if _, e = source.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	if _, e = source.Exec("INSERT INTO parent(id,nik,password,settings,amount,raw_bytes) VALUES(7,'0000000000000001','must-not-leak',?,12345678901234.1234,?)", `{"escaped":"a\\b","list":[1,null]}`, []byte{0, 255, 13, 65}); e != nil {
		t.Fatal(e)
	}
	if _, e = source.Exec("INSERT INTO child(parent_id,result,notes,at_time) VALUES(7,'K',NULL,'2026-09-19 01:02:03.456'),(7,'BK','','2026-09-19 01:02:03.001')"); e != nil {
		t.Fatal(e)
	}
	conn, e := source.Conn(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(context.Background(), "SET SESSION FOREIGN_KEY_CHECKS=0"); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(context.Background(), "INSERT INTO child(parent_id,result) VALUES(999,'BK')"); e != nil {
		t.Fatal(e)
	}
	if _, e = conn.ExecContext(context.Background(), "SET SESSION FOREIGN_KEY_CHECKS=1"); e != nil {
		t.Fatal(e)
	}
	conn.Close()
	before, e := loadLegacySnapshot(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	target := testApp(t, root, c)
	if e = createLegacyTables(context.Background(), target.db, before.Catalog); e != nil {
		t.Fatal(e)
	}
	if e = createLegacyTables(context.Background(), target.db, before.Catalog); e != nil {
		t.Fatalf("retry of an empty migration failed: %v", e)
	}
	catalog, e := target.importRegisterWeb(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	if len(catalog.Tables) != 3 {
		t.Fatal("missing empty table")
	}
	if len(catalog.Orphans) != 1 || catalog.Orphans[0].Count != 1 {
		t.Fatalf("source orphan was not recorded: %+v", catalog.Orphans)
	}
	for _, table := range catalog.Tables {
		rows, e := readLegacyRows(context.Background(), target.db, table, table.Target)
		if e != nil {
			t.Fatal(e)
		}
		if legacyDigest(rows) != table.Digest {
			t.Fatal("data changed during migration")
		}
	}
	if _, e = target.importRegisterWeb(context.Background(), source); e == nil {
		t.Fatal("repeated import overwrote snapshot")
	}
	if _, e = target.db.Exec("INSERT INTO rw_child(parent_id,result) VALUES(999,'K')"); e == nil {
		t.Fatal("foreign key was lost")
	}
	table, e := target.legacyTable("parent")
	if e != nil {
		t.Fatal(e)
	}
	values, e := target.legacyRows(table, " WHERE 1=1", nil, 50, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(values)
	if strings.Contains(string(b), "must-not-leak") || *values[0]["password"] != "[dilindungi]" {
		t.Fatal("password hash exposed")
	}
	if _, _, e = legacyWhere(table, "", "password", "must-not-leak"); e == nil {
		t.Fatal("password search allowed")
	}
	backup := filepath.Join(t.TempDir(), "all-tables.zip")
	if e = target.writeBackup(backup); e != nil {
		t.Fatal(e)
	}
	restored := testApp(t, root, c)
	if e = restored.restore(backup); e != nil {
		t.Fatal(e)
	}
	restoredCatalog, e := restored.legacyCatalog()
	if e != nil || len(restoredCatalog.Tables) != 3 {
		t.Fatalf("backup omitted RegisterWeb: %v", e)
	}
	for _, table := range restoredCatalog.Tables {
		rows, e := readLegacyRows(context.Background(), restored.db, table, table.Target)
		if e != nil || legacyDigest(rows) != table.Digest {
			t.Fatalf("restored data mismatch: %s %v", table.Name, e)
		}
	}
	if e = restored.restore(backup); e == nil {
		t.Fatal("restore overwrote snapshot")
	}
	after, e := loadLegacySnapshot(context.Background(), source)
	if e != nil {
		t.Fatal(e)
	}
	for i, tb := range before.Catalog.Tables {
		if tb.Digest != after.Catalog.Tables[i].Digest || tb.DDL != after.Catalog.Tables[i].DDL {
			t.Fatal("source changed")
		}
	}
	malicious := before
	malicious.Catalog.Tables = append([]LegacyTable(nil), before.Catalog.Tables...)
	malicious.Catalog.Tables[0].DDL = strings.Replace(malicious.Catalog.Tables[0].DDL, "REFERENCES `parent` (", "REFERENCES `parent`.`user` (", 1)
	if e = validateLegacySnapshot(malicious); e == nil {
		t.Fatal("cross-schema foreign key accepted")
	}
}

func TestActualRegisterWebSchemaCompatibility(t *testing.T) {
	b, e := os.ReadFile("docs/registerweb-schema.json")
	if os.IsNotExist(e) {
		t.Skip("local inventory not available")
	}
	if e != nil {
		t.Fatal(e)
	}
	var c LegacyCatalog
	if e = json.Unmarshal(b, &c); e != nil {
		t.Fatal(e)
	}
	for _, table := range c.Tables {
		ddl, e := mappedLegacyDDL(table, c)
		if e != nil {
			t.Fatal(e)
		}
		if !strings.HasPrefix(ddl, "CREATE TABLE "+quoteID(table.Target)) {
			t.Fatal("wrong target")
		}
	}
	if _, e = legacyOrder(c); e != nil {
		t.Fatal(e)
	}
	if env := os.Getenv("LSPFI_TEST_SOURCE_ENV"); env != "" {
		cfg, _, e := sourceConfig(env)
		if e != nil {
			t.Fatal(e)
		}
		root, e := connect(cfg, "")
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { root.Close() })
		target := testApp(t, root, cfg)
		if e = createLegacyTables(context.Background(), target.db, c); e != nil {
			t.Fatal(e)
		}
		if e = createLegacyTables(context.Background(), target.db, c); e != nil {
			t.Fatalf("actual schema retry failed: %v", e)
		}
	}
}
