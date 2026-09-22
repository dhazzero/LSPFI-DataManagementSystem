# LSPFI Arsip Digital

Aplikasi web lokal berbasis Go untuk mengelola arsip asesmen LSP Fintech Indonesia. Database MySQL khusus **`lspfi_dms`** terpisah dari RegisterWeb Dashboard. HTML, CSS, dan JavaScript dibundel dalam executable Windows; pengguna tidak perlu memasang Node.js, Go, atau Python.

## Membuka aplikasi pada komputer ini

1. Pastikan layanan MySQL aktif.
2. Klik dua kali **`Mulai-LSPFI.cmd`**. Browser membuka `http://127.0.0.1:4080`.
3. Kredensial admin awal tersedia di **`.local/LOGIN-AWAL.txt`** setelah inisialisasi.
4. Ganti kata sandi melalui **Pengaturan**. Hapus salinan kata sandi awal setelah tersimpan di pengelola kata sandi Anda.

Penutupan browser tidak menghentikan aplikasi. Proses `LSPFI-Arsip.exe` berjalan di latar belakang dan dapat dihentikan melalui Task Manager setelah semua penyimpanan selesai. Jangan menjalankan dua salinan aplikasi yang menulis database/folder arsip yang sama.

### macOS / Linux

Pada mesin pengembangan, jalankan `./scripts/build.sh` untuk pengujian dan build. Setelah `config.json` disiapkan dan MySQL aktif, jalankan `./scripts/start.sh`, atau klik **Mulai-LSPFI.command** pada macOS. Buka alamat yang ditampilkan di terminal. Server berjalan di terminal tersebut; gunakan Ctrl+C untuk menghentikannya. Komputer pengguna yang sudah memiliki executable tidak memerlukan Go.

### Mengelola referensi

Buka **Data referensi**, pilih kategori, lalu isi kode dan nama. Untuk pendidikan, kode induk boleh dikosongkan; untuk kabupaten/kota, pilih provinsi induknya. Gunakan **Edit** pada baris yang sudah ada, lalu **Perbarui referensi**. Penambahan kode ganda ditolak agar data lama tidak tertimpa. Pencarian dan pembagian halaman memudahkan penelusuran daftar wilayah.

Referensi yang masih dipakai asesmen, wilayah anak, penomoran, atau data RegisterWeb tidak boleh dihapus atau diganti kode/induknya. Nama tetap bisa diedit. Pesan kegagalan tampil di formulir dan isian dipertahankan. Perubahan serta sinkronisasi ke salinan lokal RegisterWeb disimpan dalam satu transaksi.

Pada formulir asesmen, pilih provinsi terlebih dahulu agar kabupaten/kota tersaring sesuai induknya. Mengganti provinsi mengosongkan pilihan kabupaten yang tidak sesuai.

## Yang sudah tersedia

- Login admin dan pembaca. Admin mengelola data, master, scan, akun baru, dan cadangan; pembaca dapat melihat serta mengekspor seluruh arsip.
- Ringkasan, pencarian nama/NIK/registrasi/sertifikat/perusahaan, filter skema/tahun/hasil, dan halaman daftar.
- Tambah dan edit data asesmen dengan pemeriksaan perubahan bersamaan. Satu NIK dapat memiliki banyak riwayat asesmen.
- Impor `.xlsx` atau CSV UTF-8, pilihan sheet, tinjauan per baris, pilihan baris valid, laporan kesalahan, pencegahan duplikat, transaksi penyimpanan, serta pelacakan file/baris sumber.
- Template dan ekspor Excel 32 kolom, ditambah sheet referensi. NIK, kode, telepon, dan nomor sertifikat dipertahankan sebagai teks.
- Scan PDF/JPG/PNG per asesmen dengan kategori dokumen, pratinjau, unduh, checksum SHA-256, dan akses yang membutuhkan login.
- Master skema, pendidikan, pekerjaan, provinsi, kabupaten, sumber anggaran, dan kementerian.
- Audit perubahan, cadangan ZIP referensi tanpa data asesi, dan perintah pemulihan ke database kosong.
- Menu **Database RegisterWeb** untuk admin: seluruh tabel MySQL sumber, isi baris, pencarian, detail lengkap, struktur, indeks, relasi, dan ekspor Excel per tabel.
- Pendaftaran lama dapat dipetakan ke formulir **Data Asesmen** melalui **Lihat → Gunakan di Data Asesmen**. Admin memeriksa dan melengkapi data sebelum menyimpan; pendaftaran yang sudah dipetakan akan membuka arsip yang sama.

