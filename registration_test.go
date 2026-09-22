package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestMySQLBackupBeforeCertificateCounters(t *testing.T) {
	a := numberingTestApp(t)
	backupPath := filepath.Join(t.TempDir(), "reference.zip")
	if e := a.writeBackup(backupPath); e != nil {
		t.Fatal(e)
	}
	z, e := zip.OpenReader(backupPath)
	if e != nil {
		t.Fatal(e)
	}
	reader, e := z.File[0].Open()
	if e != nil {
		z.Close()
		t.Fatal(e)
	}
	var snapshot Backup
	e = json.NewDecoder(reader).Decode(&snapshot)
	reader.Close()
	z.Close()
	if e != nil {
		t.Fatal(e)
	}
	// Simulate a reference backup made before certificate numbering existed.
	delete(snapshot.Tables, "certificate_sequences")
	oldPath := filepath.Join(t.TempDir(), "old-reference.zip")
	f, e := os.Create(oldPath)
	if e != nil {
		t.Fatal(e)
	}
	zw := zip.NewWriter(f)
	w, e := zw.Create("manifest.json")
	if e != nil {
		t.Fatal(e)
	}
	if e = json.NewEncoder(w).Encode(snapshot); e != nil {
		t.Fatal(e)
	}
	if e = zw.Close(); e != nil {
		t.Fatal(e)
	}
	if e = f.Close(); e != nil {
		t.Fatal(e)
	}
	root, e := connect(a.cfg, "")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	restored := testApp(t, root, a.cfg)
	if e = restored.restore(oldPath); e != nil {
		t.Fatal(e)
	}
	tx, e := restored.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	r := numberedRecord("24", "2026")
	if _, e = saveRecord(tx, r); e != nil {
		t.Fatal(e)
	}
	if r.Fields["certificate"] != "64911 4210 3 0000001 2026" {
		t.Fatal(r.Fields["certificate"])
	}
}

func numberingTestApp(t *testing.T) *App {
	t.Helper()
	source := os.Getenv("LSPFI_TEST_SOURCE_ENV")
	if source == "" {
		t.Skip("set LSPFI_TEST_SOURCE_ENV for isolated MySQL integration test")
	}
	c, _, e := sourceConfig(source)
	if e != nil {
		t.Fatal(e)
	}
	root, e := connect(c, "")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	a := testApp(t, root, c)
	for _, m := range append(testMasters(), Master{Category: "SKEMA", Code: "25", Label: "Skema kedua"}, Master{Category: "SKEMA", Code: "99", Label: "Skema baru"}) {
		if _, e = a.db.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?)", m.Category, m.Code, m.Label, m.Parent); e != nil {
			t.Fatal(e)
		}
	}
	a.sessions["admin"] = Session{Username: "admin", Role: "admin", Expires: time.Now().Add(time.Hour)}
	return a
}

func numberedRecord(scheme, year string) Record {
	f := testFields()
	f["scheme"], f["registration"], f["certificate"] = scheme, "", ""
	f["registration_year"], f["certificate_year"] = year, year
	return Record{Fields: f, GenerateRegistration: true, GenerateCertificate: true}
}

