package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Master struct {
	ID       int64  `json:"id"`
	Category string `json:"category"`
	Code     string `json:"code"`
	Label    string `json:"label"`
	Parent   string `json:"parent"`
}
type Record struct {
	GenerateRegistration bool              `json:"generate_registration,omitempty"`
	LegacyID             int64             `json:"legacy_id,omitempty"`
	ID                   int64             `json:"id"`
	Fields               map[string]string `json:"fields"`
	Source               string            `json:"source"`
	Version              int               `json:"version"`
	Documents            int               `json:"documents"`
	Updated              string            `json:"updated"`
}
type Field struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Kind     string `json:"kind,omitempty"`
	Category string `json:"category,omitempty"`
}

var fields = []Field{
	{"blanko", "No. Blanko Sertifikat", "", ""}, {"certificate", "Nomor Sertifikat", "", ""}, {"certificate_year", "Tahun Sertifikat", "", ""},
	{"registration", "Nomor Registrasi", "", ""}, {"met_year", "Tahun MET", "", ""}, {"plenary_date", "Tanggal Tanda Tanggal/Rapat Pleno", "date", ""},
	{"tuk", "TUK Asesmen", "", ""}, {"company", "Asal Perusahaan", "", ""}, {"scheme", "Skema Sertifikasi", "master", "SKEMA"},
	{"name_upper", "Nama Upper", "", ""}, {"name", "Nama Proper", "", ""}, {"nik", "NIK", "", ""},
	{"birth_place", "Tempat Lahir", "", ""}, {"birth_date", "Tanggal Lahir (dd/mm/yyyy)", "date", ""}, {"gender", "Jenis Kelamin", "", ""},
	{"address", "Tempat Tinggal", "", ""}, {"city", "Kode Kota", "master", "KABUPATEN"}, {"province", "Kode Provinsi", "master", "PROVINSI"},
	{"phone", "Telp", "", ""}, {"email", "Email", "email", ""}, {"education", "Kode Pendidikan", "master", "PENDIDIKAN"},
	{"occupation", "Kode Pekerjaan", "master", "PEKERJAAN"}, {"schedule", "Kode Jadwal", "", ""}, {"test_date", "TANGGAL UJI (hh/bb/yyyy)", "date", ""},
	{"assessor_registration", "NOMOR REGISTRASI ASESOR", "", ""}, {"funding", "KODE SUMBER ANGGARAN", "master", "SUMBER_ANGGARAN"},
	{"ministry", "KODE KEMENTERIAN", "master", "KEMENTERIAN"}, {"result", "K/BK", "result", ""}, {"assessor", "Asesor Kompetensi", "", ""},
	{"education_label", "Pendidikan Terakhir", "", ""}, {"occupation_label", "Jabatan", "", ""},
}