## Salinan lengkap database RegisterWeb

Database aktual RegisterWeb diperiksa langsung melalui MySQL, lalu seluruh tabel disalin ke `lspfi_dms` dengan awalan `rw_`. Tabel bawaan DMS (`users`, `master`, `assessments`, `documents`, `audit`) tetap terpisah. Data sumber tidak diubah. Akun pada `rw_user` adalah arsip pengguna lama dan **tidak otomatis memperoleh akses login ke DMS**.

Tabel meliputi pengguna, profil asesi, pendaftaran, asesmen mandiri, master BNSP, skema, unit, elemen, KUK, bank soal, paket soal, isi paket, urutan registrasi/sertifikat, audit, serta catatan impor arsip. Tabel kosong juga dipertahankan. Laporan kolom, tipe, default, nullable, indeks, dan foreign key tersedia di [docs/registerweb-database.md](docs/registerweb-database.md); metadata lengkap ada di [docs/registerweb-schema.json](docs/registerweb-schema.json).

Menu Database RegisterWeb membaca salinan lokal dan hanya tersedia bagi admin. Nilai password/secret/token tidak ditampilkan atau disertakan dalam ekspor tabel biasa. Cadangan hanya memuat data referensi dan akun pengelola/asesor; informasi asesi dikecualikan. Setiap tabel diperiksa melalui jumlah baris dan checksum SHA-256 terhadap snapshot sumber.

**Relasi bermasalah di sumber:** jika baris lama menunjuk data induk yang sudah tidak ada, baris tetap dipertahankan dan ditandai dalam menu/laporan. Pemeriksaan foreign key hanya dilepas sementara pada koneksi khusus penyalinan tabel `rw_`, kemudian langsung diaktifkan kembali sebelum transaksi selesai. Struktur foreign key tetap tersedia; tidak ada identitas yang ditebak atau data yang dibuang. Hubungan yang hanya ada di model Prisma tetapi tidak ada di database tidak ditambahkan sebagai constraint baru.

Untuk menjalankan migrasi pada instalasi baru yang belum memiliki salinan RegisterWeb, hentikan aplikasi dahulu:

```powershell
.\LSPFI-Arsip.exe --import-registerweb --source-env "F:\Dhany\Project\LSPFI - RegisterWeb_Dashboard\.env"
```

Sebelum migrasi, aplikasi membuat ZIP cadangan referensi tanpa data asesi `data/before-registerweb-*.zip`. Penyalinan dilakukan dalam transaksi, setelah membuat struktur tabel. Bila penyalinan gagal, isi tabel dibatalkan; struktur kosong yang sudah cocok dapat dipakai kembali saat percobaan berikutnya. Migrasi menolak menimpa salinan yang sudah selesai. Perubahan berikutnya di database lama tidak tersinkron otomatis.

Untuk pemeriksaan struktur sumber tanpa menyalin data:

```powershell
.\LSPFI-Arsip.exe --inspect-registerweb --source-env "F:\Dhany\Project\LSPFI - RegisterWeb_Dashboard\.env"
```

Backup–restore DMS menyertakan struktur seluruh tabel `rw_`, data referensi yang diizinkan, metadata migrasi, serta catatan relasi putus pada tabel referensi. Tabel yang berisi informasi asesi tetap dipulihkan strukturnya, dengan isi kosong. Snapshot MySQL ini **belum mencakup koleksi MongoDB dan berkas yang dirujuk melalui MongoDB**, sesuai cakupan migrasi tahap ini. Alur bank soal/ujian portal lama belum menjadi modul operasional DMS; datanya dapat ditelusuri dan diekspor dari menu Database RegisterWeb.

## Aturan konversi dari RegisterWeb Dashboard

Acuan yang dibaca: `lib/massal-template.ts`, `app/actions/uploadMassalAsesi.ts`, `app/actions/exportDataInduk.ts`, `app/api/export-asesi/route.ts`, dan `prisma/schema.prisma` dari proyek saudara `LSPFI - RegisterWeb_Dashboard`.

