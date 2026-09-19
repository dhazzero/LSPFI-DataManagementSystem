package main

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
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
	{"registration_sequences", []string{"id", "scheme", "year", "last_seq"}},
	{"assessments", []string{"id", "identity_key", "nik", "name", "scheme", "registration", "certificate", "test_date", "result", "fields", "source", "version", "created_at", "updated_at"}},
	{"documents", []string{"id", "assessment_id", "name", "category", "mime", "size", "sha256", "created_at"}},
	{"audit", []string{"id", "username", "action", "detail", "created_at"}},
}

type Backup struct {
	Format      string                 `json:"format"`
	Scope       string                 `json:"scope,omitempty"`
	Created     string                 `json:"created"`
	Tables      map[string][][]*string `json:"tables"`
	Files       map[string]string      `json:"files"`
	RegisterWeb *LegacySnapshot        `json:"registerweb,omitempty"`
}

var backupPath = regexp.MustCompile(`^(documents/[a-f0-9]{32}|imports/[a-f0-9]{32}\.(xlsx|csv))$`)

func (a *App) writeBackup(path string) error {
	snapshot := Backup{Format: "lspfi-dms-v2", Scope: referenceBackupScope, Created: time.Now().UTC().Format(time.RFC3339), Tables: map[string][][]*string{}, Files: map[string]string{}}
	tx, e := a.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	for _, table := range backupTables {
		if table.Name != "users" && table.Name != "master" && table.Name != "registration_sequences" {
			snapshot.Tables[table.Name] = [][]*string{}
			continue
		}
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
	if e = preserveRegistrationFloors(tx, &snapshot); e != nil {
		return e
	}
	var catalogBytes []byte
	if e = tx.QueryRow("SELECT catalog FROM registerweb_snapshot WHERE id=1").Scan(&catalogBytes); e == nil {
		var catalog LegacyCatalog
		if e = json.Unmarshal(catalogBytes, &catalog); e != nil {
			return e
		}
		legacy := LegacySnapshot{Catalog: catalog, Rows: map[string][][]*string{}}
		for i := range legacy.Catalog.Tables {
			t := &legacy.Catalog.Tables[i]
			rows, e := readLegacyRows(context.Background(), tx, *t, t.Target)
			if e != nil {
				return e
			}
			t.Count = int64(len(rows))
			t.Digest = legacyDigest(rows)
			legacy.Rows[t.Name] = rows
		}
		legacy = referenceLegacySnapshot(legacy)
		if e = validateLegacySnapshot(legacy); e != nil {
			return e
		}
		snapshot.RegisterWeb = &legacy
	} else if e != sql.ErrNoRows {
		return e
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
	// Candidate scans and source spreadsheets are deliberately never added.
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
	if _, e = a.db.Exec("INSERT INTO audit(username,action,detail) VALUES(?,'BACKUP','Cadangan tanpa data asesi diunduh')", s.Username); e != nil {
		internal(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="LSPFI-Referensi-%s.zip"`, time.Now().Format("20060102-150405")))
	http.ServeFile(w, r, path)
}
func (a *App) restore(path string) error {
	var snapshots int
	if e := a.db.QueryRow("SELECT COUNT(*) FROM registerweb_snapshot").Scan(&snapshots); e != nil {
		return e
	}
	if snapshots > 0 {
		return errors.New("salinan RegisterWeb sudah ada; pemulihan tidak menimpanya")
	}
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
	if manifest.Format != "lspfi-dms-v1" && manifest.Format != "lspfi-dms-v2" {
		return errors.New("versi backup tidak sesuai")
	}
	if manifest.Format == "lspfi-dms-v2" {
		if e = validateReferenceBackup(manifest); e != nil {
			return e
		}
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
	if manifest.RegisterWeb != nil {
		if e = validateLegacySnapshot(*manifest.RegisterWeb); e != nil {
			return e
		}
		if e = createLegacyTables(context.Background(), a.db, manifest.RegisterWeb.Catalog); e != nil {
			return e
		}
	}
	tx, cleanup, e := beginArchiveTx(context.Background(), a.db)
	if e != nil {
		return e
	}
	defer cleanup()
	// Backups made before automatic numbering have no local counter table.
	if manifest.Tables == nil {
		return errors.New("tabel backup tidak lengkap")
	}
	if _, ok := manifest.Tables["registration_sequences"]; !ok {
		manifest.Tables["registration_sequences"] = [][]*string{}
	}
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
	if manifest.RegisterWeb != nil {
		if e = insertLegacySnapshot(context.Background(), tx, *manifest.RegisterWeb); e != nil {
			return e
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
