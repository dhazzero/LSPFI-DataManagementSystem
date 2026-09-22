package main

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func testMasters() []Master {
	return []Master{{Category: "SKEMA", Code: "24", Label: "3 Penagihan"}, {Category: "PENDIDIKAN", Code: "04", Label: "S1 / D4"}, {Category: "PEKERJAAN", Code: "01", Label: "Karyawan"}, {Category: "PROVINSI", Code: "31", Label: "DKI Jakarta"}, {Category: "KABUPATEN", Code: "3171", Label: "Jakarta Selatan", Parent: "31"}}
}
func testFields() map[string]string {
	return map[string]string{"nik": "0000000000000001", "name": "ASESI UJI", "scheme": "24", "email": "uji@example.test", "phone": "081234567890", "birth_date": "01/02/1990", "test_date": "19/09/2026", "registration": "ARSIP-UJI-1", "education": "04", "education_label": "S1", "occupation": "01", "occupation_label": "Karyawan", "result": "K", "province": "31", "city": "3171"}
}
func TestDatesAndValidation(t *testing.T) {
	for input, want := range map[string]string{"1/2/1990": "1990-02-01", "2026-09-19": "2026-09-19", "Sabtu, 19 September 2026": "2026-09-19", "29/02/2024": "2024-02-29"} {
		got, e := parseDate(input)
		if e != nil || got != want {
			t.Fatalf("%s: %s %v", input, got, e)
		}
	}
	for _, input := range []string{"31/02/2026", "29/02/2025", "2026-13-01", "01/01/0020"} {
		if _, e := parseDate(input); e == nil {
			t.Fatalf("invalid accepted: %s", input)
		}
	}
	out, issues := validate(testFields(), testMasters())
	if len(issues) != 0 {
		t.Fatal(issues)
	}
	if out["education"] != "04" || out["birth_date"] != "1990-02-01" {
		t.Fatal(out)
	}
	for key, value := range map[string]string{"nik": "123456789012345", "result": "Kompeten", "education_label": "SMA", "province": "32", "email": "invalid", "scheme": "missing"} {
		f := testFields()
		f[key] = value
		if _, issues := validate(f, testMasters()); len(issues) == 0 {
			t.Fatalf("bad %s accepted", key)
		}
	}
	f := testFields()
	f["registration"] = ""
	f["test_date"] = ""
	if _, issues := validate(f, testMasters()); len(issues) == 0 {
		t.Fatal("missing identity accepted")
	}
}

func TestRealLegacySchemeNames(t *testing.T) {
	masters := []Master{{Category: "SKEMA", Code: "24", Label: "3 Jenjang Kualifikasi 3 Bidang Fintech P2P Lending Sub Bidang Penagihan"}, {Category: "SKEMA", Code: "30", Label: "5 Jenjang Kualifikasi 5 Bidang Fintech P2P Lending Sub Bidang Eksekutif NON - Teknologi Informasi"}}
	for value, code := range map[string]string{"3 Penagihan": "24", "Jenjang Kualifikasi 3 Bidang Fintech P2P Lending Sub Bidang Penagihan": "24", "24 - Jenjang Kualifikasi 3 Bidang Fintech P2P Lending Sub Bidang Penagihan": "24", "5 non-ti": "30"} {
		m, e := matchMaster(value, "SKEMA", masters)
		if e != nil || m.Code != code {
			t.Fatalf("%s: %+v %v", value, m, e)
		}
	}
}

