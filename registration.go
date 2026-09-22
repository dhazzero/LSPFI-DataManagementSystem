package main

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const maxRegistrationSequence int64 = 2147483647 // RegisterWeb Prisma Int

var registrationPattern = regexp.MustCompile(`^PPL 2605 ([0-9]+)$`)
var errRegistration = errors.New("penomoran registrasi")

// Certificate format from registrasiAsesi.ts:
// `${skema.kode_sektor} ${skema.kode_profesi} ${skema.jenjang} ${certRunningNumber} ${currentYear}`
// e.g. "64911 4210 3 0000045 2026"
var certificatePattern = regexp.MustCompile(`^([0-9]{5})\s+([0-9]{4})\s+([1-9])\s+([0-9]+)\s+([0-9]{4})$`)
var errCertificate = errors.New("penomoran sertifikat")

type SchemeMetadata struct {
	KodeSektor  string
	KodeProfesi string
	Jenjang     int
}

var defaultSchemeMetadata = map[string]SchemeMetadata{
	"24": {"64911", "4210", 3},
	"25": {"64911", "4210", 3},
	"26": {"64911", "4210", 3},
	"27": {"64911", "4210", 4},
	"28": {"64911", "4210", 4},
	"29": {"64911", "4210", 4},
	"30": {"64911", "2510", 5},
	"31": {"64911", "2510", 5},
	"32": {"64911", "2510", 6},
	"33": {"64911", "2510", 6},
	"34": {"64911", "2420", 6},
}

func skemaInfo(db registrationReader, scheme string) (SchemeMetadata, error) {
	scheme = strings.TrimSpace(scheme)
	if db != nil {
		for _, table := range []string{"rw_skema", "Skema", "skema"} {
			var exists int
			if e := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=?", table).Scan(&exists); e != nil {
				return SchemeMetadata{}, e
			}
			if exists == 0 {
				continue
			}
			var meta SchemeMetadata
			e := db.QueryRow(fmt.Sprintf("SELECT COALESCE(kode_sektor,''), COALESCE(kode_profesi,''), COALESCE(jenjang,0) FROM `%s` WHERE CAST(id AS CHAR)=? OR kode_skema=? LIMIT 1", table), scheme, scheme).Scan(&meta.KodeSektor, &meta.KodeProfesi, &meta.Jenjang)
			if e == sql.ErrNoRows {
				continue
			}
			if e != nil {
				return SchemeMetadata{}, e
			}
			if !validSchemeMetadata(meta) {
				return SchemeMetadata{}, fmt.Errorf("%w: metadata skema %s tidak lengkap", errCertificate, scheme)
			}
			return meta, nil
		}
	}
	if meta, ok := defaultSchemeMetadata[scheme]; ok {
		return meta, nil
	}
	return SchemeMetadata{}, fmt.Errorf("%w: metadata skema %s belum tersedia", errCertificate, scheme)
}

var sectorPattern = regexp.MustCompile(`^[0-9]{5}$`)
var professionPattern = regexp.MustCompile(`^[0-9]{4}$`)

func validSchemeMetadata(meta SchemeMetadata) bool {
	return sectorPattern.MatchString(meta.KodeSektor) && professionPattern.MatchString(meta.KodeProfesi) && meta.Jenjang >= 1 && meta.Jenjang <= 9
}

func certificateNumber(meta SchemeMetadata, n int64, year int) string {
	return fmt.Sprintf("%s %s %d %07d %04d", meta.KodeSektor, meta.KodeProfesi, meta.Jenjang, n, year)
}

func certificateSequence(s string) (int64, int) {
	m := certificatePattern.FindStringSubmatch(strings.ToUpper(strings.Join(strings.Fields(s), " ")))
	if m == nil {
		return 0, 0
	}
	n, e := strconv.ParseInt(m[4], 10, 64)
	if e != nil || n > maxRegistrationSequence {
		n = maxRegistrationSequence
	}
	yr, _ := strconv.Atoi(m[5])
	return n, yr
}

