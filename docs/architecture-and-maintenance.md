# Struktur dan pemeliharaan LSPFI DMS

## Batas modul

| Bagian | File | Tanggung jawab |
|---|---|---|
| Startup | `main.go`, `schema.sql` | Konfigurasi, koneksi, migrasi, lifecycle server |
| HTTP | `http.go` | Rute, sesi, izin admin/pembaca, impor, ekspor, dokumen |
| Data asesmen | `data.go`, `excel.go` | Validasi, pemetaan kolom Excel, identitas arsip, penyimpanan |
| Referensi | `masters.go` | Validasi referensi, transaksi, pemeriksaan penggunaan, sinkronisasi lokal |
| Penomoran | `registration.go` | Alokasi dan validasi nomor registrasi/sertifikat |
| RegisterWeb | `registerweb.go`, `registerweb_http.go` | Snapshot dan pembacaan data sumber |
| Cadangan | `backup.go`, `backup_policy.go` | Ekspor/pemulihan referensi dan kebijakan isi cadangan |
| Akses API browser | `web/api.js` | JSON, kesalahan koneksi/server, sesi kedaluwarsa |
| Referensi browser | `web/masters.js` | Formulir, pencarian, pengurutan, halaman, aksi referensi |
| Pilihan asesmen | `web/assessment-form.js` | Opsi master, relasi provinsi/kabupaten, nilai lama yang perlu diperiksa |
| Shell aplikasi | `web/app.js` | Navigasi, state, tampilan asesmen/impor/pengaturan, event umum |
| Modul browser lain | `web/registerweb.js`, `web/registrations.js` | Penelusuran snapshot dan ringkasan penomoran |

Frontend tetap JavaScript tanpa bundler. Skrip dimuat dengan `defer`; fungsi modul dipanggil setelah state di `app.js` diinisialisasi. Semua aset browser dibundel oleh Go, sehingga perubahan JavaScript membutuhkan build dan restart server. Refresh browser saja tidak mengganti aset yang tertanam dalam executable lama.

## Aturan referensi

- `master` menjadi sumber pilihan dan validasi impor DMS. Identitas referensi adalah kategori dan kode; nama boleh berubah.
- Penambahan kode yang sudah ada ditolak dengan HTTP 409. Pengguna harus memilih Edit agar tidak menimpa referensi tanpa sengaja.
- Kabupaten/kota wajib menunjuk provinsi yang ada. Provinsi tidak memiliki induk.
- Menghapus referensi atau mengubah kode/induknya ditolak jika masih digunakan oleh asesmen, anak wilayah, counter penomoran, atau kolom RegisterWeb yang diperiksa di `masterInUse`.
- Pemeriksaan mencakup ketergantungan yang dikenal aplikasi, bukan seluruh kemungkinan relasi pada tabel kustom. Tambahkan pemetaan di `masterInUse` jika modul baru mulai memakai referensi.
- Penyimpanan `master`, sinkronisasi `rw_parameterbnsp`/nama `rw_skema`, dan audit berada dalam satu transaksi. Jika sinkronisasi gagal, seluruh perubahan dibatalkan.
- Sinkronisasi hanya menyentuh salinan lokal `rw_`; database RegisterWeb asal tidak ikut berubah.
- `rw_parameterbnsp` lama tidak selalu mempunyai unique index kategori/kode. Karena itu, jumlah baris diperiksa sebelum update/insert. Hasil `RowsAffected == 0` bukan bukti bahwa baris tidak ada.
- ID skema RegisterWeb diprioritaskan saat menyinkronkan nama. Master skema baru saja belum cukup untuk penerbitan sertifikat: sektor, profesi, dan jenjang juga harus tersedia.
- Nilai pilihan yang sudah tidak cocok tetap ditampilkan dengan penanda untuk diperiksa; formulir tidak boleh diam-diam menggantinya menjadi kosong.

## Menambah fitur

1. Tetapkan alur pengguna, aturan validasi, dan sumber data. Pisahkan referensi, asesmen, snapshot, dan penomoran.
2. Letakkan aturan bisnis di fungsi layanan seperti `storeMaster`, sehingga bisa diuji tanpa browser. Handler HTTP menangani parsing, izin, dan respons.
3. Gunakan parameter SQL untuk nilai. Nama tabel/kolom dinamis harus berasal dari daftar internal yang dibatasi.
4. Kembalikan seluruh error database; jangan meneruskan commit setelah sinkronisasi gagal. Tulis audit di transaksi yang sama.
5. Bila menambah field asesmen, periksa `fields`, validasi, formulir, pemetaan Excel, ekspor, kebijakan cadangan, dan referensi yang dipakai. Jangan mengubah format 32 kolom tanpa strategi kompatibilitas.
6. Uji kegagalan dan data lama, bukan hanya penyimpanan sukses. Gunakan MySQL terisolasi dan data sintetis.
7. Build, jalankan pada port uji, lalu restart aplikasi aktif setelah pemeriksaan lulus. Hindari menjalankan dua server yang menulis database aktif yang sama.

## Pengujian

```sh
go test ./...
go vet ./...
```

Pengujian MySQL memerlukan server uji dan akun yang boleh membuat/menghapus database sementara. `LSPFI_TEST_SOURCE_ENV` menunjuk file berisi `DATABASE_URL=mysql://...` untuk server uji. Jangan memperluas hak akun aplikasi aktif hanya untuk menjalankan pengujian.

```sh
LSPFI_TEST_SOURCE_ENV=/path/to/test.env go test -race ./... -count=1
```

Tanpa variabel tersebut, pengujian integrasi MySQL dilewati. Tes mencakup impor dan duplikat, perubahan asesmen, penomoran, dokumen, snapshot, cadangan/pemulihan, perlindungan referensi, serta rollback saat sinkronisasi gagal.

Pengujian browser memakai Playwright dan Chrome/Chromium yang tersedia:

```sh
PLAYWRIGHT_MODULE=/path/to/playwright CHROME_PATH=/path/to/chrome node scripts/browser-masters.test.cjs
```

Tes referensi menggunakan API tiruan; tidak menulis database. Tes `scripts/browser-numbering.test.cjs` memakai aplikasi nyata dan harus diarahkan ke database kosong terisolasi melalui `LSPFI_BROWSER_TEST_URL` dan `LSPFI_BROWSER_TEST_PASSWORD`, dengan master skema 24, 25, dan 99 (99 tanpa metadata sertifikat).

## Batas penggunaan dan pengembangan berikutnya

- Aplikasi masih lokal/localhost, dengan akses pembaca ke seluruh arsip. Penggunaan LAN atau portal klien memerlukan rancangan autentikasi, pembatasan data, HTTPS, dan deployment tersendiri.
- Backup aplikasi adalah backup referensi tanpa data asesi. Pemulihan penuh operasional membutuhkan cadangan MySQL dan folder arsip yang dikelola terpisah.
- Daftar asesmen masih diambil sekaligus ke browser; untuk pertumbuhan besar, pindahkan pencarian dan pagination ke server sebelum menambah beban data.
- Modul referensi sudah dipisahkan; pemecahan lebih lanjut pada `http.go` dan tampilan asesmen dapat dilakukan per fitur dengan tes regresi, tanpa mengganti seluruh stack.
- Jangan menyamakan kesiapan metadata skema dengan adanya kode di master. Pengelolaan metadata sertifikat melalui UI memerlukan rancangan field dan validasi khusus.
