package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

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
	defer root.Close()
	a := testApp(t, root, c)
	for _, m := range append(testMasters(), Master{Category: "SKEMA", Code: "25", Label: "Skema kedua"}) {
		if _, e = a.db.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?)", m.Category, m.Code, m.Label, m.Parent); e != nil {
			t.Fatal(e)
		}
	}
	for _, query := range []string{
		"CREATE TABLE rw_registrationsequence (skema_id INT, year INT, last_seq INT)",
		"INSERT INTO rw_registrationsequence VALUES(24,2025,40)",
		"CREATE TABLE rw_pendaftaran (skema_id INT, nomor_registrasi VARCHAR(191))",
		"INSERT INTO rw_pendaftaran VALUES(24,'PPL 2605 00050')",
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
		f["test_date"] = ""
		f["registration_year"] = year
		return Record{Fields: f, GenerateRegistration: true}
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
	var reg string
	if e = a.db.QueryRow("SELECT registration FROM assessments WHERE id=?", id).Scan(&reg); e != nil || reg != "PPL 2605 00051" {
		t.Fatalf("seed %s %v", reg, e)
	}
	id = save(newRecord("24", "2027"), 200)
	if e = a.db.QueryRow("SELECT registration FROM assessments WHERE id=?", id).Scan(&reg); e != nil || reg != "PPL 2605 00052" {
		t.Fatalf("year collision %s %v", reg, e)
	}
	id = save(newRecord("25", "2026"), 200)
	if e = a.db.QueryRow("SELECT registration FROM assessments WHERE id=?", id).Scan(&reg); e != nil || reg != "PPL 2605 00001" {
		t.Fatalf("scheme independence %s %v", reg, e)
	}
	manual := newRecord("24", "2026")
	manual.GenerateRegistration = false
	manual.Fields["registration"] = "PPL 2605 00100"
	manual.Fields["certificate"] = "CERT-TEST"
	save(manual, 200)
	failed := newRecord("24", "2026")
	failed.Fields["certificate"] = "CERT-TEST"
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
	if _, e = a.db.Exec("INSERT INTO rw_pendaftaran VALUES(24,'ppl 2605 00600')"); e != nil {
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
}