- 32 kolom dari `MASSAL_HEADERS` menjadi format utama. Variasi header huruf kecil dan format ekspor 22 kolom juga dipetakan sejauh tersedia. Kolom tambahan tidak masuk ke struktur utama; ditampilkan dalam tinjauan dan tetap berada di file sumber.
- NIK harus 16 digit teks. NIK yang berupa angka/formula Excel ditolak karena presisi digit tidak dapat dijamin; periksa sumber asli sebelum memperbaiki sel menjadi teks.
- Nama, NIK, tanggal lahir, email, telepon, dan skema wajib diisi sesuai aturan impor lama. Data yang belum lengkap tetap perlu diperbaiki sebelum disimpan.
- Kode dan nama pendidikan/pekerjaan harus merujuk ke satu nilai master yang sama; kode nol di depan dinormalisasi ke kode master. Kabupaten harus sesuai provinsi bila hubungan induk tersedia.
- Tanggal teks menerima dd/mm/yyyy, yyyy-mm-dd, serta nama bulan Indonesia. Tanggal numerik Excel mendukung kalender 1900 dan 1904.
- Nomor registrasi unik per skema; nomor sertifikat unik jika diisi. Jika registrasi kosong, identitas arsip menggunakan kombinasi NIK + skema + tanggal uji. Asesmen berulang pada hari yang sama harus menggunakan nomor registrasi berbeda.
- Nomor sertifikat/registrasi lama tidak dibuat ulang. Untuk nomor registrasi kosong, admin dapat memilih penomoran otomatis di detail asesmen. Hasil K/BK tidak ditebak dari scan. Nilai kosong berarti belum tercatat.
- Impor tidak menimpa data yang sudah ada. Perubahan dilakukan melalui detail asesmen.
- Aplikasi ini merupakan konversi alur **arsip/Data Induk**, bukan salinan seluruh portal pendaftaran. Alur pembayaran, ujian daring, bank soal, penerbitan sertifikat baru, dan OCR belum dipindahkan.

## Penyimpanan dan cadangan

### Penomoran registrasi

Menu **Nomor registrasi** menampilkan pemetaan per skema, urutan tertinggi yang diketahui, pratinjau nomor berikutnya, jumlah arsip tanpa nomor, dan nomor berformat lama/lain. Buka detail arsip untuk mengisi nomor sumber atau memilih **Buat nomor registrasi otomatis saat disimpan**. Asesmen baru menggunakan pilihan otomatis secara default; nonaktifkan untuk mencatat nomor dari dokumen lama. Impor Excel tetap mempertahankan nomor sumber dan tidak menerbitkan nomor tanpa tinjauan admin.

Aturan diadaptasi dari `app/actions/registrasiAsesi.ts`, `submitApl01.ts`, `uploadMassalAsesi.ts`, dan model `RegistrationSequence` pada RegisterWeb: format **PPL 2605 + urutan minimal lima digit**, counter per skema dan tahun pendaftaran (default tahun berjalan WIB). Tahun registrasi disimpan terpisah dari tahun sertifikat/tanggal uji; pemetaan RegisterWeb mengambil tahun `createdAt` bila tersedia. Format ekspor Excel 32 kolom tetap kompatibel dengan sumber dan tidak menambahkan kolom tahun registrasi.

Karena nomor tidak memuat tahun dan DMS menjaga keunikan per skema lintas tahun, nomor baru melanjutkan di atas urutan tertinggi seluruh tahun dari counter DMS, arsip DMS, counter RegisterWeb, dan pendaftaran dalam salinan lokal RegisterWeb. Urutan tidak di-reset ke nomor yang sudah digunakan. Ini merupakan penyesuaian pencegahan benturan pada DMS, bukan perubahan format nomor RegisterWeb. Nomor dengan lebih dari lima digit tidak dipotong, mengikuti `padStart(5)` sumber.

