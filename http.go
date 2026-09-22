package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

type Session struct {
	Username string    `json:"username"`
	Role     string    `json:"role"`
	Expires  time.Time `json:"-"`
}
type Document struct {
	ID           string `json:"id"`
	AssessmentID int64  `json:"assessment_id"`
	Name         string `json:"name"`
	Category     string `json:"category"`
	MIME         string `json:"mime"`
	Size         int64  `json:"size"`
	SHA          string `json:"sha256"`
	Created      string `json:"created"`
}

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, code int, message string) {
	jsonOut(w, code, map[string]string{"error": message})
}
func internal(w http.ResponseWriter, e error) {
	log.Printf("Operasi gagal: %v", e)
	var dbErr *mysql.MySQLError
	if errors.As(e, &dbErr) && dbErr.Number == 1062 {
		fail(w, 409, "Data duplikat: nomor registrasi pada skema ini atau nomor sertifikat sudah digunakan.")
		return
	}
	fail(w, 500, "Operasi gagal. Periksa koneksi database dan log aplikasi.")
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		fail(w, 400, "Data permintaan tidak valid.")
		return false
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		fail(w, 400, "Data permintaan berlebih.")
		return false
	}
	return true
}
func (a *App) session(r *http.Request) (Session, bool) {
	c, e := r.Cookie("lspfi_session")
	if e != nil {
		return Session{}, false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s, ok := a.sessions[c.Value]
	if !ok || time.Now().After(s.Expires) {
		delete(a.sessions, c.Value)
		return Session{}, false
	}
	return s, true
}
func (a *App) protected(admin bool, h func(http.ResponseWriter, *http.Request, Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s, ok := a.session(r)
		if !ok {
			fail(w, 401, "Silakan masuk kembali.")
			return
		}
		if admin && s.Role != "admin" {
			fail(w, 403, "Hanya admin yang dapat melakukan tindakan ini.")
			return
		}
		h(w, r, s)
	}
}
func sameOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return r.Header.Get("Sec-Fetch-Site") != "cross-site"
	}
	u, e := url.Parse(origin)
	return e == nil && u.Host == r.Host && u.Scheme == "http"
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]string{"application": "lspfi-dms", "status": "ready"})
	})
	static, _ := fs.Sub(assets, "web")
	mux.Handle("GET /", http.FileServer(http.FS(static)))
	mux.HandleFunc("POST /api/login", a.login)
	mux.HandleFunc("POST /api/logout", a.protected(false, func(w http.ResponseWriter, r *http.Request, s Session) {
		c, _ := r.Cookie("lspfi_session")
		a.mu.Lock()
		delete(a.sessions, c.Value)
		a.mu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "lspfi_session", Value: "", Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		jsonOut(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/me", a.protected(false, func(w http.ResponseWriter, r *http.Request, s Session) { jsonOut(w, 200, s) }))
	mux.HandleFunc("GET /api/meta", a.protected(false, func(w http.ResponseWriter, r *http.Request, s Session) {
		m, e := a.masters()
		if e != nil {
			internal(w, e)
			return
		}
		jsonOut(w, 200, map[string]any{"fields": fields, "masters": m, "database": databaseName, "storage": a.cfg.Storage})
	}))
	mux.HandleFunc("GET /api/records", a.protected(false, func(w http.ResponseWriter, r *http.Request, s Session) {
		q := r.URL.Query()
		rows, e := a.records(q.Get("q"), q.Get("scheme"), q.Get("result"), q.Get("year"))
		if e != nil {
			internal(w, e)
			return
		}
		jsonOut(w, 200, rows)
	}))
	mux.HandleFunc("POST /api/records", a.protected(true, a.save))
	mux.HandleFunc("GET /api/registrations", a.protected(true, a.registrationSummary))
	mux.HandleFunc("GET /api/records/{id}/documents", a.protected(false, a.listDocuments))
	mux.HandleFunc("POST /api/records/{id}/documents", a.protected(true, a.uploadDocument))
	mux.HandleFunc("GET /api/documents/{id}", a.protected(false, a.readDocument))
	mux.HandleFunc("POST /api/import/preview", a.protected(true, a.preview))
	mux.HandleFunc("POST /api/import/commit", a.protected(true, a.commitImport))
	mux.HandleFunc("GET /api/export", a.protected(false, a.export))
	mux.HandleFunc("POST /api/master", a.protected(true, a.saveMaster))
	mux.HandleFunc("DELETE /api/master/{id}", a.protected(true, a.deleteMaster))
	mux.HandleFunc("GET /api/audit", a.protected(true, a.auditList))
	mux.HandleFunc("POST /api/password", a.protected(false, a.changePassword))
	mux.HandleFunc("POST /api/users", a.protected(true, a.createUser))
	mux.HandleFunc("GET /api/backup", a.protected(true, a.backup))
	mux.HandleFunc("GET /api/registerweb", a.protected(true, a.legacyTables))
	mux.HandleFunc("GET /api/registerweb/{table}", a.protected(true, a.legacyData))
	mux.HandleFunc("GET /api/registerweb/{table}/export", a.protected(true, a.legacyExport))
	mux.HandleFunc("GET /api/registerweb/pendaftaran/{id}/candidate", a.protected(true, a.legacyCandidateHandler))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, _, e := net.SplitHostPort(r.Host)
		if e != nil {
			host = r.Host
		}
		if host != "127.0.0.1" && host != "localhost" && host != "::1" {
			fail(w, 403, "Host tidak diizinkan.")
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; img-src 'self' blob: data:; frame-src 'self' blob:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'self'")
		if r.Method != "GET" && r.Method != "HEAD" && !sameOrigin(r) {
			fail(w, 403, "Permintaan harus berasal dari aplikasi ini.")
			return
		}
		mux.ServeHTTP(w, r)
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if !decode(w, r, &input) {
		return
	}
	ip, _, _ := net.SplitHostPort(r.RemoteAddr)
	a.mu.Lock()
	recent := []time.Time{}
	for _, t := range a.attempts[ip] {
		if time.Since(t) < time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 10 {
		a.mu.Unlock()
		fail(w, 429, "Terlalu banyak percobaan. Tunggu satu menit.")
		return
	}
	a.attempts[ip] = append(recent, time.Now())
	a.mu.Unlock()
	var hash, role string
	e := a.db.QueryRow("SELECT password_hash,role FROM users WHERE username=?", strings.TrimSpace(input.Username)).Scan(&hash, &role)
	if e != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		fail(w, 401, "Nama pengguna atau kata sandi salah.")
		return
	}
	s := Session{Username: strings.TrimSpace(input.Username), Role: role, Expires: time.Now().Add(8 * time.Hour)}
	id := token()
	a.mu.Lock()
	for k, v := range a.sessions {
		if time.Now().After(v.Expires) {
			delete(a.sessions, k)
		}
	}
	a.sessions[id] = s
	delete(a.attempts, ip)
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: "lspfi_session", Value: id, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 28800})
	jsonOut(w, 200, s)
}
func (a *App) save(w http.ResponseWriter, r *http.Request, s Session) {
	var input Record
	if !decode(w, r, &input) {
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	masters, e := a.masters()
	if e != nil {
		internal(w, e)
		return
	}
	mapped, issues := validate(input.Fields, masters, input.GenerateRegistration)
	if len(issues) > 0 {
		jsonOut(w, 422, map[string]any{"error": "Periksa data yang diisi.", "issues": issues})
		return
	}
	input.Fields = mapped
	if input.ID > 0 {
		if e = a.db.QueryRow("SELECT source FROM assessments WHERE id=?", input.ID).Scan(&input.Source); e == sql.ErrNoRows {
			fail(w, 404, "Data tidak ditemukan.")
			return
		} else if e != nil {
			internal(w, e)
			return
		}
	} else {
		input.Source = "Input manual"
		if input.LegacyID > 0 {
			candidate, e := a.legacyCandidate(input.LegacyID)
			if e == sql.ErrNoRows {
				fail(w, 404, "Pendaftaran RegisterWeb tidak ditemukan.")
				return
			}
			if e != nil {
				internal(w, e)
				return
			}
			if candidate.ID > 0 {
				fail(w, 409, "Pendaftaran ini sudah menjadi arsip. Buka arsip yang sudah ada untuk mengubahnya.")
				return
			}
			input.Source = candidate.Source
			if input.GenerateRegistration && candidate.Fields["registration"] != "" {
				fail(w, 422, "Pendaftaran RegisterWeb sudah memiliki nomor registrasi. Gunakan nomor sumber.")
				return
			}
			if input.GenerateCertificate && candidate.Fields["certificate"] != "" {
				fail(w, 422, "Pendaftaran RegisterWeb sudah memiliki nomor sertifikat. Gunakan nomor sumber.")
				return
			}
		}
	}
	tx, e := a.db.Begin()
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback()
	id, e := saveRecord(tx, input)
	if e != nil {
		if errors.Is(e, errRegistration) || errors.Is(e, errCertificate) {
			fail(w, 422, e.Error())
		} else if strings.Contains(e.Error(), "muat ulang") {
			fail(w, 409, e.Error())
		} else {
			internal(w, e)
		}
		return
	}
	if e = audit(tx, s.Username, "SAVE_ASSESSMENT", fmt.Sprintf("Asesmen #%d · skema %s · registrasi %s · sertifikat %s · otomatis_reg %t · otomatis_cert %t", id, input.Fields["scheme"], input.Fields["registration"], input.Fields["certificate"], input.GenerateRegistration, input.GenerateCertificate)); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"id": id})
}
func (a *App) preview(w http.ResponseWriter, r *http.Request, s Session) {
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if e := r.ParseMultipartForm(21 << 20); e != nil {
		fail(w, 400, "File maksimal 20 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, h, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "Pilih file Excel atau CSV.")
		return
	}
	defer file.Close()
	if h.Size > 20<<20 {
		fail(w, 400, "File maksimal 20 MB.")
		return
	}
	b, e := io.ReadAll(file)
	if e != nil {
		fail(w, 400, "File gagal dibaca.")
		return
	}
	masters, e := a.masters()
	if e != nil {
		internal(w, e)
		return
	}
	p, e := parseWorkbook(b, h.Filename, r.FormValue("sheet"), masters)
	if e != nil {
		fail(w, 422, e.Error())
		return
	}
	if e = a.validateDuplicates(&p); e != nil {
		internal(w, e)
		return
	}
	p.User = s.Username
	a.mu.Lock()
	for k, v := range a.previews {
		if time.Now().After(v.Expires) || v.User == s.Username {
			delete(a.previews, k)
		}
	}
	if len(a.previews) >= 5 {
		a.mu.Unlock()
		fail(w, 429, "Tinjauan impor sedang penuh. Coba kembali setelah 30 menit.")
		return
	}
	a.previews[p.ID] = p
	a.mu.Unlock()
	jsonOut(w, 200, p)
}
func (a *App) commitImport(w http.ResponseWriter, r *http.Request, s Session) {
	var input struct {
		ID   string `json:"id"`
		Rows []int  `json:"rows"`
	}
	if !decode(w, r, &input) {
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	a.mu.Lock()
	p, ok := a.previews[input.ID]
	a.mu.Unlock()
	if !ok || p.User != s.Username || time.Now().After(p.Expires) {
		fail(w, 410, "Tinjauan kedaluwarsa; unggah file kembali.")
		return
	}
	selected := map[int]bool{}
	for _, n := range input.Rows {
		selected[n] = true
	}
	if len(selected) == 0 {
		fail(w, 400, "Pilih sedikitnya satu baris valid.")
		return
	}
	if e := a.validateDuplicates(&p); e != nil {
		internal(w, e)
		return
	}
	masters, e := a.masters()
	if e != nil {
		internal(w, e)
		return
	}
	chosen := []ImportRow{}
	for _, row := range p.Rows {
		if !selected[row.Number] {
			continue
		}
		if len(row.Errors) > 0 {
			fail(w, 409, fmt.Sprintf("Baris %d tidak valid atau sudah diimpor. Tinjau ulang.", row.Number))
			return
		}
		mapped, issues := validate(row.Fields, masters)
		if len(issues) > 0 {
			fail(w, 409, "Master berubah; tinjau impor kembali.")
			return
		}
		row.Fields = mapped
		chosen = append(chosen, row)
	}
	if len(chosen) != len(selected) {
		fail(w, 400, "Baris pilihan tidak ditemukan.")
		return
	}
	original := p.ID + strings.ToLower(filepath.Ext(p.Filename))
	path := filepath.Join(a.cfg.Storage, "imports", original)
	if e = os.WriteFile(path, p.Bytes, 0600); e != nil {
		internal(w, e)
		return
	}
	committed := false
	defer func() {
		if !committed {
			os.Remove(path)
		}
	}()
	tx, e := a.db.Begin()
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback()
	for _, row := range chosen {
		rec := Record{Fields: row.Fields, Source: fmt.Sprintf("%s · %s · baris %d · sumber %s", p.Filename, p.Sheet, row.Number, original)}
		if _, e = saveRecord(tx, rec); e != nil {
			internal(w, e)
			return
		}
	}
	if e = audit(tx, s.Username, "IMPORT", fmt.Sprintf("%d baris dari %s (%s)", len(chosen), p.Filename, p.ID)); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w, e)
		return
	}
	committed = true
	a.mu.Lock()
	delete(a.previews, p.ID)
	a.mu.Unlock()
	jsonOut(w, 200, map[string]any{"imported": len(chosen)})
}
func (a *App) export(w http.ResponseWriter, r *http.Request, s Session) {
	masters, e := a.masters()
	if e != nil {
		internal(w, e)
		return
	}
	records := []Record{}
	q := r.URL.Query()
	if q.Get("template") != "1" {
		records, e = a.records(q.Get("q"), q.Get("scheme"), q.Get("result"), q.Get("year"))
		if e != nil {
			internal(w, e)
			return
		}
	}
	f, e := makeWorkbook(records, masters)
	if e != nil {
		internal(w, e)
		return
	}
	defer f.Close()
	b, e := f.WriteToBuffer()
	if e != nil {
		internal(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="Data_Induk_Asesi_LSPFI.xlsx"`)
	w.Write(b.Bytes())
}
func (a *App) documents(id int64) ([]Document, error) {
	rows, e := a.db.Query("SELECT id,assessment_id,name,category,mime,size,sha256,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s') FROM documents WHERE assessment_id=? ORDER BY created_at DESC", id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Document{}
	for rows.Next() {
		var d Document
		if e = rows.Scan(&d.ID, &d.AssessmentID, &d.Name, &d.Category, &d.MIME, &d.Size, &d.SHA, &d.Created); e != nil {
			return nil, e
		}
		out = append(out, d)
	}
	return out, rows.Err()
}
func (a *App) listDocuments(w http.ResponseWriter, r *http.Request, s Session) {
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	docs, e := a.documents(id)
	if e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, docs)
}

var documentCategories = map[string]bool{"KTP": true, "FR.APL.01": true, "FR.APL.02": true, "FR.AK.02": true, "FR.IA": true, "SERTIFIKAT": true, "IJAZAH": true, "PORTOFOLIO": true, "LAINNYA": true}

func (a *App) uploadDocument(w http.ResponseWriter, r *http.Request, s Session) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "ID tidak valid.")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if e = r.ParseMultipartForm(21 << 20); e != nil {
		fail(w, 400, "File maksimal 20 MB.")
		return
	}
	defer r.MultipartForm.RemoveAll()
	file, h, e := r.FormFile("file")
	if e != nil {
		fail(w, 400, "Pilih file scan.")
		return
	}
	defer file.Close()
	category := r.FormValue("category")
	if !documentCategories[category] {
		fail(w, 422, "Jenis dokumen tidak valid.")
		return
	}
	if h.Size == 0 || h.Size > 20<<20 {
		fail(w, 422, "File harus berisi data dan maksimal 20 MB.")
		return
	}
	b, e := io.ReadAll(file)
	if e != nil {
		fail(w, 400, "Gagal membaca file.")
		return
	}
	typ := http.DetectContentType(b)
	ext := strings.ToLower(filepath.Ext(h.Filename))
	valid := (typ == "application/pdf" && ext == ".pdf") || (typ == "image/png" && ext == ".png") || (typ == "image/jpeg" && (ext == ".jpg" || ext == ".jpeg"))
	if !valid {
		fail(w, 422, "Scan harus berupa PDF, JPG, atau PNG dengan isi sesuai ekstensi.")
		return
	}
	name := filepath.Base(h.Filename)
	if len(name) > 255 {
		fail(w, 422, "Nama file terlalu panjang.")
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	var exists int
	if e = a.db.QueryRow("SELECT COUNT(*) FROM assessments WHERE id=?", id).Scan(&exists); e != nil {
		internal(w, e)
		return
	}
	if exists == 0 {
		fail(w, 404, "Asesmen tidak ditemukan.")
		return
	}
	docID := token()
	path := filepath.Join(a.cfg.Storage, "documents", docID)
	if e = os.WriteFile(path, b, 0600); e != nil {
		internal(w, e)
		return
	}
	done := false
	defer func() {
		if !done {
			os.Remove(path)
		}
	}()
	sum := sha256.Sum256(b)
	sha := hex.EncodeToString(sum[:])
	tx, e := a.db.Begin()
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO documents(id,assessment_id,name,category,mime,size,sha256) VALUES(?,?,?,?,?,?,?)", docID, id, name, category, typ, len(b), sha); e != nil {
		internal(w, e)
		return
	}
	if e = audit(tx, s.Username, "UPLOAD_SCAN", fmt.Sprintf("Asesmen #%d · %s · %s", id, category, docID)); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w, e)
		return
	}
	done = true
	jsonOut(w, 201, map[string]string{"id": docID})
}
func (a *App) readDocument(w http.ResponseWriter, r *http.Request, s Session) {
	id := r.PathValue("id")
	var name, typ, sha string
	e := a.db.QueryRow("SELECT name,mime,sha256 FROM documents WHERE id=?", id).Scan(&name, &typ, &sha)
	if e == sql.ErrNoRows {
		fail(w, 404, "Dokumen tidak ditemukan.")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	file, e := os.Open(filepath.Join(a.cfg.Storage, "documents", id))
	if e != nil {
		fail(w, 404, "Berkas tidak tersedia di penyimpanan. Periksa cadangan.")
		return
	}
	defer file.Close()
	hash := sha256.New()
	if _, e = io.Copy(hash, file); e != nil {
		internal(w, e)
		return
	}
	if hex.EncodeToString(hash.Sum(nil)) != sha {
		fail(w, 409, "Integritas berkas tidak cocok. Periksa cadangan sebelum menggunakan dokumen.")
		return
	}
	if _, e = file.Seek(0, 0); e != nil {
		internal(w, e)
		return
	}
	info, e := file.Stat()
	if e != nil {
		internal(w, e)
		return
	}
	disposition := "inline"
	if r.URL.Query().Get("download") == "1" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", typ)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func (a *App) auditList(w http.ResponseWriter, r *http.Request, s Session) {
	rows, e := a.db.Query("SELECT username,action,detail,DATE_FORMAT(created_at,'%Y-%m-%d %H:%i:%s') FROM audit ORDER BY id DESC LIMIT 200")
	if e != nil {
		internal(w, e)
		return
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var u, action, detail, date string
		if e = rows.Scan(&u, &action, &detail, &date); e != nil {
			internal(w, e)
			return
		}
		out = append(out, map[string]string{"username": u, "action": action, "detail": detail, "date": date})
	}
	if e = rows.Err(); e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, out)
}
func (a *App) changePassword(w http.ResponseWriter, r *http.Request, s Session) {
	var v struct {
		Current  string `json:"current"`
		Password string `json:"password"`
	}
	if !decode(w, r, &v) {
		return
	}
	if len(v.Password) < 12 || len(v.Password) > 72 {
		fail(w, 422, "Kata sandi baru harus 12–72 byte.")
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	var hash string
	if e := a.db.QueryRow("SELECT password_hash FROM users WHERE username=?", s.Username).Scan(&hash); e != nil {
		internal(w, e)
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(v.Current)) != nil {
		fail(w, 403, "Kata sandi saat ini tidak cocok.")
		return
	}
	b, e := bcrypt.GenerateFromPassword([]byte(v.Password), bcrypt.DefaultCost)
	if e != nil {
		internal(w, e)
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("UPDATE users SET password_hash=? WHERE username=?", string(b), s.Username); e != nil {
		internal(w, e)
		return
	}
	if e = audit(tx, s.Username, "CHANGE_PASSWORD", "Kata sandi diperbarui"); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w, e)
		return
	}
	a.mu.Lock()
	for k, v := range a.sessions {
		if v.Username == s.Username {
			delete(a.sessions, k)
		}
	}
	a.mu.Unlock()
	jsonOut(w, 200, map[string]bool{"ok": true})
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request, s Session) {
	var v struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if !decode(w, r, &v) {
		return
	}
	v.Username = strings.TrimSpace(v.Username)
	if len(v.Username) < 3 || len(v.Username) > 100 || len(v.Password) < 12 || len(v.Password) > 72 || (v.Role != "admin" && v.Role != "viewer") {
		fail(w, 422, "Isi nama pengguna (3–100 karakter), kata sandi (12–72 byte), dan peran yang valid.")
		return
	}
	b, e := bcrypt.GenerateFromPassword([]byte(v.Password), bcrypt.DefaultCost)
	if e != nil {
		internal(w, e)
		return
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	tx, e := a.db.Begin()
	if e != nil {
		internal(w, e)
		return
	}
	defer tx.Rollback()
	if _, e = tx.Exec("INSERT INTO users(username,password_hash,role) VALUES(?,?,?)", v.Username, string(b), v.Role); e != nil {
		internal(w, e)
		return
	}
	if e = audit(tx, s.Username, "CREATE_USER", v.Username+" / "+v.Role); e != nil {
		internal(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 201, map[string]bool{"ok": true})
}
