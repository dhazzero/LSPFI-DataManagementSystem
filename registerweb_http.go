package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

func secretColumn(name string) bool {
	n := strings.ToLower(name)
	return strings.Contains(n, "password") || strings.Contains(n, "secret") || strings.Contains(n, "token")
}

func (a *App) legacyCandidate(id int64) (Record, error) {
	registration, e := a.legacyTable("pendaftaran")
	if e != nil {
		return Record{}, e
	}
	rows, e := a.legacyRows(registration, " WHERE id=?", []any{id}, 1, 0, "", "")
	if e != nil {
		return Record{}, e
	}
	if len(rows) != 1 {
		return Record{}, sql.ErrNoRows
	}
	p := rows[0]
	profileTable, e := a.legacyTable("asesiprofile")
	if e != nil {
		return Record{}, e
	}
	profile := map[string]*string{}
	if p["user_id"] != nil {
		rows, e = a.legacyRows(profileTable, " WHERE user_id=?", []any{*p["user_id"]}, 1, 0, "", "")
		if e != nil {
			return Record{}, e
		}
		if len(rows) > 0 {
			profile = rows[0]
		}
	}
	f := map[string]string{}
	profileMap := map[string]string{"name": "nama_lengkap", "nik": "nik", "birth_place": "tempat_lahir", "birth_date": "tanggal_lahir", "gender": "jenis_kelamin", "address": "alamat_rumah", "city": "kota_kabupaten", "province": "provinsi", "phone": "telp_hp", "email": "email_pribadi", "education": "kualifikasi_pendidikan", "occupation": "jabatan", "company": "nama_institusi", "met_year": "tahun_met"}
	registrationMap := map[string]string{"blanko": "no_blanko_sertifikat", "certificate": "no_sertifikat", "certificate_year": "tahun_sertifikat", "registration": "nomor_registrasi", "plenary_date": "tanggal_pleno", "tuk": "nama_tuk", "scheme": "skema_id", "test_date": "tanggal_uji", "schedule": "kode_jadwal", "assessor_registration": "no_reg_asesor", "funding": "sumber_anggaran", "ministry": "kementerian", "result": "keputusan_asesmen", "assessor": "nama_asesor"}
	for to, from := range profileMap {
		if profile[from] != nil {
			f[to] = *profile[from]
		}
	}
	for to, from := range registrationMap {
		if p[from] != nil {
			f[to] = *p[from]
		}
	}
	if created := p["createdAt"]; created != nil && len(*created) >= 4 {
		if year, e := strconv.Atoi((*created)[:4]); e == nil && year >= 1900 && year <= 2100 {
			f["registration_year"] = strconv.Itoa(year)
		}
	}
	for _, key := range []string{"birth_date", "test_date", "plenary_date"} {
		if len(f[key]) >= 10 {
			f[key] = f[key][:10]
		}
	}
	f["name_upper"] = strings.ToUpper(f["name"])
	catalog, e := a.legacyCatalog()
	if e != nil {
		return Record{}, e
	}
	source := fmt.Sprintf("RegisterWeb:%s:pendaftaran:%d", catalog.Source, id)
	var existing int64
	e = a.db.QueryRow("SELECT id FROM assessments WHERE source=? LIMIT 1", source).Scan(&existing)
	if e != nil && e != sql.ErrNoRows {
		return Record{}, e
	}
	return Record{ID: existing, LegacyID: id, Fields: f, Source: source}, nil
}
func (a *App) legacyCandidateHandler(w http.ResponseWriter, r *http.Request, s Session) {
	id, e := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if e != nil || id < 1 {
		fail(w, 400, "ID pendaftaran tidak valid.")
		return
	}
	candidate, e := a.legacyCandidate(id)
	if e == sql.ErrNoRows {
		fail(w, 404, "Pendaftaran sumber tidak ditemukan.")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, candidate)
}
func (a *App) legacyTables(w http.ResponseWriter, r *http.Request, s Session) {
	c, e := a.legacyCatalog()
	if e == sql.ErrNoRows {
		jsonOut(w, 200, LegacyCatalog{Tables: []LegacyTable{}})
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, c)
}
func (a *App) legacyTable(name string) (LegacyTable, error) {
	c, e := a.legacyCatalog()
	if e != nil {
		return LegacyTable{}, e
	}
	for _, t := range c.Tables {
		if t.Name == name {
			return t, nil
		}
	}
	return LegacyTable{}, sql.ErrNoRows
}
func legacyWhere(t LegacyTable, q, column, value string) (string, []any, error) {
	where := " WHERE 1=1"
	args := []any{}
	if q != "" {
		parts := []string{}
		for _, c := range t.Columns {
			if !secretColumn(c.Name) {
				parts = append(parts, "CAST("+quoteID(c.Name)+" AS CHAR) LIKE ?")
				args = append(args, "%"+q+"%")
			}
		}
		where += " AND (" + strings.Join(parts, " OR ") + ")"
	}
	if column != "" {
		found := false
		for _, c := range t.Columns {
			if c.Name == column && !secretColumn(c.Name) {
				found = true
			}
		}
		if !found {
			return "", nil, fmt.Errorf("kolom filter tidak tersedia")
		}
		where += " AND " + quoteID(column) + "=?"
		args = append(args, value)
	}
	return where, args, nil
}
func legacySort(t LegacyTable, sortBy, sortDir string) (string, string) {
	dir := strings.ToUpper(sortDir)
	if dir != "DESC" {
		dir = "ASC"
	}
	if sortBy == "" {
		return "", dir
	}
	for _, c := range t.Columns {
		if c.Name == sortBy {
			return c.Name, dir
		}
	}
	return "", dir
}
func (a *App) legacyRows(t LegacyTable, where string, args []any, limit, offset int, sortBy, sortDir string) ([]map[string]*string, error) {
	cols := []string{}
	for _, c := range t.Columns {
		if secretColumn(c.Name) {
			cols = append(cols, "NULL")
		} else {
			cols = append(cols, "CAST("+quoteID(c.Name)+" AS CHAR)")
		}
	}
	query := "SELECT " + strings.Join(cols, ",") + " FROM " + quoteID(t.Target) + where
	if sortBy != "" {
		query += " ORDER BY " + quoteID(sortBy) + " " + sortDir
	} else {
		primary := []string{}
		for _, idx := range t.Indexes {
			if idx.Name == "PRIMARY" {
				primary = append(primary, quoteID(idx.Column))
			}
		}
		if len(primary) > 0 {
			query += " ORDER BY " + strings.Join(primary, ",")
		}
	}
	if limit > 0 {
		query += " LIMIT ? OFFSET ?"
		args = append(args, limit, offset)
	}
	rows, e := a.db.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	result := []map[string]*string{}
	for rows.Next() {
		cells := make([]*string, len(cols))
		dest := make([]any, len(cols))
		for i := range cells {
			dest[i] = &cells[i]
		}
		if e = rows.Scan(dest...); e != nil {
			return nil, e
		}
		row := map[string]*string{}
		for i, c := range t.Columns {
			if secretColumn(c.Name) {
				redacted := "[dilindungi]"
				row[c.Name] = &redacted
			} else {
				row[c.Name] = cells[i]
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}
func (a *App) legacyData(w http.ResponseWriter, r *http.Request, s Session) {
	t, e := a.legacyTable(r.PathValue("table"))
	if e == sql.ErrNoRows {
		fail(w, 404, "Tabel salinan tidak ditemukan.")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	query := r.URL.Query()
	where, args, e := legacyWhere(t, query.Get("q"), query.Get("column"), query.Get("value"))
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	var count int
	if e = a.db.QueryRow("SELECT COUNT(*) FROM "+quoteID(t.Target)+where, args...).Scan(&count); e != nil {
		internal(w, e)
		return
	}
	page, _ := strconv.Atoi(query.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > max(1, (count+49)/50) {
		page = max(1, (count+49)/50)
	}
	sortBy, sortDir := legacySort(t, query.Get("sort"), query.Get("dir"))
	rows, e := a.legacyRows(t, where, args, 50, (page-1)*50, sortBy, sortDir)
	if e != nil {
		internal(w, e)
		return
	}
	jsonOut(w, 200, map[string]any{"table": t, "rows": rows, "total": count, "page": page, "page_size": 50, "sort": sortBy, "dir": sortDir})
}
func (a *App) legacyExport(w http.ResponseWriter, r *http.Request, s Session) {
	t, e := a.legacyTable(r.PathValue("table"))
	if e == sql.ErrNoRows {
		fail(w, 404, "Tabel salinan tidak ditemukan.")
		return
	}
	if e != nil {
		internal(w, e)
		return
	}
	q := r.URL.Query()
	where, args, e := legacyWhere(t, q.Get("q"), q.Get("column"), q.Get("value"))
	if e != nil {
		fail(w, 400, e.Error())
		return
	}
	sortBy, sortDir := legacySort(t, q.Get("sort"), q.Get("dir"))
	rows, e := a.legacyRows(t, where, args, 100001, 0, sortBy, sortDir)
	if e != nil {
		internal(w, e)
		return
	}
	if len(rows) > 100000 {
		fail(w, 422, "Ekspor maksimal 100.000 baris. Persempit filter atau gunakan cadangan lengkap.")
		return
	}
	f := excelize.NewFile()
	defer f.Close()
	header := []any{}
	columns := []string{}
	for _, c := range t.Columns {
		if !secretColumn(c.Name) {
			header = append(header, c.Name)
			columns = append(columns, c.Name)
		}
	}
	if e = f.SetSheetRow("Sheet1", "A1", &header); e != nil {
		internal(w, e)
		return
	}
	for i, row := range rows {
		values := []any{}
		for _, c := range columns {
			value := ""
			if row[c] != nil {
				value = *row[c]
			}
			values = append(values, value)
		}
		if e = f.SetSheetRow("Sheet1", fmt.Sprintf("A%d", i+2), &values); e != nil {
			internal(w, e)
			return
		}
	}
	last, _ := excelize.ColumnNumberToName(len(columns))
	style, e := f.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		internal(w, e)
		return
	}
	if e = f.SetColStyle("Sheet1", "A:"+last, style); e != nil {
		internal(w, e)
		return
	}
	f.SetColWidth("Sheet1", "A", last, 24)
	b, e := f.WriteToBuffer()
	if e != nil {
		internal(w, e)
		return
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="RegisterWeb-%s.xlsx"`, t.Name))
	w.Write(b.Bytes())
}