func TestMySQLNumberingWithOldSnapshots(t *testing.T) {
	a := numberingTestApp(t)
	const workers = 8
	// Every transaction establishes its snapshot before any allocation commits.
	transactions := make([]*sql.Tx, workers)
	for i := range transactions {
		tx, e := a.db.Begin()
		if e != nil {
			t.Fatal(e)
		}
		t.Cleanup(func() { tx.Rollback() })
		var count int
		if e = tx.QueryRow("SELECT COUNT(*) FROM assessments").Scan(&count); e != nil {
			t.Fatal(e)
		}
		transactions[i] = tx
	}
	errs := make(chan error, workers)
	for i, tx := range transactions {
		go func(i int, tx *sql.Tx) {
			r := numberedRecord(fmt.Sprint(24+i%2), fmt.Sprint(2026+i%3))
			_, e := saveRecord(tx, r)
			if e == nil {
				e = tx.Commit()
			} else {
				tx.Rollback()
			}
			errs <- e
		}(i, tx)
	}
	for range workers {
		if e := <-errs; e != nil {
			t.Error(e)
		}
	}
	var total, unique int
	if e := a.db.QueryRow("SELECT COUNT(*),COUNT(DISTINCT certificate) FROM assessments").Scan(&total, &unique); e != nil || total != workers || unique != workers {
		t.Fatalf("certificates %d/%d: %v", total, unique, e)
	}
	for _, scheme := range []string{"24", "25"} {
		floor, e := registrationFloor(a.db, scheme)
		if e != nil || floor != 4 {
			t.Fatalf("scheme %s floor %d: %v", scheme, floor, e)
		}
	}
	floor, e := certificateFloor(a.db)
	if e != nil || floor != workers {
		t.Fatalf("certificate floor %d: %v", floor, e)
	}
}

func TestMySQLCertificateValidationAndMetadata(t *testing.T) {
	a := numberingTestApp(t)
	cookie := &http.Cookie{Name: "lspfi_session", Value: "admin"}
	save := func(r Record, status int) int64 {
		w := jsonRequest(a, "POST", "/api/records", r, cookie)
		requireStatus(t, w, status)
		var result struct {
			ID int64 `json:"id"`
		}
		if e := json.Unmarshal(w.Body.Bytes(), &result); e != nil {
			t.Fatal(e)
		}
		return result.ID
	}
	unknown := numberedRecord("99", "2026")
	save(unknown, 422)
	if floor, e := registrationFloor(a.db, "99"); e != nil || floor != 0 {
		t.Fatalf("failed allocation consumed registration: %d %v", floor, e)
	}
	for _, q := range []string{
		"CREATE TABLE rw_skema(id INT PRIMARY KEY,kode_skema VARCHAR(100),kode_sektor VARCHAR(20),kode_profesi VARCHAR(20),jenjang INT)",
		"INSERT INTO rw_skema VALUES(24,'CUSTOM-24','64912','2420',6)",
	} {
		if _, e := a.db.Exec(q); e != nil {
			t.Fatal(e)
		}
	}
	r := numberedRecord("24", "2026")
	id := save(r, 200)
	var cert string
	if e := a.db.QueryRow("SELECT certificate FROM assessments WHERE id=?", id).Scan(&cert); e != nil || cert != "64912 2420 6 0000001 2026" {
		t.Fatalf("source metadata: %s %v", cert, e)
	}
	meta, e := skemaInfo(a.db, "CUSTOM-24")
	if e != nil || meta.Jenjang != 6 {
		t.Fatalf("scheme code lookup: %+v %v", meta, e)
	}
	manual := numberedRecord("25", "2028")
	manual.GenerateCertificate = false
	manual.Fields["certificate"] = "64911 4210 3 0000500 2027"
	save(manual, 422)
	manual.Fields["certificate_year"] = ""
	manualID := save(manual, 200)
	var year string
	if e := a.db.QueryRow("SELECT fields->>'$.certificate_year' FROM assessments WHERE id=?", manualID).Scan(&year); e != nil || year != "2027" {
		t.Fatalf("manual year: %s %v", year, e)
	}
	nextID := save(numberedRecord("25", "2029"), 200)
	if e := a.db.QueryRow("SELECT certificate FROM assessments WHERE id=?", nextID).Scan(&cert); e != nil || cert != "64911 4210 3 0000501 2029" {
		t.Fatalf("manual floor: %s %v", cert, e)
	}
	edit := numberedRecord("25", "2029")
	edit.ID, edit.Version = nextID, 1
	edit.GenerateRegistration = false
	edit.Fields["registration"] = "EDIT-EXISTING"
	save(edit, 422)
	// Duplicates roll back both counters.
	save(manual, 409)
	if floor, e := certificateFloor(a.db); e != nil || floor != 501 {
		t.Fatalf("rollback counter %d %v", floor, e)
	}
	if _, e := a.db.Exec("UPDATE rw_skema SET kode_profesi='' WHERE id=24"); e != nil {
		t.Fatal(e)
	}
	save(numberedRecord("24", "2026"), 422)
	if _, e := a.db.Exec("UPDATE certificate_sequences SET last_seq=? WHERE scheme='0' AND year=0", maxRegistrationSequence); e != nil {
		t.Fatal(e)
	}
	save(numberedRecord("25", "2026"), 422)
}