func registrationNumber(n int64) string { return fmt.Sprintf("PPL 2605 %05d", n) }
func registrationSequence(s string) int64 {
	m := registrationPattern.FindStringSubmatch(strings.ToUpper(strings.Join(strings.Fields(s), " ")))
	if m == nil {
		return 0
	}
	n, e := strconv.ParseInt(m[1], 10, 64)
	if e != nil || n > maxRegistrationSequence {
		return maxRegistrationSequence
	}
	return n
}
func registrationYear() int { return time.Now().In(time.FixedZone("WIB", 7*60*60)).Year() }

type registrationReader interface {
	Query(string, ...any) (*sql.Rows, error)
	QueryRow(string, ...any) *sql.Row
}

// Read the local snapshot only. The source portal is never modified.
func registrationFloor(db registrationReader, scheme string) (int64, error) {
	var floor int64
	if e := db.QueryRow("SELECT COALESCE(MAX(last_seq),0) FROM registration_sequences WHERE scheme=?", scheme).Scan(&floor); e != nil {
		return 0, e
	}
	queries := []string{"SELECT registration FROM assessments WHERE scheme=? AND registration IS NOT NULL"}
	for _, table := range []string{"rw_registrationsequence", "rw_pendaftaran"} {
		var exists int
		if e := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND table_name=?", table).Scan(&exists); e != nil {
			return 0, e
		}
		if exists == 0 {
			continue
		}
		if table == "rw_registrationsequence" {
			var last int64
			if e := db.QueryRow("SELECT COALESCE(MAX(last_seq),0) FROM rw_registrationsequence WHERE CAST(skema_id AS CHAR)=?", scheme).Scan(&last); e != nil {
				return 0, e
			}
			floor = max(floor, last)
		} else {
			queries = append(queries, "SELECT nomor_registrasi FROM rw_pendaftaran WHERE CAST(skema_id AS CHAR)=? AND nomor_registrasi IS NOT NULL")
		}
	}
	for _, query := range queries {
		rows, e := db.Query(query, scheme)
		if e != nil {
			return 0, e
		}
		for rows.Next() {
			var value string
			if e = rows.Scan(&value); e != nil {
				rows.Close()
				return 0, e
			}
			floor = max(floor, registrationSequence(value))
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return 0, e
		}
	}
	return floor, nil
}

