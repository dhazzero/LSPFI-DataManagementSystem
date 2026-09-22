package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

var masterCategories = map[string]bool{"SKEMA": true, "PENDIDIKAN": true, "PEKERJAAN": true, "PROVINSI": true, "KABUPATEN": true, "SUMBER_ANGGARAN": true, "KEMENTERIAN": true}

type masterError struct {
	status  int
	message string
}

func (e *masterError) Error() string                 { return e.message }
func masterProblem(status int, message string) error { return &masterError{status, message} }
func masterFailure(w http.ResponseWriter, err error) {
	var problem *masterError
	if errors.As(err, &problem) {
		fail(w, problem.status, problem.message)
		return
	}
	internal(w, err)
}

func normalizeMaster(m *Master) error {
	m.Category = strings.TrimSpace(m.Category)
	m.Code = strings.TrimSpace(m.Code)
	m.Label = strings.TrimSpace(m.Label)
	m.Parent = strings.TrimSpace(m.Parent)
	if m.ID < 0 || !masterCategories[m.Category] || m.Code == "" || m.Label == "" || utf8.RuneCountInString(m.Code) > 100 || utf8.RuneCountInString(m.Label) > 255 || utf8.RuneCountInString(m.Parent) > 100 {
		return masterProblem(422, "Kategori, kode, dan nama referensi wajib diisi. Batas kode 100 dan nama 255 karakter.")
	}
	if m.Category == "KABUPATEN" && m.Parent == "" {
		return masterProblem(422, "Pilih provinsi induk untuk kabupaten/kota.")
	}
	if m.Category == "PROVINSI" && m.Parent != "" {
		return masterProblem(422, "Provinsi tidak memiliki kode induk.")
	}
	return nil
}

func masterTableExists(tx *sql.Tx, table string) (bool, error) {
	var count int
	err := tx.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&count)
	return count > 0, err
}

// Fixed, internal table/column names only. Legacy snapshots can omit optional columns.
func masterLegacyUse(tx *sql.Tx, table, column, code, label string) (bool, error) {
	var count int
	if err := tx.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name=? AND column_name=?", table, column).Scan(&count); err != nil {
		return false, err
	}
	if count == 0 {
		return false, nil
	}
	err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM `"+table+"` WHERE CAST(`"+column+"` AS CHAR) IN (?,?))", code, label).Scan(&count)
	return count > 0, err
}

// Codes already referenced elsewhere stay stable. Labels remain editable.
func masterInUse(tx *sql.Tx, m Master) (bool, error) {
	for _, field := range fields {
		if field.Kind != "master" || field.Category != m.Category {
			continue
		}
		var used bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM assessments WHERE JSON_UNQUOTE(JSON_EXTRACT(fields,?))=?)", "$."+field.Key, m.Code).Scan(&used); err != nil {
			return false, err
		}
		if used {
			return true, nil
		}
	}
	if m.Category == "PROVINSI" {
		var used bool
		if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM master WHERE category='KABUPATEN' AND parent_code=?)", m.Code).Scan(&used); err != nil {
			return false, err
		}
		if used {
			return true, nil
		}
		exists, err := masterTableExists(tx, "rw_parameterbnsp")
		if err != nil {
			return false, err
		}
		if exists {
			if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM rw_parameterbnsp WHERE kategori='KABUPATEN' AND parent_kode=?)", m.Code).Scan(&used); err != nil {
				return false, err
			}
			if used {
				return true, nil
			}
		}
	}
	checks := map[string][][2]string{
		"SKEMA":           {{"rw_skema", "id"}, {"rw_skema", "kode_skema"}, {"registration_sequences", "scheme"}, {"certificate_sequences", "scheme"}, {"rw_pendaftaran", "skema_id"}},
		"PROVINSI":        {{"rw_asesiprofile", "provinsi"}},
		"KABUPATEN":       {{"rw_asesiprofile", "kota_kabupaten"}},
		"PENDIDIKAN":      {{"rw_asesiprofile", "kualifikasi_pendidikan"}},
		"PEKERJAAN":       {{"rw_asesiprofile", "status_pekerjaan"}, {"rw_asesiprofile", "jabatan"}},
		"SUMBER_ANGGARAN": {{"rw_pendaftaran", "sumber_anggaran"}},
		"KEMENTERIAN":     {{"rw_pendaftaran", "kementerian"}},
	}
	for _, check := range checks[m.Category] {
		used, err := masterLegacyUse(tx, check[0], check[1], m.Code, m.Label)
		if err != nil || used {
			return used, err
		}
	}
	return false, nil
}

