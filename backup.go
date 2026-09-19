package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var backupTables = []struct {
	Name    string
	Columns []string
}{
	{"users", []string{"id", "username", "password_hash", "role", "created_at"}},
	{"master", []string{"id", "category", "code", "label", "parent_code"}},
	{"assessments", []string{"id", "identity_key", "nik", "name", "scheme", "registration", "certificate", "test_date", "result", "fields", "source", "version", "created_at", "updated_at"}},
	{"documents", []string{"id", "assessment_id", "name", "category", "mime", "size", "sha256", "created_at"}},
	{"audit", []string{"id", "username", "action", "detail", "created_at"}},
}

type Backup struct {
	Format  string                 `json:"format"`
	Created string                 `json:"created"`
	Tables  map[string][][]*string `json:"tables"`
	Files   map[string]string      `json:"files"`
}

var backupPath = regexp.MustCompile(`^(documents/[a-f0-9]{32}|imports/[a-f0-9]{32}\.(xlsx|csv))$`)

func (a *App) writeBackup(path string) error {
	snapshot := Backup{Format: "lspfi-dms-v1", Created: time.Now().UTC().Format(time.RFC3339), Tables: map[string][][]*string{}, Files: map[string]string{}}
	tx, e := a.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, table := range backupTables {
		columns := []string{}
		for _, c := range table.Columns {
			columns = append(columns, "CAST(`"+c+"` AS CHAR)")
		}
		rows, e := tx.Query("SELECT " + strings.Join(columns, ",") + " FROM `" + table.Name + "` ORDER BY id")
		if e != nil {
			return e
		}
		data := [][]*string{}
		for rows.Next() {
			values := make([]*string, len(columns))
			dest := make([]any, len(columns))
			for i := range dest {
				dest[i] = &values[i]
			}
			if e = rows.Scan(dest...); e != nil {
				rows.Close()
				return e
			}
			data = append(data, values)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		snapshot.Tables[table.Name] = data
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	file, e := os.Create(path)
	if e != nil {
		return e
	}
	defer file.Close()
	zw := zip.NewWriter(file)
	defer zw.Close()
	// A missing referenced scan makes the backup fail rather than silently incomplete.
	for _, row := range snapshot.Tables["documents"] {
		if row[0] == nil || row[6] == nil {
			return errors.New("metadata dokumen tidak valid")
		}
		pathName := "documents/" + *row[0]
		if !backupPath.MatchString(pathName) {
			return errors.New("ID dokumen tidak valid")
		}
		b, e := os.ReadFile(filepath.Join(a.cfg.Storage, filepath.FromSlash(pathName)))
		if e != nil {
			return e
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != *row[6] {
			return errors.New("checksum dokumen berbeda; backup dibatalkan")
		}
		snapshot.Files[pathName] = *row[6]
	}
	for _, folder := range []string{"documents", "imports"} {
		entries, e := os.ReadDir(filepath.Join(a.cfg.Storage, folder))
		if e != nil {
			return e
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			name := folder + "/" + entry.Name()
			if !backupPath.MatchString(name) {
				continue
			}
			in, e := os.Open(filepath.Join(a.cfg.Storage, folder, entry.Name()))
			if e != nil {
				return e
			}
			out, e := zw.Create(name)
			if e != nil {
				in.Close()
				return e
			}
			hash := sha256.New()
			_, e = io.Copy(io.MultiWriter(out, hash), in)
			in.Close()
			if e != nil {
				return e
			}
			snapshot.Files[name] = hex.EncodeToString(hash.Sum(nil))
		}
	}
	out, e := zw.Create("manifest.json")
	if e != nil {
		return e
	}
	if e = json.NewEncoder(out).Encode(snapshot); e != nil {
		return e
	}
	if e = zw.Close(); e != nil {
		return e
	}
	return file.Close()
}
func (a *App) backup(w http.ResponseWriter, r *http.Request, s Session) {
	a.writes.Lock()
	defer a.writes.Unlock()
	temp, e := os.CreateTemp(a.cfg.Storage, "backup-*.zip")
	if e != nil {
		internal(w, e)
		return
	}
	path := temp.Name()
	temp.Close()
	defer os.Remove(path)
	if e = a.writeBackup(path); e != nil {
		internal(w, e)
		return
	}
	if _, e = a.db.Exec("INSERT INTO audit(username,action,detail) VALUES(?,'BACKUP','Cadangan ZIP diunduh')", s.Username); e != nil {
		internal(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="LSPFI-Arsip-%s.zip"`, time.Now().Format("20060102-150405")))
	http.ServeFile(w, r, path)
}
func (a *App) restore(path string) error {
	// Restore only to an empty schema: never replace an existing archive or users.
	for _, table := range backupTables {
		var count int
		if e := a.db.QueryRow("SELECT COUNT(*) FROM `" + table.Name + "`").Scan(&count); e != nil {
			return e
		}
		if count > 0 {
			return errors.New("pemulihan memerlukan database lspfi_dms kosong, termasuk akun dan master; gunakan komputer/server MySQL pemulihan terpisah")
		}
	}
	for _, folder := range []string{"documents", "imports"} {
		entries, e := os.ReadDir(filepath.Join(a.cfg.Storage, folder))
		if e != nil {
			return e
		}
		if len(entries) > 0 {
			return errors.New("folder penyimpanan pemulihan harus kosong")
		}
	}
	z, e := zip.OpenReader(path)
	if e != nil {
		return e
	}
	defer z.Close()
	var manifest Backup
	entries := map[string]*zip.File{}
	var total uint64
	for _, f := range z.File {
		if entries[f.Name] != nil {
			return errors.New("entri backup ganda")
		}
		if f.Name != "manifest.json" && !backupPath.MatchString(f.Name) {
			return errors.New("path backup tidak diizinkan")
		}
		entries[f.Name] = f
		total += f.UncompressedSize64
		if total > 2<<30 || len(entries) > 100000 {
			return errors.New("backup melebihi batas pemulihan 2 GB / 100.000 file")
		}
	}
	mf := entries["manifest.json"]
	if mf == nil || mf.UncompressedSize64 > 128<<20 {
		return errors.New("manifest backup tidak valid")
	}
	reader, e := mf.Open()
	if e != nil {
		return e
	}
	decoder := json.NewDecoder(io.LimitReader(reader, 128<<20))
	e = decoder.Decode(&manifest)
	reader.Close()
	if e != nil {
		return e
	}
	if manifest.Format != "lspfi-dms-v1" {
		return errors.New("versi backup tidak sesuai")
	}
	if len(entries) != len(manifest.Files)+1 {
		return errors.New("isi backup tidak sesuai manifest")
	}
	staging, e := os.MkdirTemp(a.cfg.Storage, "restore-")
	if e != nil {
		return e
	}
	defer os.RemoveAll(staging)
	for name, checksum := range manifest.Files {
		if !backupPath.MatchString(name) {
			return errors.New("path berkas tidak aman")
		}
		f := entries[name]
		if f == nil || f.UncompressedSize64 > 20<<20 {
			return errors.New("berkas hilang atau melebihi 20 MB")
		}
		reader, e := f.Open()
		if e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(reader, (20<<20)+1))
		reader.Close()
		if e != nil {
			return e
		}
		if len(b) > 20<<20 {
			return errors.New("berkas terlalu besar")
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != checksum {
			return errors.New("checksum backup tidak cocok")
		}
		target := filepath.Join(staging, filepath.FromSlash(name))
		if e = os.MkdirAll(filepath.Dir(target), 0700); e != nil {
			return e
		}
		if e = os.WriteFile(target, b, 0600); e != nil {
			return e
		}
	}
	for _, row := range manifest.Tables["documents"] {
		if len(row) != 8 || row[0] == nil || row[6] == nil || manifest.Files["documents/"+*row[0]] != *row[6] {
			return errors.New("referensi dokumen tidak lengkap")
		}
	}
	tx, e := a.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	if len(manifest.Tables) != len(backupTables) {
		return errors.New("tabel backup tidak lengkap")
	}
	for _, table := range backupTables {
		data, ok := manifest.Tables[table.Name]
		if !ok {
			return errors.New("tabel backup hilang")
		}
		marks := strings.TrimSuffix(strings.Repeat("?,", len(table.Columns)), ",")
		query := "INSERT INTO `" + table.Name + "` (`" + strings.Join(table.Columns, "`,`") + "`) VALUES(" + marks + ")"
		for _, row := range data {
			if len(row) != len(table.Columns) {
				return errors.New("kolom backup tidak sesuai")
			}
			args := make([]any, len(row))
			for i, v := range row {
				if v != nil {
					args[i] = *v
				}
			}
			if _, e = tx.Exec(query, args...); e != nil {
				return e
			}
		}
	}
	moved := []string{}
	committed := false
	defer func() {
		if !committed {
			for _, p := range moved {
				os.Remove(p)
			}
		}
	}()
	for name := range manifest.Files {
		target := filepath.Join(a.cfg.Storage, filepath.FromSlash(name))
		if e = os.Rename(filepath.Join(staging, filepath.FromSlash(name)), target); e != nil {
			return e
		}
		moved = append(moved, target)
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	committed = true
	return nil
}