func normal(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}
func headerKey(s string) string {
	n := normal(s)
	for _, f := range fields {
		if normal(f.Label) == n {
			return f.Key
		}
	}
	aliases := map[string]string{
		"namalengkap": "name", "tanggal lahir": "birth_date", "tanggaluji": "test_date", "tanggalujiddmmyyyy": "test_date",
		"alamatlengkap": "address", "kotakabupatenkode": "city", "kotakabupaten": "city", "provinsikode": "province", "provinsi": "province",
		"nohp": "phone", "emailpribadi": "email", "pendidikanterakhirkode": "education", "jabatankode": "occupation", "namainstansi": "company",
		"sumberanggarankode": "funding", "sumberanggaran": "funding", "namatuk": "tuk", "tanggal tanda tangan rapat pleno": "plenary_date",
		"tanggal lahir dd mm yyyy": "birth_date", "no blanko sertifikat": "blanko",
	}
	for k, v := range aliases {
		if normal(k) == n {
			return v
		}
	}
	return ""
}
func parseDate(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "2006-1-2", "2/1/2006", "02/01/2006", "2-1-2006", "02-01-2006"} {
		if d, e := time.Parse(layout, s); e == nil {
			if d.Year() < 1900 || d.Year() > 2100 {
				return "", errors.New("tahun harus 1900–2100")
			}
			return d.Format("2006-01-02"), nil
		}
	}
	months := []string{"januari", "februari", "maret", "april", "mei", "juni", "juli", "agustus", "september", "oktober", "november", "desember"}
	parts := strings.Fields(strings.TrimSpace(strings.ToLower(s[strings.LastIndex(s, ",")+1:])))
	if len(parts) == 3 {
		for i, m := range months {
			if parts[1] == m {
				return parseDate(fmt.Sprintf("%s/%d/%s", parts[0], i+1, parts[2]))
			}
		}
	}
	return "", errors.New("gunakan dd/mm/yyyy atau yyyy-mm-dd; tanggal harus valid")
}
func (a *App) masters() ([]Master, error) {
	rows, e := a.db.Query("SELECT id,category,code,label,parent_code FROM master ORDER BY category,label")
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Master{}
	for rows.Next() {
		var m Master
		if e = rows.Scan(&m.ID, &m.Category, &m.Code, &m.Label, &m.Parent); e != nil {
			return nil, e
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

var schemeDropdownPattern = regexp.MustCompile(`^(\d+)\s*-\s*.+$`)
var schemePrefixPattern = regexp.MustCompile(`^\d+\s+`)
var schemeFullPattern = regexp.MustCompile(`(?i)^(?:\d+\s+)?Jenjang Kualifikasi\s+(\d+)\s+Bidang Fintech P2P Lending Sub Bidang\s+(.+)$`)

func matchMaster(value, category string, masters []Master) (Master, error) {
	matches := []Master{}
	v := normal(value)
	for _, m := range masters {
		if m.Category != category {
			continue
		}
		ok := m.Code == value || normal(m.Label) == v || value == m.Code+" - "+m.Label
		if category == "SKEMA" {
			// Old workbooks use ID dropdowns, full scheme titles, and short level labels.
			if parts := schemeDropdownPattern.FindStringSubmatch(value); len(parts) == 2 && parts[1] == m.Code {
				ok = true
			}
			for _, alias := range schemeAliases(m.Label) {
				if normal(alias) == v {
					ok = true
				}
			}
		}
		if category == "PENDIDIKAN" {
			for _, alias := range strings.Split(m.Label, "/") {
				if normal(alias) == v {
					ok = true
				}
			}
		}
		if !ok {
			a, e1 := strconv.Atoi(value)
			b, e2 := strconv.Atoi(m.Code)
			ok = e1 == nil && e2 == nil && a == b
		}
		if category == "SKEMA" && ((v == "5ti" && normal(m.Label) == "5eksekutifti") || (v == "5nonti" && normal(m.Label) == "5eksekutifnonti")) {
			ok = true
		}
		if ok {
			matches = append(matches, m)
		}
	}
	if len(matches) != 1 {
		return Master{}, fmt.Errorf("%s: nilai %q tidak ditemukan secara unik pada master", category, value)
	}
	return matches[0], nil
}

func schemeAliases(label string) []string {
	aliases := []string{label}
	withoutPrefix := schemePrefixPattern.ReplaceAllString(label, "")
	aliases = append(aliases, withoutPrefix)
	if parts := schemeFullPattern.FindStringSubmatch(label); len(parts) == 3 {
		level, name := parts[1], strings.TrimSpace(parts[2])
		aliases = append(aliases, level+" "+name)
		if strings.EqualFold(name, "Analis Kredit") {
			aliases = append(aliases, level+" Analisis Kredit")
		}
		if strings.EqualFold(name, "Analisis Kredit") {
			aliases = append(aliases, level+" Analis Kredit")
		}
		n := normal(name)
		if strings.Contains(n, "nonteknologiinformasi") {
			aliases = append(aliases, level+" NON-TI", level+" Non-Teknologi Informasi")
		} else if strings.Contains(n, "teknologiinformasi") {
			aliases = append(aliases, level+" TI", level+" Teknologi Informasi")
		}
	}
	return aliases
}

var nikPattern = regexp.MustCompile(`^\d{16}$`)

func validate(input map[string]string, masters []Master, generate ...bool) (map[string]string, []string) {
	out := map[string]string{}
	issues := []string{}
	if year := strings.TrimSpace(input["registration_year"]); year != "" {
		out["registration_year"] = year
		n, e := strconv.Atoi(year)
		if e != nil || n < 1900 || n > 2100 {
			issues = append(issues, "Tahun registrasi harus 1900–2100")
		}
	}
	for _, f := range fields {
		v := strings.TrimSpace(input[f.Key])
		if v == "-" {
			v = ""
		}
		out[f.Key] = v
		if len(v) > 1000 {
			issues = append(issues, f.Label+": maksimal 1000 karakter")
		}
	}
	if out["name"] == "" {
		out["name"] = out["name_upper"]
	}
	out["name_upper"] = strings.ToUpper(out["name"])
	for key, label := range map[string]string{"name": "Nama", "nik": "NIK", "scheme": "Skema", "email": "Email", "phone": "Telp", "birth_date": "Tanggal lahir"} {
		if out[key] == "" {
			issues = append(issues, label+" wajib diisi")
		}
	}
	if !nikPattern.MatchString(out["nik"]) {
		issues = append(issues, "NIK harus 16 digit dan disimpan sebagai teks di Excel")
	}
	if len(out["name"]) > 255 {
		issues = append(issues, "Nama maksimal 255 karakter")
	}
	for _, k := range []string{"registration", "certificate"} {
		if len(out[k]) > 191 {
			issues = append(issues, "Nomor registrasi/sertifikat maksimal 191 karakter")
		}
	}
	if out["email"] != "" {
		m, e := mail.ParseAddress(out["email"])
		if e != nil || m.Address != out["email"] {
			issues = append(issues, "Email tidak valid")
		}
	}
	for _, f := range fields {
		if f.Kind == "date" {
			d, e := parseDate(out[f.Key])
			if e != nil {
				issues = append(issues, f.Label+": "+e.Error())
			} else {
				out[f.Key] = d
			}
		}
	}
	out["result"] = strings.ToUpper(out["result"])
	if out["result"] != "" && out["result"] != "K" && out["result"] != "BK" {
		issues = append(issues, "Hasil asesmen harus K, BK, atau kosong")
	}
	for _, f := range fields {
		if f.Category == "" {
			continue
		}
		v := out[f.Key]
		labelKey := ""
		if f.Key == "education" {
			labelKey = "education_label"
		}
		if f.Key == "occupation" {
			labelKey = "occupation_label"
		}
		label := out[labelKey]
		if v == "" && label == "" {
			continue
		}
		if v == "" {
			v = label
		}
		m, e := matchMaster(v, f.Category, masters)
		if e != nil {
			issues = append(issues, e.Error())
			continue
		}
		if label != "" {
			byLabel, e := matchMaster(label, f.Category, masters)
			if e != nil || byLabel.Code != m.Code {
				issues = append(issues, f.Label+": kode dan nama tidak cocok dengan master")
			}
		}
		out[f.Key] = m.Code
		if labelKey != "" {
			out[labelKey] = m.Label
		}
	}
	if out["city"] != "" {
		for _, m := range masters {
			if m.Category == "KABUPATEN" && m.Code == out["city"] && m.Parent != "" && m.Parent != out["province"] {
				issues = append(issues, "Kode kota tidak sesuai provinsi")
			}
		}
	}
	if out["registration"] == "" && out["test_date"] == "" && !(len(generate) > 0 && generate[0]) {
		issues = append(issues, "Nomor registrasi atau tanggal uji wajib untuk membedakan riwayat asesmen")
	}
	return out, issues
}
func identity(f map[string]string) string {
	parts := []string{f["scheme"], "registration", strings.ToUpper(f["registration"])}
	if f["registration"] == "" {
		parts = []string{f["scheme"], "assessment", f["nik"], f["test_date"]}
	}
	b, _ := json.Marshal(parts)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}
func saveRecord(tx *sql.Tx, r Record) (int64, error) {
	f := r.Fields
	if e := prepareRegistration(tx, &r); e != nil {
		return 0, e
	}
	b, e := json.Marshal(f)
	if e != nil {
		return 0, e
	}
	args := []any{identity(f), f["nik"], f["name"], f["scheme"], nullable(f["registration"]), nullable(f["certificate"]), f["test_date"], f["result"], string(b), r.Source}
	if r.ID == 0 {
		res, e := tx.Exec("INSERT INTO assessments(identity_key,nik,name,scheme,registration,certificate,test_date,result,fields,source) VALUES(?,?,?,?,?,?,?,?,?,?)", args...)
		if e != nil {
			return 0, e
		}
		return res.LastInsertId()
	}
	args = append(args, r.ID, r.Version)
	res, e := tx.Exec("UPDATE assessments SET identity_key=?,nik=?,name=?,scheme=?,registration=?,certificate=?,test_date=?,result=?,fields=?,source=?,version=version+1 WHERE id=? AND version=?", args...)
	if e != nil {
		return 0, e
	}
	count, e := res.RowsAffected()
	if e != nil {
		return 0, e
	}
	if count != 1 {
		return 0, errors.New("data telah berubah; muat ulang sebelum menyimpan")
	}
	return r.ID, nil
}
func audit(tx *sql.Tx, user, action, detail string) error {
	_, e := tx.Exec("INSERT INTO audit(username,action,detail) VALUES(?,?,?)", user, action, detail)
	return e
}
func (a *App) records(q, scheme, result, year string) ([]Record, error) {
	query := `SELECT a.id,a.fields,a.source,a.version,DATE_FORMAT(a.updated_at,'%Y-%m-%d %H:%i:%s'),(SELECT COUNT(*) FROM documents d WHERE d.assessment_id=a.id) FROM assessments a WHERE 1=1`
	args := []any{}
	if q != "" {
		query += " AND (name LIKE ? OR nik LIKE ? OR registration LIKE ? OR certificate LIKE ? OR JSON_UNQUOTE(JSON_EXTRACT(fields,'$.company')) LIKE ?)"
		for range 5 {
			args = append(args, "%"+q+"%")
		}
	}
	if scheme != "" {
		query += " AND scheme=?"
		args = append(args, scheme)
	}
	if result != "" {
		if result == "pending" {
			query += " AND result=''"
		} else {
			query += " AND result=?"
			args = append(args, result)
		}
	}
	if year != "" {
		query += " AND LEFT(test_date,4)=?"
		args = append(args, year)
	}
	query += " ORDER BY a.id DESC"
	rows, e := a.db.Query(query, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Record{}
	for rows.Next() {
		var r Record
		var b []byte
		if e = rows.Scan(&r.ID, &b, &r.Source, &r.Version, &r.Updated, &r.Documents); e != nil {
			return nil, e
		}
		if e = json.Unmarshal(b, &r.Fields); e != nil {
			return nil, e
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