func TestTieredMasterMatching(t *testing.T) {
	masters := []Master{
		{ID: 1, Category: "PENDIDIKAN", Code: "05", Label: "D1/D2/D3"},
		{ID: 2, Category: "PENDIDIKAN", Code: "5", Label: "D1/D2/D3"},
		{ID: 3, Category: "PEKERJAAN", Code: "3", Label: "Pegawai Swasta"},
		{ID: 4, Category: "PEKERJAAN", Code: "15", Label: "Pegawai Swasta"},
		{ID: 5, Category: "KEMENTERIAN", Code: "01", Label: "Kementerian Ketenagakerjaan"},
	}

	// Exact code matching should resolve uniquely even with leading zero duplicates
	m5, err := matchMaster("5", "PENDIDIKAN", masters)
	if err != nil || m5.Code != "5" || m5.ID != 2 {
		t.Fatalf("expected code 5, got %+v, err: %v", m5, err)
	}
	m05, err := matchMaster("05", "PENDIDIKAN", masters)
	if err != nil || m05.Code != "05" || m05.ID != 1 {
		t.Fatalf("expected code 05, got %+v, err: %v", m05, err)
	}
	m15, err := matchMaster("15", "PEKERJAAN", masters)
	if err != nil || m15.Code != "15" || m15.ID != 4 {
		t.Fatalf("expected code 15, got %+v, err: %v", m15, err)
	}
	m3, err := matchMaster("3", "PEKERJAAN", masters)
	if err != nil || m3.Code != "3" || m3.ID != 3 {
		t.Fatalf("expected code 3, got %+v, err: %v", m3, err)
	}

	// Fallback to integer match when only "05" exists
	only05 := []Master{{ID: 10, Category: "PENDIDIKAN", Code: "05", Label: "D1/D2/D3"}}
	mLead, err := matchMaster("5", "PENDIDIKAN", only05)
	if err != nil || mLead.Code != "05" {
		t.Fatalf("expected leading zero match to 05, got %+v, err: %v", mLead, err)
	}

	// Unregistered code 100 should fail when not in masters
	_, err = matchMaster("100", "KEMENTERIAN", masters)
	if err == nil {
		t.Fatal("expected error for unregistered code 100 in KEMENTERIAN")
	}
}

