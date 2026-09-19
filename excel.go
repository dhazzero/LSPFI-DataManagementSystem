package main

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
)

type ImportRow struct {
	Number int               `json:"number"`
	Fields map[string]string `json:"fields"`
	Errors []string          `json:"errors"`
}
type Preview struct {
	ID       string      `json:"id"`
	User     string      `json:"-"`
	Filename string      `json:"filename"`
	Sheet    string      `json:"sheet"`
	Sheets   []string    `json:"sheets"`
	Rows     []ImportRow `json:"rows"`
	Unknown  []string    `json:"unknown"`
	Valid    int         `json:"valid"`
	Bytes    []byte      `json:"-"`
	Expires  time.Time   `json:"expires"`
}

func parseWorkbook(data []byte, name, sheet string, masters []Master) (Preview, error) {
	p := Preview{ID: token(), Filename: filepath.Base(name), Rows: []ImportRow{}, Unknown: []string{}, Bytes: data, Expires: time.Now().Add(30 * time.Minute)}
	table := [][]string{}
	badNIK := map[int]bool{}
	numericDates := map[string]bool{}
	date1904 := false
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".csv":
		if !utf8.Valid(data) {
			return p, errors.New("simpan CSV dengan encoding UTF-8")
		}
		data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
		reader := csv.NewReader(bytes.NewReader(data))
		reader.FieldsPerRecord = -1
		first := strings.SplitN(string(data), "\n", 2)[0]
		if strings.Count(first, ";") > strings.Count(first, ",") {
			reader.Comma = ';'
		}
		for {
			row, e := reader.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return p, fmt.Errorf("CSV tidak valid: %w", e)
			}
			if len(row) > 100 {
				return p, errors.New("maksimal 100 kolom")
			}
			table = append(table, row)
			if len(table) > 10020 {
				return p, errors.New("maksimal 10.000 baris per impor")
			}
		}
		p.Sheet = "CSV"
		p.Sheets = []string{"CSV"}
	case ".xlsx":
		f, e := excelize.OpenReader(bytes.NewReader(data), excelize.Options{UnzipSizeLimit: 64 << 20, UnzipXMLSizeLimit: 8 << 20})
		if e != nil {
			return p, errors.New("Excel tidak dapat dibaca; gunakan .xlsx tanpa password")
		}
		defer f.Close()
		props, e := f.GetWorkbookProps()
		if e != nil {
			return p, e
		}
		if props.Date1904 != nil {
			date1904 = *props.Date1904
		}
		p.Sheets = f.GetSheetList()
		if len(p.Sheets) == 0 {
			return p, errors.New("workbook tidak memiliki sheet")
		}
		if sheet == "" {
			sheet = p.Sheets[0]
			for _, s := range p.Sheets {
				if strings.Contains(normal(s), "datainduk") {
					sheet = s
					break
				}
			}
		}
		p.Sheet = sheet
		iterator, e := f.Rows(sheet)
		if e != nil {
			return p, errors.New("sheet tidak ditemukan")
		}
		defer iterator.Close()
		for iterator.Next() {
			row, e := iterator.Columns(excelize.Options{RawCellValue: true})
			if e != nil {
				return p, e
			}
			if len(row) > 100 {
				return p, errors.New("sheet berisi lebih dari 100 kolom")
			}
			table = append(table, row)
			if len(table) > 10020 {
				return p, errors.New("maksimal 10.000 baris per impor")
			}
		}
		if e = iterator.Error(); e != nil {
			return p, e
		}
		header := -1
		for i, row := range table {
			for _, v := range row {
				if headerKey(v) == "nik" {
					header = i
					break
				}
			}
			if header >= 0 {
				break
			}
			if i >= 19 {
				break
			}
		}
		if header >= 0 {
			for col, h := range table[header] {
				key := headerKey(h)
				if key == "nik" || key == "birth_date" || key == "test_date" || key == "plenary_date" {
					for r := header + 1; r < len(table); r++ {
						cell, _ := excelize.CoordinatesToCellName(col+1, r+1)
						typ, e := f.GetCellType(sheet, cell)
						if e != nil {
							return p, e
						}
						formula, e := f.GetCellFormula(sheet, cell)
						if e != nil {
							return p, e
						}
						if key == "nik" && (typ == excelize.CellTypeNumber || typ == excelize.CellTypeUnset || formula != "") {
							if col < len(table[r]) && strings.TrimSpace(table[r][col]) != "" {
								badNIK[r+1] = true
							}
						}
						if key != "nik" && (typ == excelize.CellTypeNumber || typ == excelize.CellTypeUnset) {
							numericDates[fmt.Sprintf("%d/%d", r, col)] = true
						}
					}
				}
			}
		}
	default:
		return p, errors.New("gunakan file .xlsx atau .csv; untuk .xls lama, simpan ulang sebagai .xlsx di Excel")
	}
	header := -1
	for i, row := range table {
		hasNIK, hasName := false, false
		for _, v := range row {
			k := headerKey(v)
			hasNIK = hasNIK || k == "nik"
			hasName = hasName || k == "name" || k == "name_upper"
		}
		if hasNIK && hasName {
			header = i
			break
		}
		if i >= 19 {
			break
		}
	}
	if header < 0 {
		return p, errors.New("header NIK dan Nama Proper/Nama Upper/Nama Lengkap tidak ditemukan pada 20 baris pertama")
	}
	keys := map[int]string{}
	seen := map[string]bool{}
	for i, h := range table[header] {
		key := headerKey(h)
		if key == "" {
			if normal(h) != "no" && strings.TrimSpace(h) != "" {
				p.Unknown = append(p.Unknown, h)
			}
			continue
		}
		if seen[key] {
			return p, fmt.Errorf("kolom ganda untuk %s", h)
		}
		seen[key] = true
		keys[i] = key
	}
	for i := header + 1; i < len(table); i++ {
		raw := map[string]string{}
		nonempty := false
		for col, key := range keys {
			if col < len(table[i]) {
				v := strings.TrimSpace(table[i][col])
				if v != "" && v != "-" {
					nonempty = true
				}
				if numericDates[fmt.Sprintf("%d/%d", i, col)] && v != "" {
					num, e := strconv.ParseFloat(v, 64)
					if e == nil {
						d, e := excelize.ExcelDateToTime(num, date1904)
						if e == nil {
							v = d.Format("2006-01-02")
						}
					}
				}
				raw[key] = v
			}
		}
		if !nonempty {
			continue
		}
		mapped, issues := validate(raw, masters)
		if badNIK[i+1] {
			issues = append(issues, "NIK berupa angka/formula Excel: presisi 16 digit tidak dapat dipastikan. Cocokkan dengan sumber dan simpan sebagai teks.")
		}
		p.Rows = append(p.Rows, ImportRow{Number: i + 1, Fields: mapped, Errors: issues})
		if len(p.Rows) > 10000 {
			return p, errors.New("maksimal 10.000 baris per impor")
		}
	}
	if len(p.Rows) == 0 {
		return p, errors.New("tidak ada baris data pada sheet ini")
	}
	return p, nil
}