func prepareRegistration(tx *sql.Tx, r *Record) error {
	f := r.Fields
	n := registrationSequence(f["registration"])
	if !r.GenerateRegistration && n == 0 {
		return nil
	}
	// Serialize all years for a scheme, including manual edits and imports.
	var code string
	if e := tx.QueryRow("SELECT code FROM master WHERE category='SKEMA' AND code=? FOR UPDATE", f["scheme"]).Scan(&code); e != nil {
		return e
	}
	year := registrationYear()
	if f["registration_year"] != "" {
		var e error
		year, e = strconv.Atoi(f["registration_year"])
		if e != nil || year < 1900 || year > 2100 {
			return fmt.Errorf("%w: tahun registrasi harus 1900–2100", errRegistration)
		}
	}
	if r.GenerateRegistration {
		if f["registration"] != "" {
			return fmt.Errorf("%w: kosongkan nomor untuk menggunakan penomoran otomatis", errRegistration)
		}
		if r.ID > 0 {
			var old sql.NullString
			if e := tx.QueryRow("SELECT registration FROM assessments WHERE id=? FOR UPDATE", r.ID).Scan(&old); e != nil {
				return e
			}
			if old.String != "" {
				return fmt.Errorf("%w: nomor yang sudah tersimpan tidak boleh dibuat ulang otomatis", errRegistration)
			}
		}
		floor, e := registrationFloor(tx, code)
		if e != nil {
			return e
		}
		// A batch may already have a repeatable-read snapshot. Read current
		// counters under lock so numbers committed since that snapshot count.
		rows, e := tx.Query("SELECT last_seq FROM registration_sequences WHERE scheme=? FOR UPDATE", code)
		if e != nil {
			return e
		}
		for rows.Next() {
			var allocated int64
			if e = rows.Scan(&allocated); e != nil {
				rows.Close()
				return e
			}
			floor = max(floor, allocated)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		if floor >= maxRegistrationSequence {
			return fmt.Errorf("%w: batas urutan RegisterWeb tercapai; periksa pemetaan skema", errRegistration)
		}
		n = floor + 1
		f["registration"], f["registration_year"] = registrationNumber(n), strconv.Itoa(year)
	}
	_, e := tx.Exec("INSERT INTO registration_sequences(scheme,year,last_seq) VALUES(?,?,?) ON DUPLICATE KEY UPDATE last_seq=GREATEST(last_seq,VALUES(last_seq))", code, year, n)
	return e
}

func certificateFloor(db registrationReader) (int64, error) {
	var floor int64
	if e := db.QueryRow("SELECT COALESCE(MAX(last_seq),0) FROM certificate_sequences WHERE scheme='0'").Scan(&floor); e != nil {
		return 0, e
	}
	queries := []string{"SELECT certificate FROM assessments WHERE certificate IS NOT NULL"}
	for _, table := range []string{"rw_certificatesequence", "rw_pendaftaran", "CertificateSequence", "certificatesequence"} {
		var exists int
		if e := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=DATABASE() AND BINARY table_name=?", table).Scan(&exists); e != nil {
			return 0, e
		}
		if exists == 0 {
			continue
		}
		if strings.EqualFold(table, "rw_certificatesequence") || strings.EqualFold(table, "CertificateSequence") {
			var last int64
			if e := db.QueryRow(fmt.Sprintf("SELECT COALESCE(MAX(last_seq),0) FROM %s WHERE skema_id=0", table)).Scan(&last); e != nil {
				return 0, e
			}
			floor = max(floor, last)
		} else {
			queries = append(queries, fmt.Sprintf("SELECT no_sertifikat FROM %s WHERE no_sertifikat IS NOT NULL", table))
		}
	}
	for _, query := range queries {
		rows, e := db.Query(query)
		if e != nil {
			return 0, e
		}
		for rows.Next() {
			var value string
			if e = rows.Scan(&value); e != nil {
				rows.Close()
				return 0, e
			}
			seq, _ := certificateSequence(value)
			floor = max(floor, seq)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return 0, e
		}
	}
	return floor, nil
}