func TestRegistrationFormat(t *testing.T) {
	for n, want := range map[int64]string{1: "PPL 2605 00001", 99999: "PPL 2605 99999", 100000: "PPL 2605 100000"} {
		if got := registrationNumber(n); got != want || registrationSequence(got) != n {
			t.Fatalf("%d: %s", n, got)
		}
	}
	for _, value := range []string{"", "ARSIP-123", "PPL 2606 00001", "PPL 2605 -1"} {
		if registrationSequence(value) != 0 {
			t.Fatalf("legacy value treated as sequence: %s", value)
		}
	}
	if registrationSequence("ppl 2605 00009 ") != 9 {
		t.Fatal("case-insensitive legacy collision ignored")
	}
	if registrationSequence("PPL 2605 999999999999999999999999") != maxRegistrationSequence {
		t.Fatal("overflow must stop allocation")
	}
	f := testFields()
	f["registration"] = ""
	f["test_date"] = ""
	if _, issues := validate(f, testMasters(), true); len(issues) > 0 {
		t.Fatal(issues)
	}
	if _, issues := validate(f, testMasters()); len(issues) == 0 {
		t.Fatal("archive identity validation removed")
	}
	f["registration_year"] = "1800"
	if _, issues := validate(f, testMasters(), true); len(issues) == 0 {
		t.Fatal("invalid year accepted")
	}
}

func TestCertificateFormat(t *testing.T) {
	meta := SchemeMetadata{KodeSektor: "64911", KodeProfesi: "4210", Jenjang: 3}
	want := "64911 4210 3 0000001 2026"
	if got := certificateNumber(meta, 1, 2026); got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
	want45 := "64911 4210 3 0000045 2026"
	if got := certificateNumber(meta, 45, 2026); got != want45 {
		t.Fatalf("want %s, got %s", want45, got)
	}
	seq, yr := certificateSequence(want45)
	if seq != 45 || yr != 2026 {
		t.Fatalf("parsed seq=%d yr=%d, want 45, 2026", seq, yr)
	}
	// Case-insensitive and extra spaces
	seqSpace, yrSpace := certificateSequence("  64911   4210   3   0000045   2026  ")
	if seqSpace != 45 || yrSpace != 2026 {
		t.Fatalf("extra spaces: seq=%d yr=%d", seqSpace, yrSpace)
	}
	// Invalid formats
	for _, invalid := range []string{"", "INVALID", "64911 4210 3", "CERT-123"} {
		if s, _ := certificateSequence(invalid); s != 0 {
			t.Fatalf("invalid certificate parsed as sequence: %s", invalid)
		}
	}
	// Default metadata lookup for standard schemes
	meta24, _ := skemaInfo(nil, "24")
	if meta24.KodeSektor != "64911" || meta24.KodeProfesi != "4210" || meta24.Jenjang != 3 {
		t.Fatalf("meta24 mismatch: %+v", meta24)
	}
	meta30, _ := skemaInfo(nil, "30")
	if meta30.KodeSektor != "64911" || meta30.KodeProfesi != "2510" || meta30.Jenjang != 5 {
		t.Fatalf("meta30 mismatch: %+v", meta30)
	}
	meta34, _ := skemaInfo(nil, "34")
	if meta34.KodeSektor != "64911" || meta34.KodeProfesi != "2420" || meta34.Jenjang != 6 {
		t.Fatalf("meta34 mismatch: %+v", meta34)
	}
}

