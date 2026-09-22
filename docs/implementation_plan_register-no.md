> Catatan review: dokumen ini adalah rencana awal. Implementasi dan perbaikan terkini dijelaskan di [review-penomoran.md](review-penomoran.md). DMS meneruskan urutan lintas tahun; perilaku ini berbeda dari reset tahunan pada referensi TypeScript.

# Rencana Penerapan Pola dan Logika Penomoran dari `registrasiAsesi.ts`

Dokumen ini menjelaskan implementasi pola penomoran otomatis untuk **Nomor Registrasi** dan **Nomor Pendaftaran / Sertifikat** pada sistem LSPFI DMS berdasarkan file sumber [`web/data/registrasiAsesi.ts`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/web/data/registrasiAsesi.ts).

---

## Analisis Pola & Logika di `web/data/registrasiAsesi.ts`

Di dalam file [`registrasiAsesi.ts`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/web/data/registrasiAsesi.ts), terdapat dua nomor yang digenerate dalam transaksi pembuatan pendaftaran (`tx.pendaftaran.create`):

1. **Nomor Registrasi (`nomor_registrasi`)**:
   - **Pola**: `PPL 2605 XXXXX` (5 digit running number dengan zero padding).
     - `2605` adalah nomor lisensi tetap LSPFI.
     - Contoh urutan: `PPL 2605 00001`, `PPL 2605 00012`, dst.
   - **Logika**: Menggunakan counter `registrationSequence` yang dihitung per **tahun** (`year`) dan per **skema** (`skema_id`).

2. **Nomor Sertifikat / Pendaftaran Sertifikasi (`no_sertifikat`)**:
   - **Pola**: `${skema.kode_sektor} ${skema.kode_profesi} ${skema.jenjang} ${certRunningNumber} ${currentYear}`
     - `kode_sektor`: 5 digit kode sektor dari skema (contoh: `64911`).
     - `kode_profesi`: 4 digit kode profesi dari skema (contoh: `4210`, `2510`, `2420`).
     - `jenjang`: 1 digit jenjang KKNI dari skema (contoh: `3`, `4`, `5`, `6`).
     - `certRunningNumber`: 7 digit running number dengan zero padding (`padStart(7, '0')`, contoh: `0000001`, `0000045`).
     - `currentYear`: 4 digit tahun sertifikasi (contoh: `2026`).
     - Format pemisah: spasi tunggal.
     - Contoh lengkap: `64911 4210 3 0000045 2026`.
   - **Logika**: Menggunakan counter `certificateSequence` yang dihitung per **tahun** secara **global** lintas seluruh skema (`skema_id = 0`).

---

## Kondisi Proyek Saat Ini

- **Nomor Registrasi**: Sudah sebagian didukung di [`registration.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/registration.go) dengan pola `PPL 2605 %05d` dan counter tabel `registration_sequences`.
- **Nomor Sertifikat**: Belum ada generator otomatis; saat ini hanya input manual, belum ada tabel `certificate_sequences`, dan belum ada lookup kode sektor/profesi/jenjang untuk membentuk format BNSP.
- **Tabel Pendaftaran & Skema**: DMS memiliki salinan tabel `rw_pendaftaran`, `rw_registrationsequence`, `rw_certificatesequence`, dan `rw_skema` (atau master data skema di `master_data.json` / `master_data.sql`).

---

## Rencana Perubahan

### 1. Database Schema
#### [MODIFY] [`schema.sql`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/schema.sql)
- Menambahkan tabel `certificate_sequences`:
  ```sql
  CREATE TABLE IF NOT EXISTS certificate_sequences (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    scheme VARCHAR(100) NOT NULL DEFAULT '0',
    year INT NOT NULL,
    last_seq BIGINT NOT NULL DEFAULT 0,
    UNIQUE KEY scheme_year(scheme,year)
  ) ENGINE=InnoDB;
  ```

---

### 2. Backend Logic (Go)
#### [MODIFY] [`registration.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/registration.go)
- **Metadata Skema**:
  - Fungsi `skemaInfo(db, schemeCode)` untuk mendapatkan `kode_sektor`, `kode_profesi`, dan `jenjang`. Mengambil dari `rw_skema` / `Skema`, dan fallback ke metadata bawaan 11 skema standar LSPFI Fintech P2P Lending jika tabel belum terisi.