func prepareCertificate(tx *sql.Tx, r *Record) error {
	f := r.Fields
	seq, certYr := certificateSequence(f["certificate"])
	if !r.GenerateCertificate && seq == 0 {
		return nil
	}
	year := registrationYear()
	if f["certificate_year"] != "" {
		var e error
		year, e = strconv.Atoi(f["certificate_year"])
		if e != nil || year < 1900 || year > 2100 {
			return fmt.Errorf("%w: tahun sertifikat harus 1900–2100", errCertificate)
		}
	} else if certYr >= 1900 && certYr <= 2100 {
		year = certYr

	}

	if seq > 0 {
		if certYr < 1900 || certYr > 2100 || (f["certificate_year"] != "" && year != certYr) {
			return fmt.Errorf("%w: tahun sertifikat tidak sesuai dengan nomor", errCertificate)
		}
		year = certYr
		f["certificate_year"] = strconv.Itoa(year)
	}

	if r.GenerateCertificate {
		if f["certificate"] != "" {
			return fmt.Errorf("%w: kosongkan nomor sertifikat untuk menggunakan penomoran otomatis", errCertificate)
		}
		if r.ID > 0 {
			var old sql.NullString
			if e := tx.QueryRow("SELECT certificate FROM assessments WHERE id=? FOR UPDATE", r.ID).Scan(&old); e != nil {
				return e
			}
			if old.String != "" {
				return fmt.Errorf("%w: nomor sertifikat yang sudah tersimpan tidak boleh dibuat ulang otomatis", errCertificate)
			}
		}
		meta, e := skemaInfo(tx, f["scheme"])
		if e != nil {
			return e
		}
		floor, e := certificateFloor(tx)
		if e != nil {
			return e
		}
		// A locking read sees the latest committed allocation even when a
		// batch transaction established a repeatable-read snapshot earlier.
		var allocated int64
		if e = tx.QueryRow("SELECT last_seq FROM certificate_sequences WHERE scheme='0' AND year=0 FOR UPDATE").Scan(&allocated); e != nil {
			return e
		}
		floor = max(floor, allocated)
		if floor >= maxRegistrationSequence {
			return fmt.Errorf("%w: batas urutan sertifikat tercapai", errCertificate)
		}
		seq = floor + 1
		f["certificate"] = certificateNumber(meta, seq, year)
		f["certificate_year"] = strconv.Itoa(year)
	}
	if _, e := tx.Exec("UPDATE certificate_sequences SET last_seq=GREATEST(last_seq,?) WHERE scheme='0' AND year=0", seq); e != nil {
		return e
	}
	_, e := tx.Exec("INSERT INTO certificate_sequences(scheme,year,last_seq) VALUES('0',?,?) ON DUPLICATE KEY UPDATE last_seq=GREATEST(last_seq,VALUES(last_seq))", year, seq)
	return e
}

type registrationSummary struct {
	Scheme   string `json:"scheme"`
	Label    string `json:"label"`
	Last     int64  `json:"last"`
	Next     string `json:"next"`
	CertNext string `json:"cert_next,omitempty"`
	Total    int    `json:"total"`
	Missing  int    `json:"missing"`
	Legacy   int    `json:"legacy"`
}

type certificateSummaryInfo struct {
	Last          int64             `json:"last"`
	Next          string            `json:"next"`
	DefaultScheme string            `json:"default_scheme,omitempty"`
	NextByScheme  map[string]string `json:"next_by_scheme,omitempty"`
	Total         int               `json:"total"`
	Missing       int               `json:"missing"`
	Legacy        int               `json:"legacy"`
}

func (a *App) registrationSummary(w http.ResponseWriter, r *http.Request, s Session) {
	masters, e := a.masters()
	if e != nil {
		internal(w, e)
		return
	}
	year := registrationYear()
	certFloor, e := certificateFloor(a.db)
	if e != nil {
		internal(w, e)
		return
	}

	result := []registrationSummary{}
	for _, m := range masters {
		if m.Category != "SKEMA" {
			continue
		}
		item := registrationSummary{Scheme: m.Code, Label: m.Label}
		item.Last, e = registrationFloor(a.db, m.Code)
		if e != nil {
			internal(w, e)
			return
		}
		if item.Last < maxRegistrationSequence {
			item.Next = registrationNumber(item.Last + 1)
		}
		if meta, err := skemaInfo(a.db, m.Code); err == nil && certFloor < maxRegistrationSequence {
			item.CertNext = certificateNumber(meta, certFloor+1, year)
		}
		rows, e := a.db.Query("SELECT COALESCE(registration,'') FROM assessments WHERE scheme=?", m.Code)
		if e != nil {
			internal(w, e)
			return
		}
		for rows.Next() {
			var value string
			if e = rows.Scan(&value); e != nil {
				rows.Close()
				internal(w, e)
				return
			}
			item.Total++
			if value == "" {
				item.Missing++
			} else if !registrationPattern.MatchString(value) {
				item.Legacy++
			}
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			internal(w, e)
			return
		}
		result = append(result, item)
	}

	certSummary := certificateSummaryInfo{
		Last:         certFloor,
		NextByScheme: make(map[string]string),
	}
	for _, item := range result {
		if item.CertNext != "" {
			certSummary.NextByScheme[item.Scheme] = item.CertNext
			if certSummary.Next == "" {
				certSummary.Next = item.CertNext
				certSummary.DefaultScheme = item.Scheme
			}
		}
	}
	if certSummary.Next == "" && certFloor < maxRegistrationSequence {
		if meta, err := skemaInfo(a.db, "24"); err == nil {
			certSummary.Next = certificateNumber(meta, certFloor+1, year)
			certSummary.DefaultScheme = "24"
		} else {
			certSummary.Next = fmt.Sprintf("64911 4210 3 %07d %04d", certFloor+1, year)
		}
	}

	rows, e := a.db.Query("SELECT COALESCE(certificate,'') FROM assessments")
	if e != nil {
		internal(w, e)
		return
	}
	defer rows.Close()
	for rows.Next() {
		var value string
		if e = rows.Scan(&value); e != nil {
			internal(w, e)
			return
		}
		certSummary.Total++
		if value == "" {
			certSummary.Missing++
		} else if seq, yr := certificateSequence(value); seq == 0 || yr < 1900 || yr > 2100 {
			certSummary.Legacy++
		}
	}
	if e = rows.Err(); e != nil {
		internal(w, e)
		return
	}

	jsonOut(w, 200, map[string]any{"year": year, "schemes": result, "certificate": certSummary})
}