func syncMaster(tx *sql.Tx, old, next Master) error {
	if next.Category == "SKEMA" {
		exists, err := masterTableExists(tx, "rw_skema")
		if err != nil || !exists {
			return err
		}
		// Imported master codes are RegisterWeb IDs, not necessarily kode_skema.
		var id int64
		err = tx.QueryRow("SELECT id FROM rw_skema WHERE CAST(id AS CHAR)=? OR kode_skema=? ORDER BY (CAST(id AS CHAR)=?) DESC LIMIT 1", next.Code, next.Code, next.Code).Scan(&id)
		if err == sql.ErrNoRows {
			return nil
		}
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE rw_skema SET nama_skema=?,updatedAt=NOW() WHERE id=?", next.Label, id)
		return err
	}
	exists, err := masterTableExists(tx, "rw_parameterbnsp")
	if err != nil || !exists {
		return err
	}
	var labelLimit int
	if err = tx.QueryRow("SELECT character_maximum_length FROM information_schema.columns WHERE table_schema=DATABASE() AND table_name='rw_parameterbnsp' AND column_name='label'").Scan(&labelLimit); err != nil {
		return err
	}
	if utf8.RuneCountInString(next.Label) > labelLimit {
		return masterProblem(422, fmt.Sprintf("Nama referensi maksimal %d karakter agar sesuai tabel RegisterWeb.", labelLimit))
	}
	key := next
	if old.ID > 0 {
		key = old
	}
	var count int
	if err = tx.QueryRow("SELECT COUNT(*) FROM rw_parameterbnsp WHERE kategori=? AND kode=?", key.Category, key.Code).Scan(&count); err != nil {
		return err
	}
	if count > 1 {
		return masterProblem(409, "Kode referensi ganda ditemukan pada salinan RegisterWeb. Rapikan duplikat sebelum menyimpan.")
	}
	if old.ID > 0 && old.Code != next.Code {
		var collision int
		if err = tx.QueryRow("SELECT COUNT(*) FROM rw_parameterbnsp WHERE kategori=? AND kode=?", next.Category, next.Code).Scan(&collision); err != nil {
			return err
		}
		if collision > 0 {
			return masterProblem(409, "Kode tujuan sudah digunakan pada salinan RegisterWeb.")
		}
	}
	if count == 0 {
		_, err = tx.Exec("INSERT INTO rw_parameterbnsp(kategori,kode,label,parent_kode,createdAt,updatedAt) VALUES(?,?,?,?,NOW(),NOW())", next.Category, next.Code, next.Label, next.Parent)
	} else {
		_, err = tx.Exec("UPDATE rw_parameterbnsp SET kode=?,label=?,parent_kode=?,updatedAt=NOW() WHERE kategori=? AND kode=?", next.Code, next.Label, next.Parent, key.Category, key.Code)
	}
	return err
}