- **Pola & Helper Nomor Sertifikat**:
  - `certificatePattern`: regexp untuk memvalidasi dan mem-parse `<sektor> <profesi> <jenjang> <7-digit-seq> <tahun>`.
  - `certificateNumber(sektor, profesi string, jenjang int, n int64, year int) string`: memformat string sertifikat.
  - `certificateSequence(s string) (int64, int, bool)`: mengekstrak counter dan tahun dari nomor sertifikat.
- **Floor & Sequence Counter**:
  - `certificateFloor(db, year int) (int64, error)`: menghitung angka urutan tertinggi yang sudah terbit dari:
    1. `certificate_sequences` lokal
    2. `assessments.certificate`
    3. Salinan RegisterWeb: `rw_certificatesequence` (`skema_id = 0`) dan `rw_pendaftaran.no_sertifikat`
- **Penerbitan Nomor (`prepareCertificate`)**:
  - Jika `r.GenerateCertificate == true`:
    - Ambil floor urutan global tahun berjalan.
    - Ambil metadata skema (`kode_sektor`, `kode_profesi`, `jenjang`).
    - Format nomor sertifikat baru (`n = floor + 1`).
    - Set `fields["certificate"]` dan `fields["certificate_year"]`.
    - Simpan/naikkan counter di `certificate_sequences`.
  - Jika sertifikat diinput manual dengan pola yang sama, naikkan counter agar nomor otomatis berikutnya tidak bertabrakan.
- **Ringkasan Penomoran (`registrationSummary`)**:
  - Sertakan data urutan sertifikat saat ini dan pratinjau nomor sertifikat berikutnya.

#### [MODIFY] [`data.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/data.go)
- Tambahkan field `GenerateCertificate bool json:"generate_certificate,omitempty"` ke `Record struct`.
- Di dalam `saveRecord(tx, r)`: jalankan `prepareCertificate(tx, r)` bersamaan dengan `prepareRegistration(tx, r)`.
- Di `aliases`: tambahkan alias `"nomorpendaftaran": "registration"`, `"nomorregistrasi": "registration"`, `"nomorsertifikat": "certificate"`.

#### [MODIFY] [`backup.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/backup.go) & [`backup_policy.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/backup_policy.go)
- Daftarkan `certificate_sequences` ke `backupTables` dan `validateReferenceBackup`.
- Pastikan kompatibilitas saat restore backup lama yang belum memiliki tabel `certificate_sequences`.
- Simpan floor `certificate_sequences` saat ekspor cadangan referensi.

---

### 3. Frontend & UI
#### [MODIFY] [`web/registrations.js`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/web/registrations.js)
- Pada kontrol form asesmen (`registrationControls`):
  - Tambahkan toggle checkbox: `Buat nomor sertifikat otomatis saat disimpan`.
  - Tampilkan informasi pola penomoran sertifikat (`kode_sektor kode_profesi jenjang XXXXXXX TAHUN`).
- Pada halaman Ringkasan Penomoran (`renderRegistrations`):
  - Tampilkan panel status urutan nomor sertifikat global tahun berjalan dan pratinjau nomor berikutnya.

#### [MODIFY] [`web/app.js`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/web/app.js)
- Saat membuka form Tambah Asesmen Baru (`openDetail` dengan id 0):
  - Default `generate_registration: true` dan `generate_certificate: true`.
- Update event listener change untuk `#certificate-auto`:
  - Set input `certificate` menjadi `readonly` dengan placeholder `"Dibuat saat disimpan"`.
- Kirim flag `generate_certificate` saat submit `record-form`.

---

## Rencana Verifikasi

### Pengujian Otomatis
- Jalankan unit test di [`registration_test.go`](file:///Users/dhanyhasyimazhari/IdeaProjects/LSPFI-DataManagementSystem/registration_test.go):
  - Test format nomor sertifikat (7 digit sequence, kode sektor, profesi, jenjang, tahun).
  - Test penomoran berurutan global lintas skema.
  - Test pencegahan nomor ganda dan benturan dengan nomor manual/lama.
  - Test backup & restore dengan tabel `certificate_sequences`.
- Jalankan seluruh test suite Go: `go test ./...`.

### Verifikasi Antarmuka & Fungsional
- Buka antarmuka web, uji Tambah Asesmen Baru.
- Pastikan nomor registrasi terisi `PPL 2605 XXXXX` dan nomor sertifikat terisi `${sektor} ${profesi} ${jenjang} ${XXXXXXX} ${tahun}`.
- Uji simpan beberapa asesmen berturut-turut untuk memastikan counter naik secara konsisten.
