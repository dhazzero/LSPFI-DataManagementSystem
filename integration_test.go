package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testApp(t *testing.T, admin *sql.DB, c Config) *App {
	t.Helper()
	name := "lspfi_dms_test_" + token()[:8]
	if _, e := admin.Exec("CREATE DATABASE `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); e != nil {
		t.Fatal(e)
	}
	db, e := connect(c, name)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() {
		db.Close()
		if _, e := admin.Exec("DROP DATABASE `" + name + "`"); e != nil {
			t.Error(e)
		}
	})
	if e = migrate(db); e != nil {
		t.Fatal(e)
	}
	c.Storage = t.TempDir()
	for _, dir := range []string{"documents", "imports"} {
		if e = os.MkdirAll(filepath.Join(c.Storage, dir), 0700); e != nil {
			t.Fatal(e)
		}
	}
	return &App{db: db, cfg: c, sessions: map[string]Session{}, previews: map[string]Preview{}, attempts: map[string][]time.Time{}}
}
func request(a *App, method, path string, body io.Reader, cookie *http.Cookie, contentType string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "http://localhost"+path, body)
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if contentType != "" {
		r.Header.Set("Content-Type", contentType)
	}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, r)
	return w
}
func jsonRequest(a *App, method, path string, value any, cookie *http.Cookie) *httptest.ResponseRecorder {
	b, _ := json.Marshal(value)
	return request(a, method, path, bytes.NewReader(b), cookie, "application/json")
}
func multipartRequest(a *App, path, name string, data []byte, extra map[string]string, cookie *http.Cookie) *httptest.ResponseRecorder {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	part, _ := w.CreateFormFile("file", name)
	part.Write(data)
	for k, v := range extra {
		w.WriteField(k, v)
	}
	w.Close()
	return request(a, "POST", path, &b, cookie, w.FormDataContentType())
}
func requireStatus(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("HTTP %d wanted %d: %s", w.Code, status, w.Body.String())
	}
}
func TestMySQLArchiveLifecycle(t *testing.T) {
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
	for _, m := range testMasters() {
		if _, e = a.db.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?)", m.Category, m.Code, m.Label, m.Parent); e != nil {
			t.Fatal(e)
		}
	}
	hash, _ := bcrypt.GenerateFromPassword([]byte("uji-password-123"), bcrypt.MinCost)
	for _, role := range []string{"admin", "viewer"} {
		if _, e = a.db.Exec("INSERT INTO users(username,password_hash,role) VALUES(?,?,?)", role, string(hash), role); e != nil {
			t.Fatal(e)
		}
	}
	login := jsonRequest(a, "POST", "/api/login", map[string]string{"username": "admin", "password": "uji-password-123"}, nil)
	requireStatus(t, login, 200)
	cookie := login.Result().Cookies()[0]
	requireStatus(t, request(a, "GET", "/api/records", nil, nil, ""), 401)
	r := httptest.NewRequest("POST", "http://localhost/api/master", strings.NewReader(`{}`))
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://evil.example")
	out := httptest.NewRecorder()
	a.routes().ServeHTTP(out, r)
	requireStatus(t, out, 403)
	viewer := jsonRequest(a, "POST", "/api/login", map[string]string{"username": "viewer", "password": "uji-password-123"}, nil).Result().Cookies()[0]
	requireStatus(t, jsonRequest(a, "POST", "/api/master", testMasters()[0], viewer), 403)
	requireStatus(t, request(a, "GET", "/api/backup", nil, viewer, ""), 403)
	f, issues := validate(testFields(), testMasters())
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	badFields := testFields()
	badFields["nik"] = "123"
	badFields["registration"] = "INVALID-ROW"
	wb, e := makeWorkbook([]Record{{Fields: f}, {Fields: badFields}}, testMasters())
	if e != nil {
		t.Fatal(e)
	}
	buffer, e := wb.WriteToBuffer()
	wb.Close()
	if e != nil {
		t.Fatal(e)
	}
	preview := multipartRequest(a, "/api/import/preview", "DataInduk.xlsx", buffer.Bytes(), nil, cookie)
	requireStatus(t, preview, 200)
	var p Preview
	if e = json.Unmarshal(preview.Body.Bytes(), &p); e != nil {
		t.Fatal(e)
	}
	if p.Valid != 1 {
		t.Fatalf("preview: %s", preview.Body.String())
	}
	requireStatus(t, jsonRequest(a, "POST", "/api/import/commit", map[string]any{"id": p.ID, "rows": []int{2, 3}}, cookie), 409)
	var count int
	if e = a.db.QueryRow("SELECT COUNT(*) FROM assessments").Scan(&count); e != nil || count != 0 {
		t.Fatalf("invalid batch partially committed: %d %v", count, e)
	}
	requireStatus(t, jsonRequest(a, "POST", "/api/import/commit", map[string]any{"id": p.ID, "rows": []int{2}}, cookie), 200)
	requireStatus(t, jsonRequest(a, "POST", "/api/import/commit", map[string]any{"id": p.ID, "rows": []int{2}}, cookie), 410)
	preview = multipartRequest(a, "/api/import/preview", "DataInduk.xlsx", buffer.Bytes(), nil, cookie)
	requireStatus(t, preview, 200)
	json.Unmarshal(preview.Body.Bytes(), &p)
	if p.Valid != 0 {
		t.Fatal("duplicate import accepted")
	}
	rows, e := a.records("", "", "", "")
	if e != nil || len(rows) != 1 {
		t.Fatalf("records %v %v", rows, e)
	}
	record := rows[0]
	id := record.ID
	record.Fields["company"] = "PERUSAHAAN UJI"
	requireStatus(t, jsonRequest(a, "POST", "/api/records", record, cookie), 200)
	requireStatus(t, jsonRequest(a, "POST", "/api/records", record, cookie), 409)
	pdf := []byte("%PDF-1.4\n1 0 obj\n<< /Type /Catalog >>\nendobj\n%%EOF\n")
	upload := multipartRequest(a, fmt.Sprintf("/api/records/%d/documents", id), "scan.pdf", pdf, map[string]string{"category": "FR.AK.02"}, cookie)
	requireStatus(t, upload, 201)
	requireStatus(t, multipartRequest(a, fmt.Sprintf("/api/records/%d/documents", id), "unsafe.pdf", []byte("<script>alert(1)</script>"), map[string]string{"category": "KTP"}, cookie), 422)
	var doc map[string]string
	json.Unmarshal(upload.Body.Bytes(), &doc)
	scan := request(a, "GET", "/api/documents/"+doc["id"], nil, viewer, "")
	requireStatus(t, scan, 200)
	if !bytes.Equal(scan.Body.Bytes(), pdf) {
		t.Fatal("scan changed")
	}
	exported := request(a, "GET", "/api/export", nil, cookie, "")
	requireStatus(t, exported, 200)
	parsed, e := parseWorkbook(exported.Body.Bytes(), "export.xlsx", "", testMasters())
	if e != nil || len(parsed.Rows) != 1 || len(parsed.Rows[0].Errors) != 0 {
		t.Fatalf("export roundtrip %v %+v", e, parsed.Rows)
	}
	backup := request(a, "GET", "/api/backup", nil, cookie, "")
	requireStatus(t, backup, 200)
	path := filepath.Join(t.TempDir(), "backup.zip")
	os.WriteFile(path, backup.Body.Bytes(), 0600)
	restored := testApp(t, root, c)
	if e = restored.restore(path); e != nil {
		t.Fatal(e)
	}
	rr, e := restored.records("", "", "", "")
	if e != nil || len(rr) != 1 || rr[0].Documents != 1 || rr[0].Fields["company"] != "PERUSAHAAN UJI" {
		t.Fatalf("restore mismatch: %+v %v", rr, e)
	}
	if e = restored.restore(path); e == nil {
		t.Fatal("restore overwrote existing database")
	}
	os.WriteFile(filepath.Join(a.cfg.Storage, "documents", doc["id"]), []byte("changed"), 0600)
	requireStatus(t, request(a, "GET", "/api/documents/"+doc["id"], nil, cookie, ""), 409)
	malicious := filepath.Join(t.TempDir(), "bad.zip")
	badFile, _ := os.Create(malicious)
	zw := zip.NewWriter(badFile)
	entry, _ := zw.Create("../outside.txt")
	entry.Write([]byte("bad"))
	zw.Close()
	badFile.Close()
	empty := testApp(t, root, c)
	if e = empty.restore(malicious); e == nil {
		t.Fatal("unsafe backup path accepted")
	}
	requireStatus(t, jsonRequest(a, "POST", "/api/password", map[string]string{"current": "uji-password-123", "password": "uji-password-baru"}, cookie), 200)
	requireStatus(t, request(a, "GET", "/api/records", nil, cookie, ""), 401)
}