Nomor dialokasikan dalam transaksi yang sama dengan asesmen, dikunci per skema, dan dibatalkan bila penyimpanan gagal. Nomor manual/impor dengan pola yang sama menaikkan counter tanpa menurunkannya. Nomor yang sudah tersimpan tidak dapat diganti melalui mode otomatis. Counter `registration_sequences` ikut cadangan referensi tanpa identitas asesi, sehingga pemulihan tidak mengulang urutan yang pernah diterbitkan. Backup lama tanpa tabel counter tetap dapat dipulihkan.

Salinan RegisterWeb **tidak tersinkron langsung**. Jika portal dan DMS sama-sama aktif, tetapkan satu sistem sebagai penerbit nomor; DMS tidak dapat menjamin keunikan terhadap penerbitan baru di portal setelah snapshot. Aturan di sini mengikuti implementasi proyek sumber, bukan verifikasi regulasi BNSP terbaru.

`config.json` memuat alamat MySQL, akun database khusus aplikasi, dan `storage_dir`. Password database hanya disimpan di konfigurasi lokal; jangan membagikan file ini. Pengguna aplikasi disimpan dengan bcrypt.

```text
MySQL / lspfi_dms         Data asesmen, master, akun, metadata scan, audit
data/documents/          Berkas scan asli dengan nama internal acak
data/imports/            Excel/CSV sumber yang berhasil diimpor
.local/                 Kredensial awal dan log lokal (jangan dibagikan)
```

Folder arsip dapat berada di hard drive lain melalui `storage_dir` absolut. Untuk memindahkan penyimpanan, hentikan aplikasi, salin seluruh folder arsip, ubah konfigurasi, lalu mulai kembali. Database MySQL tetap dikelola oleh layanan MySQL dan tidak berada di dalam folder scan.

**Pengaturan → Cadangan tanpa data asesi** menghasilkan ZIP format `lspfi-dms-v2`, scope `references-without-asesi`:

- Disertakan: master DMS, akun pengguna DMS (admin/pembaca), skema, unit kompetensi, elemen, KUK, bank soal, paket soal dan itemnya, parameter BNSP, serta counter penomoran registrasi/sertifikat.
- Akun RegisterWeb hanya disertakan jika perannya `superadmin`, `admin`, atau `asesor` dan tidak memiliki profil/pendaftaran asesi. Hash password pengelola tetap disertakan untuk pemulihan.
- Tidak disertakan: isi `assessments`, `documents`, `audit`, `rw_asesiprofile`, `rw_pendaftaran`, `rw_asesmenmandiri`, `rw_legacyarchiveimport`, `rw_activitylog`, akun RegisterWeb asesi, seluruh scan, dan seluruh file sumber impor Excel/CSV. Tabel RegisterWeb baru yang belum dikategorikan juga dikosongkan dalam backup.
- Struktur seluruh tabel RegisterWeb tetap disertakan. Jumlah baris dan checksum dihitung ulang berdasarkan isi backup. Referensi pembuat bank soal ke akun yang dikecualikan menjadi NULL dalam backup.

Kebijakan ini berlaku untuk unduhan dan backup otomatis sebelum migrasi. Data aktif dalam database tidak dihapus. ZIP yang dibuat sebelum perubahan ini tidak otomatis berubah dan mungkin masih berisi informasi asesi. Backup referensi tidak dapat memulihkan arsip pribadi asesi. Selama pembuatan backup, perubahan melalui aplikasi menunggu agar snapshot konsisten.

Untuk Google Drive, unggah ZIP yang telah selesai diunduh atau simpan salinannya ke folder sinkronisasi Drive. Tidak ada koneksi OAuth, unggahan otomatis, atau berbagi dokumen ke pihak lain. **Jangan menyinkronkan folder data aktif MySQL dengan Google Drive.** Cadangan tidak dienkripsi oleh aplikasi; akses folder cadangan harus dibatasi.

### Pemulihan

1. Gunakan komputer/server MySQL pemulihan dengan database `lspfi_dms` yang masih kosong. Semua tabel, akun, dan master harus kosong; aplikasi menolak menimpa arsip aktif.
2. Salin executable dan `config.example.json` menjadi `config.json`. Isi koneksi ke MySQL pemulihan. Buat database `lspfi_dms` kosong dan beri akun aplikasi akses hanya ke database ini. Jangan menjalankan `--init`, karena perintah itu membuat akun awal dan master.
3. Pastikan `storage_dir` menunjuk ke folder kosong. Jalankan:

