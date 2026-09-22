package main

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestMasterValidation(t *testing.T) {
	m := Master{Category: " PENDIDIKAN ", Code: " 04 ", Label: strings.Repeat("é", 255)}
	if err := normalizeMaster(&m); err != nil || m.Code != "04" {
		t.Fatalf("valid Unicode reference: %v", err)
	}
	for _, m := range []Master{
		{Category: "PENDIDIKAN", Code: " ", Label: "S1"},
		{Category: "INVALID", Code: "1", Label: "S1"},
		{Category: "KABUPATEN", Code: "3171", Label: "Kota"},
		{Category: "PROVINSI", Code: "31", Label: "Jakarta", Parent: "31"},
		{ID: -1, Category: "PENDIDIKAN", Code: "1", Label: "S1"},
	} {
		if normalizeMaster(&m) == nil {
			t.Fatalf("invalid accepted: %+v", m)
		}
	}
}

func TestMySQLMasterLifecycle(t *testing.T) {
	source := os.Getenv("LSPFI_TEST_SOURCE_ENV")
	if source == "" {
		t.Skip("set LSPFI_TEST_SOURCE_ENV for isolated MySQL integration test")
	}
	c, _, err := sourceConfig(source)
	if err != nil {
		t.Fatal(err)
	}
	root, err := connect(c, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	a := testApp(t, root, c)
	for _, q := range []string{
		"CREATE TABLE rw_parameterbnsp(id INT AUTO_INCREMENT PRIMARY KEY,kategori VARCHAR(191),kode VARCHAR(191),label VARCHAR(191),parent_kode VARCHAR(191),createdAt DATETIME,updatedAt DATETIME)",
		"CREATE TABLE rw_skema(id INT PRIMARY KEY,kode_skema VARCHAR(191),nama_skema VARCHAR(191),updatedAt DATETIME)",
		"INSERT INTO rw_skema VALUES(24,'FIN-24','Skema lama',NOW())",
	} {
		if _, err = a.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	save := func(m Master) Master {
		t.Helper()
		id, e := a.storeMaster(m, "test-admin")
		if e != nil {
			t.Fatal(e)
		}
		m.ID = id
		return m
	}
	wantStatus := func(e error, status int) {
		t.Helper()
		var p *masterError
		if !errors.As(e, &p) || p.status != status {
			t.Fatalf("wanted %d, got %v", status, e)
		}
	}
	education := save(Master{Category: "PENDIDIKAN", Code: "04", Label: "S1"})
	// Repeated saves must not insert duplicate legacy parameters even when UPDATE changes no value.
	save(education)
	save(education)
	var count int
	if err = a.db.QueryRow("SELECT COUNT(*) FROM rw_parameterbnsp WHERE kategori='PENDIDIKAN' AND kode='04'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("mirror duplicates: %d, %v", count, err)
	}
	_, err = a.storeMaster(Master{Category: "PENDIDIKAN", Code: "04", Label: "Overwrite"}, "test")
	wantStatus(err, 409)
	education.Label = "Sarjana"
	education = save(education)
	var label string
	if err = a.db.QueryRow("SELECT label FROM rw_parameterbnsp WHERE kode='04'").Scan(&label); err != nil || label != "Sarjana" {
		t.Fatalf("label not synchronized: %s %v", label, err)
	}
	province := save(Master{Category: "PROVINSI", Code: "31", Label: "Jakarta"})
	city := save(Master{Category: "KABUPATEN", Code: "3171", Label: "Jakarta Pusat", Parent: "31"})
	wantStatus(a.removeMaster(province.ID, "test"), 409)
	_, err = a.storeMaster(Master{ID: province.ID, Category: "PROVINSI", Code: "32", Label: "Changed"}, "test")
	wantStatus(err, 409)
	_, err = a.storeMaster(Master{Category: "KABUPATEN", Code: "9999", Label: "Invalid", Parent: "99"}, "test")
	wantStatus(err, 422)
	_, err = a.storeMaster(Master{ID: education.ID, Category: "PEKERJAAN", Code: "04", Label: "Changed"}, "test")
	wantStatus(err, 422)
	// Existing assessments prevent removal and reassignment, but allow display-name edits.
	_, err = a.db.Exec("INSERT INTO assessments(identity_key,nik,name,scheme,fields) VALUES(?,?,?,?,?)", strings.Repeat("a", 64), "0000000000000001", "TEST", "24", `{"education":"04","city":"3171","province":"31"}`)
	if err != nil {
		t.Fatal(err)
	}
	wantStatus(a.removeMaster(education.ID, "test"), 409)
	_, err = a.storeMaster(Master{ID: education.ID, Category: "PENDIDIKAN", Code: "05", Label: "S1"}, "test")
	wantStatus(err, 409)
	save(Master{Category: "PROVINSI", Code: "32", Label: "Jawa Barat"})
	city.Parent = "32"
	_, err = a.storeMaster(city, "test")
	wantStatus(err, 409)
	education.Label = "S1 / D4"
	save(education)
	// Imported schemes are identified by ID, not only kode_skema.
	scheme := save(Master{Category: "SKEMA", Code: "24", Label: "Skema baru"})
	if err = a.db.QueryRow("SELECT nama_skema FROM rw_skema WHERE id=24").Scan(&label); err != nil || label != "Skema baru" {
		t.Fatalf("scheme ID not synchronized: %s %v", label, err)
	}
	wantStatus(a.removeMaster(scheme.ID, "test"), 409)
	unused := save(Master{Category: "PENDIDIKAN", Code: "99", Label: "Unused"})
	if err = a.removeMaster(unused.ID, "test"); err != nil {
		t.Fatal(err)
	}
	if err = a.db.QueryRow("SELECT COUNT(*) FROM rw_parameterbnsp WHERE kode='99'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("delete not synchronized: %d %v", count, err)
	}
	// A mirror failure must roll back the master and its audit entry.
	var before int
	if err = a.db.QueryRow("SELECT COUNT(*) FROM audit").Scan(&before); err != nil {
		t.Fatal(err)
	}
	_, err = a.db.Exec("CREATE TRIGGER reject_parameter BEFORE INSERT ON rw_parameterbnsp FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='test mirror failure'")
	if err != nil {
		t.Fatal(err)
	}
	_, err = a.storeMaster(Master{Category: "PENDIDIKAN", Code: "98", Label: "Rollback"}, "test")
	if err == nil {
		t.Fatal("mirror error was ignored")
	}
	if err = a.db.QueryRow("SELECT COUNT(*) FROM master WHERE code='98'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial write: %d %v", count, err)
	}
	if err = a.db.QueryRow("SELECT COUNT(*) FROM audit").Scan(&count); err != nil || count != before {
		t.Fatalf("partial audit: %d %v", count, err)
	}
	_, err = a.storeMaster(Master{Category: "PENDIDIKAN", Code: "97", Label: strings.Repeat("x", 192)}, "test")
	wantStatus(err, 422)
}
