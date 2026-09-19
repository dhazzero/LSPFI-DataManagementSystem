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
		if floor >= maxRegistrationSequence {
			return fmt.Errorf("%w: batas urutan RegisterWeb tercapai; periksa pemetaan skema", errRegistration)
		}
		n = floor + 1
		f["registration"], f["registration_year"] = registrationNumber(n), strconv.Itoa(year)
	}
	_, e := tx.Exec("INSERT INTO registration_sequences(scheme,year,last_seq) VALUES(?,?,?) ON DUPLICATE KEY UPDATE last_seq=GREATEST(last_seq,VALUES(last_seq))", code, year, n)
	return e
}

type registrationSummary struct {
	Scheme  string `json:"scheme"`
	Label   string `json:"label"`
	Last    int64  `json:"last"`
	Next    string `json:"next"`
	Total   int    `json:"total"`
	Missing int    `json:"missing"`
	Legacy  int    `json:"legacy"`
}

func (a *App) registrationSummary(w http.ResponseWriter, r *http.Request, s Session) {
	masters, e := a.masters()
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
	jsonOut(w, 200, map[string]any{"year": registrationYear(), "schemes": result})
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