```powershell
.\LSPFI-Arsip.exe --restore "D:\Cadangan\LSPFI-Referensi-YYYYMMDD-HHMMSS.zip"
```

4. Buka aplikasi. Akun pengelola, master, dan referensi RegisterWeb dipulihkan. Tabel informasi asesi serta folder scan/sumber impor tetap kosong untuk backup v2. Pemulihan backup lama v1 tetap didukung dan akan memulihkan isi asli backup lama tersebut.

Batas pemulihan versi ini: total isi ZIP 2 GB, 100.000 berkas, maksimal 20 MB per scan/sumber impor. Pemulihan memeriksa path, daftar file, checksum, dan relasi database. Untuk arsip lebih besar diperlukan prosedur backup database dan folder terpisah.

## Persiapan komputer utama

Hanya komputer utama yang memerlukan **MySQL 8+** (atau MariaDB yang kompatibel) dan aplikasi ini. Executable tidak menyertakan layanan MySQL. Versi ini sengaja mendengarkan localhost saja; penggunaan melalui LAN memerlukan pengaturan jaringan dan pengamanan HTTPS sebelum diaktifkan. Akun pembaca dapat melihat semua arsip, sehingga belum cocok sebagai portal klien per asesi.

### Konversi master dari proyek lama

Pada instalasi baru tanpa `config.json`, dengan akun MySQL sumber yang berwenang membuat database dan akun aplikasi:

```powershell
.\LSPFI-Arsip.exe --init --source-env "F:\Dhany\Project\LSPFI - RegisterWeb_Dashboard\.env"
```

Perintah hanya membaca `DATABASE_URL`, tabel `Skema`, dan `ParameterBnsp` dari sumber. Perintah membuat database `lspfi_dms`, akun MySQL baru dengan izin khusus database ini, konfigurasi, serta akun admin dengan password acak. Password MySQL proyek lama tidak dicetak atau disalin sebagai kredensial aplikasi. Data pribadi dan pendaftaran belum disalin; gunakan ekspor Excel dan impor yang ditinjau admin.

### Konfigurasi tanpa proyek lama

Salin `config.example.json` menjadi `config.json`, isi koneksi MySQL dan jalankan `LSPFI-Arsip.exe --init`. Akun MySQL pada konfigurasi harus disiapkan oleh pengelola dan diberi hak hanya untuk `lspfi_dms`. Bila akun tidak dapat membuat database, buat `lspfi_dms` lebih dahulu dengan akun administrator MySQL. Tambahkan referensi melalui aplikasi sebelum mengimpor Excel.

## Pengembangan dan pengujian

Panduan pembagian modul, aturan transaksi, pengujian, dan batas pengembangan berikutnya tersedia di [Struktur dan pemeliharaan](docs/architecture-and-maintenance.md). Aset browser dibundel dalam executable: setelah mengubah kode, lakukan build dan restart server sebelum memuat ulang browser.

Go minimal 1.26. Dependensi dikunci oleh `go.mod`/`go.sum`: driver MySQL, Excelize, dan bcrypt. Tidak ada aset CDN, font daring, atau layanan cloud wajib.

```powershell
.\scripts\build.ps1
```

Skrip menggunakan Go portable `.tools/go/bin/go.exe` bila tersedia, atau Go dari PATH. Build memeriksa unit test dan `go vet`. Untuk uji integrasi MySQL, isi `LSPFI_TEST_SOURCE_ENV` dengan file konfigurasi sumber. Pengujian membuat database sementara bernama `lspfi_dms_test_<acak>`, menggunakan data sintetis, dan menghapus database uji setelah selesai; tidak mengubah database sumber atau `lspfi_dms`.

```powershell
$env:LSPFI_TEST_SOURCE_ENV='F:\Dhany\Project\LSPFI - RegisterWeb_Dashboard\.env'
.\scripts\build.ps1
```

Untuk distribusi, bawa `LSPFI-Arsip.exe`, `Mulai-LSPFI.cmd`, `scripts/start.ps1`, `config.example.json`, dan panduan ini. Siapkan `config.json` pada komputer tujuan. Jangan membagikan `.local`, `config.json` aktif, atau data pribadi sebagai paket aplikasi.