// Reference backups exclude candidate rows, so retain their highest known
// numbers even for archives written before local counters were introduced.
func preserveRegistrationFloors(tx *sql.Tx, snapshot *Backup) error {
	rows, e := tx.Query("SELECT code FROM master WHERE category='SKEMA' ORDER BY code")
	if e != nil {
		return e
	}
	schemes := []string{}
	for rows.Next() {
		var code string
		if e = rows.Scan(&code); e != nil {
			rows.Close()
			return e
		}
		schemes = append(schemes, code)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return e
	}
	counters := snapshot.Tables["registration_sequences"]
	var lastID int64
	for _, row := range counters {
		id, _ := strconv.ParseInt(*row[0], 10, 64)
		lastID = max(lastID, id)
	}
	year := strconv.Itoa(registrationYear())
	for _, scheme := range schemes {
		floor, e := registrationFloor(tx, scheme)
		if e != nil {
			return e
		}
		if floor == 0 {
			continue
		}
		value := strconv.FormatInt(floor, 10)
		found := false
		for _, row := range counters {
			if *row[1] == scheme && *row[2] == year {
				row[3] = &value
				found = true
				break
			}
		}
		if !found {
			lastID++
			id := strconv.FormatInt(lastID, 10)
			counters = append(counters, []*string{&id, &scheme, &year, &value})
		}
	}
	snapshot.Tables["registration_sequences"] = counters
	return nil
}

func preserveCertificateFloors(tx *sql.Tx, snapshot *Backup) error {
	counters := snapshot.Tables["certificate_sequences"]
	var lastID int64
	for _, row := range counters {
		id, _ := strconv.ParseInt(*row[0], 10, 64)
		lastID = max(lastID, id)
	}
	year := registrationYear()
	floor, e := certificateFloor(tx)
	if e != nil {
		return e
	}
	if floor > 0 {
		yearStr := strconv.Itoa(year)
		value := strconv.FormatInt(floor, 10)
		found := false
		for _, row := range counters {
			if *row[1] == "0" && *row[2] == yearStr {
				row[3] = &value
				found = true
				break
			}
		}
		if !found {
			lastID++
			id := strconv.FormatInt(lastID, 10)
			schemeZero := "0"
			counters = append(counters, []*string{&id, &schemeZero, &yearStr, &value})
		}
	}
	snapshot.Tables["certificate_sequences"] = counters
	return nil
}

func preserveFloors(tx *sql.Tx, snapshot *Backup) error {
	if e := preserveRegistrationFloors(tx, snapshot); e != nil {
		return e
	}
	return preserveCertificateFloors(tx, snapshot)
}