func TestMySQLRegistrationNumbering(t *testing.T) {
	source := os.Getenv("LSPFI_TEST_SOURCE_ENV")
	if source == "" {
		t.Skip("set LSPFI_TEST_SOURCE_ENV for isolated MySQL integration test")
	}
	c, _, e := sourceConfig(source)
	if e != nil {
		t.Fatal(e)
	}
	root, e := connect(c, "")
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { root.Close() })
	a := testApp(t, root, c)
	for _, m := range append(testMasters(), Master{Category: "SKEMA", Code: "25", Label: "Skema kedua"}) {
		if _, e = a.db.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?)", m.Category, m.Code, m.Label, m.Parent); e != nil {
			t.Fatal(e)
		}
	}
	for _, query := range []string{
		"CREATE TABLE rw_registrationsequence (skema_id INT, year INT, last_seq INT)",
		"INSERT INTO rw_registrationsequence VALUES(24,2025,40)",
		"CREATE TABLE rw_pendaftaran (skema_id INT, nomor_registrasi VARCHAR(191), no_sertifikat VARCHAR(191))",
		"INSERT INTO rw_pendaftaran VALUES(24,'PPL 2605 00050','64911 4210 3 0000010 2026')",
		"CREATE TABLE rw_certificatesequence (skema_id INT, year INT, last_seq INT)",
		"INSERT INTO rw_certificatesequence VALUES(0,2026,10)",
	} {
		if _, e = a.db.Exec(query); e != nil {
			t.Fatal(e)
		}
	}
	a.sessions["admin"] = Session{Username: "admin", Role: "admin", Expires: time.Now().Add(time.Hour)}
	a.sessions["viewer"] = Session{Username: "viewer", Role: "viewer", Expires: time.Now().Add(time.Hour)}
	cookie := &http.Cookie{Name: "lspfi_session", Value: "admin"}
	requireStatus(t, request(a, "GET", "/api/registrations", nil, nil, ""), 401)
	requireStatus(t, request(a, "GET", "/api/registrations", nil, &http.Cookie{Name: "lspfi_session", Value: "viewer"}, ""), 403)
	newRecord := func(scheme, year string) Record {
		f := testFields()
		f["scheme"] = scheme
		f["registration"] = ""
		f["certificate"] = ""
		f["test_date"] = ""
		f["registration_year"] = year
		f["certificate_year"] = year
		return Record{Fields: f, GenerateRegistration: true, GenerateCertificate: true}
	}
	save := func(r Record, status int) int64 {
		w := jsonRequest(a, "POST", "/api/records", r, cookie)
		requireStatus(t, w, status)
		var result struct {
			ID int64 `json:"id"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		return result.ID
	}
	id := save(newRecord("24", "2026"), 200)
	var reg, cert string
	if e = a.db.QueryRow("SELECT registration, certificate FROM assessments WHERE id=?", id).Scan(&reg, &cert); e != nil || reg != "PPL 2605 00051" || cert != "64911 4210 3 0000011 2026" {
		t.Fatalf("seed reg=%s cert=%s %v", reg, cert, e)
	}
	id = save(newRecord("24", "2027"), 200)
	if e = a.db.QueryRow("SELECT registration, certificate FROM assessments WHERE id=?", id).Scan(&reg, &cert); e != nil || reg != "PPL 2605 00052" || cert != "64911 4210 3 0000012 2027" {
		t.Fatalf("year collision reg=%s cert=%s %v", reg, cert, e)
	}
	id = save(newRecord("25", "2026"), 200)
	if e = a.db.QueryRow("SELECT registration, certificate FROM assessments WHERE id=?", id).Scan(&reg, &cert); e != nil || reg != "PPL 2605 00001" || cert != "64911 4210 3 0000013 2026" {
		t.Fatalf("scheme independence reg=%s cert=%s %v", reg, cert, e)
	}
	manual := newRecord("24", "2026")
	manual.GenerateRegistration = false
	manual.GenerateCertificate = false
	manual.Fields["registration"] = "PPL 2605 00100"
	manual.Fields["certificate"] = "CERT-TEST"
	save(manual, 200)
	failed := newRecord("24", "2026")
	failed.Fields["certificate"] = "CERT-TEST"
	failed.GenerateCertificate = false
	save(failed, 409)
	id = save(newRecord("24", "2026"), 200)
	if e = a.db.QueryRow("SELECT registration FROM assessments WHERE id=?", id).Scan(&reg); e != nil || reg != "PPL 2605 00101" {
		t.Fatalf("manual floor / rollback %s %v", reg, e)
	}
	edit := newRecord("24", "2026")
	edit.ID = id
	edit.Version = 1
	save(edit, 422)
	// Concurrent transactions bypass the HTTP mutex, verifying database locking.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tx, e := a.db.Begin()
			if e != nil {
				errs <- e
				return
			}
			defer tx.Rollback()
			if _, e = saveRecord(tx, newRecord("24", "2026")); e == nil {
				e = tx.Commit()
			}
			errs <- e
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var count, distinct int
	if e = a.db.QueryRow("SELECT COUNT(*),COUNT(DISTINCT registration) FROM assessments WHERE scheme='24'").Scan(&count, &distinct); e != nil || count != distinct {
		t.Fatalf("duplicates %d/%d: %v", count, distinct, e)
	}
	requireStatus(t, request(a, "GET", "/api/registrations", nil, cookie, ""), 200)
	// Counter survives reference-only backup even though candidate rows do not.
	// Include a legacy registration above its stale counter; its candidate row
	// must not be needed after restoration to avoid reissuing that number.
	if _, e = a.db.Exec("INSERT INTO rw_pendaftaran(skema_id, nomor_registrasi, no_sertifikat) VALUES(24,'ppl 2605 00600','64911 4210 3 0000600 2026')"); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(t.TempDir(), "references.zip")
	if e = a.writeBackup(path); e != nil {
		t.Fatal(e)
	}
	restored := testApp(t, root, c)
	if e = restored.restore(path); e != nil {
		t.Fatal(e)
	}
	tx, e := restored.db.Begin()
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback()
	r := newRecord("24", "2028")
	if _, e = saveRecord(tx, r); e != nil {
		t.Fatal(e)
	}
	if got, want := r.Fields["registration"], fmt.Sprintf("PPL 2605 %05d", 601); got != want {
		t.Fatalf("restored sequence %s want %s", got, want)
	}
	if got, want := r.Fields["certificate"], "64911 4210 3 0000601 2028"; got != want {
		t.Fatalf("restored cert sequence %s want %s", got, want)
	}
}

func TestUnknownCertificateScheme(t *testing.T) {
	if _, e := skemaInfo(nil, "unknown"); e == nil {
		t.Fatal("unknown scheme must not receive guessed metadata")
	}
	if validSchemeMetadata(SchemeMetadata{"", "4210", 3}) {
		t.Fatal("empty sector accepted")
	}
}

func TestCertificateYearMismatch(t *testing.T) {
	r := Record{Fields: map[string]string{"certificate": "64911 4210 3 0000045 2026", "certificate_year": "2027"}}
	if e := prepareCertificate(nil, &r); e == nil {
		t.Fatal("mismatched year accepted")
	}
}

func TestPrivateReferenceAssetsNotPublic(t *testing.T) {
	a := &App{}
	for _, path := range []string{"/data/manifest.json", "/data/master_data.json", "/data/master_data.sql", "/data/registrasiAsesi.ts"} {
		requireStatus(t, request(a, "GET", path, nil, nil, ""), http.StatusNotFound)
	}
	requireStatus(t, request(a, "GET", "/app.js", nil, nil, ""), http.StatusOK)
}