func TestUserRowValidation(t *testing.T) {
	masters := []Master{
		{ID: 1, Category: "SKEMA", Code: "24", Label: "3 Layanan Pengguna"},
		{ID: 2, Category: "PENDIDIKAN", Code: "5", Label: "D1/D2/D3"},
		{ID: 3, Category: "PEKERJAAN", Code: "15", Label: "Pegawai Swasta"},
		{ID: 4, Category: "PROVINSI", Code: "36", Label: "Banten"},
		{ID: 5, Category: "KABUPATEN", Code: "3671", Label: "Kota Tangerang", Parent: "36"},
		{ID: 6, Category: "SUMBER_ANGGARAN", Code: "3", Label: "Mandiri"},
		{ID: 7, Category: "KEMENTERIAN", Code: "100", Label: "Mandiri / Perusahaan"},
	}

	fields := map[string]string{
		"name":             "Indah Septiani",
		"nik":              "3174055609910005",
		"scheme":           "3 Layanan Pengguna",
		"birth_date":       "16/09/1991",
		"email":            "indahtyan16@gmail.com",
		"phone":            "082286860855",
		"test_date":        "21/01/2026",
		"registration":     "PPL 2605 00005",
		"education":        "5",
		"education_label":  "D-3",
		"occupation":       "15",
		"occupation_label": "Customer Service Staff",
		"province":         "36",
		"city":             "3671",
		"funding":          "3",
		"ministry":         "100",
		"result":           "K",
	}

	out, issues := validate(fields, masters)
	if len(issues) > 0 {
		t.Fatalf("unexpected validation issues: %v", issues)
	}
	if out["education"] != "5" || out["education_label"] != "D1/D2/D3" {
		t.Fatalf("unexpected education: %+v", out)
	}
	if out["occupation"] != "15" || out["occupation_label"] != "Customer Service Staff" {
		t.Fatalf("unexpected occupation: %+v", out)
	}
	if out["ministry"] != "100" {
		t.Fatalf("unexpected ministry: %+v", out)
	}
}
func TestExcelRoundTripAndNumericNIK(t *testing.T) {
	f, issues := validate(testFields(), testMasters())
	if len(issues) > 0 {
		t.Fatal(issues)
	}
	f["company"] = "=HYPERLINK(\"https://example.test\")"
	wb, e := makeWorkbook([]Record{{Fields: f}}, testMasters())
	if e != nil {
		t.Fatal(e)
	}
	defer wb.Close()
	b, e := wb.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	p, e := parseWorkbook(b.Bytes(), "source.xlsx", "", testMasters())
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Rows) != 1 || len(p.Rows[0].Errors) > 0 {
		t.Fatalf("roundtrip: %+v", p.Rows)
	}
	if p.Rows[0].Fields["nik"] != f["nik"] || p.Rows[0].Fields["education"] != "04" || p.Rows[0].Fields["company"] != f["company"] {
		t.Fatal("text identifiers were changed")
	}
	formula, _ := wb.GetCellFormula("Data Induk", "I2")
	if formula != "" {
		t.Fatal("formula injection in export")
	}
	wb.SetCellInt("Data Induk", "M2", 1234567890123456)
	b, e = wb.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	p, e = parseWorkbook(b.Bytes(), "source.xlsx", "", testMasters())
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Rows[0].Errors) == 0 {
		t.Fatal("numeric NIK was accepted")
	}
}
func TestExcelDatesAndLegacyHeaders(t *testing.T) {
	wb := excelize.NewFile()
	defer wb.Close()
	headers := []any{"nama proper", "nik", "skema sertifikasi", "email", "telp", "tanggal lahir(dd/mm/yyyy)", "tanggal uji(dd/mm/yyyy)", "nomor registrasi"}
	row := []any{"ASESI UJI", "0000000000000001", "24", "uji@example.test", "081234567890", time.Date(1990, 2, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 9, 19, 0, 0, 0, 0, time.UTC), "UJI"}
	wb.SetSheetRow("Sheet1", "A3", &headers)
	wb.SetSheetRow("Sheet1", "A4", &row)
	b, e := wb.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	p, e := parseWorkbook(b.Bytes(), "old.xlsx", "", testMasters())
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Rows) != 1 || len(p.Rows[0].Errors) != 0 || p.Rows[0].Fields["birth_date"] != "1990-02-01" {
		t.Fatalf("legacy dates: %+v", p.Rows)
	}
	if _, e = parseWorkbook(b.Bytes(), "old.xls", "", testMasters()); e == nil {
		t.Fatal("unsupported xls accepted")
	}
}
func TestCSVAndDuplicateHeaders(t *testing.T) {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	w.Write([]string{"Nama Lengkap", "NIK", "Skema Sertifikasi", "Email", "No. HP", "Tanggal Lahir", "Tanggal Uji"})
	w.Write([]string{"ASESI UJI", "0000000000000001", "24", "uji@example.test", "081234567890", "01/02/1990", "19/09/2026"})
	w.Flush()
	p, e := parseWorkbook(b.Bytes(), "old.csv", "", testMasters())
	if e != nil || len(p.Rows) != 1 || len(p.Rows[0].Errors) != 0 {
		t.Fatalf("%+v %v", p, e)
	}
	if _, e = parseWorkbook([]byte("Nama Proper,NIK,NIK\nTest,1,1"), "bad.csv", "", nil); e == nil {
		t.Fatal("duplicate headers accepted")
	}
}
func TestIdentityAndConfigIsolation(t *testing.T) {
	for _, config := range []string{`{"database":"registerweb","address":"127.0.0.1:4080"}`, `{"database":"lspfi_dms","address":"0.0.0.0:4080"}`} {
		path := filepath.Join(t.TempDir(), "config.json")
		if e := os.WriteFile(path, []byte(config), 0600); e != nil {
			t.Fatal(e)
		}
		if _, e := readConfig(path); e == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	f := testFields()
	first := identity(f)
	f["scheme"] = "25"
	if identity(f) == first {
		t.Fatal("registration must be scoped by scheme")
	}
	f["registration"] = ""
	first = identity(f)
	f["test_date"] = "2026-09-20"
	if identity(f) == first {
		t.Fatal("repeated assessments must remain distinct")
	}
}

func TestCertificateYearImportValidation(t *testing.T) {
	for _, year := range []string{"1800", "2101", "abc", "2027"} {
		f := testFields()
		f["certificate"] = "64911 4210 3 0000045 2026"
		f["certificate_year"] = year
		if _, issues := validate(f, testMasters()); len(issues) == 0 {
			t.Fatalf("invalid certificate year accepted: %s", year)
		}
	}
	f := testFields()
	f["certificate"] = "CERT-LEGACY"
	f["certificate_year"] = "2026"
	if _, issues := validate(f, testMasters()); len(issues) != 0 {
		t.Fatal(issues)
	}
}