func (a *App) storeMaster(m Master, username string) (int64, error) {
	if err := normalizeMaster(&m); err != nil {
		return 0, err
	}
	a.writes.Lock()
	defer a.writes.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var old Master
	if m.ID > 0 {
		old.ID = m.ID
		err = tx.QueryRow("SELECT category,code,label,parent_code FROM master WHERE id=? FOR UPDATE", m.ID).Scan(&old.Category, &old.Code, &old.Label, &old.Parent)
		if err == sql.ErrNoRows {
			return 0, masterProblem(404, "Data referensi tidak ditemukan.")
		}
		if err != nil {
			return 0, err
		}
		if old.Category != m.Category {
			return 0, masterProblem(422, "Kategori referensi tidak dapat diubah. Tambahkan referensi pada kategori yang sesuai.")
		}
		if old.Code != m.Code || old.Parent != m.Parent {
			used, e := masterInUse(tx, old)
			if e != nil {
				return 0, e
			}
			if used {
				return 0, masterProblem(409, "Kode atau induk tidak dapat diubah karena referensi sudah digunakan. Nama referensi tetap dapat diedit.")
			}
		}
	}
	var duplicate bool
	if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM master WHERE category=? AND code=? AND id<>?)", m.Category, m.Code, m.ID).Scan(&duplicate); err != nil {
		return 0, err
	}
	if duplicate {
		return 0, masterProblem(409, "Kode sudah digunakan pada kategori ini. Gunakan tombol Edit pada referensi yang ada.")
	}
	if m.Category == "KABUPATEN" {
		var parent bool
		if err = tx.QueryRow("SELECT EXISTS(SELECT 1 FROM master WHERE category='PROVINSI' AND code=?)", m.Parent).Scan(&parent); err != nil {
			return 0, err
		}
		if !parent {
			return 0, masterProblem(422, "Provinsi induk tidak ditemukan. Tambahkan provinsi terlebih dahulu.")
		}
	}
	action := "UPDATE_MASTER"
	if m.ID == 0 {
		action = "CREATE_MASTER"
		result, e := tx.Exec("INSERT INTO master(category,code,label,parent_code) VALUES(?,?,?,?)", m.Category, m.Code, m.Label, m.Parent)
		if e != nil {
			return 0, e
		}
		m.ID, err = result.LastInsertId()
	} else {
		_, err = tx.Exec("UPDATE master SET code=?,label=?,parent_code=? WHERE id=?", m.Code, m.Label, m.Parent, m.ID)
	}
	if err != nil {
		return 0, err
	}
	if err = syncMaster(tx, old, m); err != nil {
		return 0, err
	}
	if err = audit(tx, username, action, fmt.Sprintf("%s / %s (ID: %d)", m.Category, m.Code, m.ID)); err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return m.ID, nil
}

func (a *App) saveMaster(w http.ResponseWriter, r *http.Request, s Session) {
	var m Master
	if !decode(w, r, &m) {
		return
	}
	id, err := a.storeMaster(m, s.Username)
	if err != nil {
		masterFailure(w, err)
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "id": id})
}

func (a *App) removeMaster(id int64, username string) error {
	a.writes.Lock()
	defer a.writes.Unlock()
	tx, err := a.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	m := Master{ID: id}
	err = tx.QueryRow("SELECT category,code,label,parent_code FROM master WHERE id=? FOR UPDATE", id).Scan(&m.Category, &m.Code, &m.Label, &m.Parent)
	if err == sql.ErrNoRows {
		return masterProblem(404, "Data referensi tidak ditemukan.")
	}
	if err != nil {
		return err
	}
	used, err := masterInUse(tx, m)
	if err != nil {
		return err
	}
	if used {
		return masterProblem(409, "Referensi masih digunakan oleh asesmen, kabupaten/kota, penomoran, atau data RegisterWeb sehingga tidak dapat dihapus.")
	}
	if _, err = tx.Exec("DELETE FROM master WHERE id=?", id); err != nil {
		return err
	}
	exists, err := masterTableExists(tx, "rw_parameterbnsp")
	if err != nil {
		return err
	}
	if exists && m.Category != "SKEMA" {
		if _, err = tx.Exec("DELETE FROM rw_parameterbnsp WHERE kategori=? AND kode=?", m.Category, m.Code); err != nil {
			return err
		}
	}
	if err = audit(tx, username, "DELETE_MASTER", fmt.Sprintf("%s / %s (ID: %d)", m.Category, m.Code, m.ID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (a *App) deleteMaster(w http.ResponseWriter, r *http.Request, s Session) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		fail(w, 400, "ID referensi tidak valid.")
		return
	}
	if err = a.removeMaster(id, s.Username); err != nil {
		masterFailure(w, err)
		return
	}
	jsonOut(w, 200, map[string]bool{"ok": true})
}