func (a *App) validateDuplicates(p *Preview) error {
	seen := map[string]bool{}
	certs := map[string]bool{}
	p.Valid = 0
	for i := range p.Rows {
		r := &p.Rows[i]
		key := identity(r.Fields)
		cert := strings.ToLower(r.Fields["certificate"])
		if seen[key] {
			r.Errors = append(r.Errors, "Duplikat asesmen dalam file yang sama")
		}
		seen[key] = true
		if cert != "" {
			if certs[cert] {
				r.Errors = append(r.Errors, "Nomor sertifikat berulang dalam file")
			}
			certs[cert] = true
		}
		var count int
		e := a.db.QueryRow("SELECT COUNT(*) FROM assessments WHERE identity_key=? OR (certificate IS NOT NULL AND certificate=?) OR (registration IS NOT NULL AND scheme=? AND registration=?)", key, cert, r.Fields["scheme"], r.Fields["registration"]).Scan(&count)
		if e != nil {
			return e
		}
		if count > 0 {
			r.Errors = append(r.Errors, "Asesmen/nomor sertifikat sudah tersimpan; gunakan menu edit untuk koreksi")
		}
		if len(r.Errors) == 0 {
			p.Valid++
		}
	}
	return nil
}

func makeWorkbook(records []Record, masters []Master) (*excelize.File, error) {
	f := excelize.NewFile()
	sheet := "Data Induk"
	if e := f.SetSheetName("Sheet1", sheet); e != nil {
		return nil, e
	}
	headers := []any{"No"}
	for _, field := range fields {
		headers = append(headers, field.Label)
	}
	if e := f.SetSheetRow(sheet, "A1", &headers); e != nil {
		return nil, e
	}
	for i, r := range records {
		values := []any{i + 1}
		for _, field := range fields {
			v := r.Fields[field.Key]
			if field.Kind == "date" && v != "" {
				if d, e := time.Parse("2006-01-02", v); e == nil {
					v = d.Format("02/01/2006")
				}
			}
			if field.Key == "scheme" {
				for _, m := range masters {
					if m.Category == "SKEMA" && m.Code == v {
						v = m.Code + " - " + m.Label
						break
					}
				}
			}
			values = append(values, v)
		}
		cell, _ := excelize.CoordinatesToCellName(1, i+2)
		if e := f.SetSheetRow(sheet, cell, &values); e != nil {
			return nil, e
		}
	}
	style, e := f.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Color: []string{"124E4A"}, Pattern: 1}, Alignment: &excelize.Alignment{WrapText: true, Vertical: "center"}})
	if e != nil {
		return nil, e
	}
	if e = f.SetCellStyle(sheet, "A1", "AF1", style); e != nil {
		return nil, e
	}
	f.SetRowHeight(sheet, 1, 34)
	f.SetColWidth(sheet, "A", "AF", 22)
	f.SetColWidth(sheet, "A", "A", 6)
	textStyle, e := f.NewStyle(&excelize.Style{NumFmt: 49})
	if e != nil {
		return nil, e
	}
	for _, col := range []string{"M", "T", "U", "V", "W", "AA", "AB"} {
		if e = f.SetColStyle(sheet, col, textStyle); e != nil {
			return nil, e
		}
	}
	if e = f.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"}); e != nil {
		return nil, e
	}
	if e = f.AutoFilter(sheet, fmt.Sprintf("A1:AF%d", max(1, len(records)+1)), nil); e != nil {
		return nil, e
	}
	if _, e = f.NewSheet("Referensi"); e != nil {
		return nil, e
	}
	row := []any{"Kategori", "Kode", "Nama", "Kode induk"}
	f.SetSheetRow("Referensi", "A1", &row)
	for i, m := range masters {
		row = []any{m.Category, m.Code, m.Label, m.Parent}
		f.SetSheetRow("Referensi", fmt.Sprintf("A%d", i+2), &row)
	}
	f.SetColWidth("Referensi", "A", "D", 30)
	f.SetCellStyle("Referensi", "A1", "D1", style)
	return f, nil
}
