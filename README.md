# LSPFI Arsip Digital

Aplikasi web lokal berbasis Go untuk mengelola arsip asesmen LSP Fintech Indonesia. Database MySQL khusus **`lspfi_dms`** terpisah dari RegisterWeb Dashboard. HTML, CSS, dan JavaScript dibundel dalam executable Windows; pengguna tidak perlu memasang Node.js, Go, atau Python.

## Membuka aplikasi pada komputer ini

1. Pastikan layanan MySQL aktif.
2. Klik dua kali **`Mulai-LSPFI.cmd`**. Browser membuka `http://127.0.0.1:4080`.
3. Kredensial admin awal tersedia di **`.local/LOGIN-AWAL.txt`** setelah inisialisasi.
4. Ganti kata sandi melalui **Pengaturan**. Hapus salinan kata sandi awal setelah tersimpan di pengelola kata sandi Anda.

Penutupan browser tidak menghentikan aplikasi. Proses `LSPFI-Arsip.exe` berjalan di latar belakang dan dapat dihentikan melalui Task Manager setelah semua penyimpanan selesai. Jangan menjalankan dua salinan aplikasi yang menulis database/folder arsip yang sama.

## Yang sudah tersedia

- Login admin dan pembaca. Admin mengelola data, master, scan, akun baru, dan cadangan; pembaca dapat melihat serta mengekspor seluruh arsip.
- Ringkasan, pencarian nama/NIK/registrasi/sertifikat/perusahaan, filter skema/tahun/hasil, dan halaman daftar.
- Tambah dan edit data asesmen dengan pemeriksaan perubahan bersamaan. Satu NIK dapat memiliki banyak riwayat asesmen.
- Impor `.xlsx` atau CSV UTF-8, pilihan sheet, tinjauan per baris, pilihan baris valid, laporan kesalahan, pencegahan duplikat, transaksi penyimpanan, serta pelacakan file/baris sumber.
- Template dan ekspor Excel 32 kolom, ditambah sheet referensi. NIK, kode, telepon, dan nomor sertifikat dipertahankan sebagai teks.
- Scan PDF/JPG/PNG per asesmen dengan kategori dokumen, pratinjau, unduh, checksum SHA-256, dan akses yang membutuhkan login.
- Master skema, pendidikan, pekerjaan, provinsi, kabupaten, sumber anggaran, dan kementerian.
- Audit perubahan, cadangan ZIP lengkap, dan perintah pemulihan ke database kosong.

## Aturan konversi dari RegisterWeb Dashboard

Acuan yang dibaca: `lib/massal-template.ts`, `app/actions/uploadMassalAsesi.ts`, `app/actions/exportDataInduk.ts`, `app/api/export-asesi/route.ts`, dan `prisma/schema.prisma` dari proyek saudara `LSPFI - RegisterWeb_Dashboard`.

- 32 kolom dari `MASSAL_HEADERS` menjadi format utama. Variasi header huruf kecil dan format ekspor 22 kolom juga dipetakan sejauh tersedia. Kolom tambahan tidak masuk ke struktur utama; ditampilkan dalam tinjauan dan tetap berada di file sumber.
- NIK harus 16 digit teks. NIK yang berupa angka/formula Excel ditolak karena presisi digit tidak dapat dijamin; periksa sumber asli sebelum memperbaiki sel menjadi teks.
- Nama, NIK, tanggal lahir, email, telepon, dan skema wajib diisi sesuai aturan impor lama. Data yang belum lengkap tetap perlu diperbaiki sebelum disimpan.
- Kode dan nama pendidikan/pekerjaan harus merujuk ke satu nilai master yang sama; kode nol di depan dinormalisasi ke kode master. Kabupaten harus sesuai provinsi bila hubungan induk tersedia.
- Tanggal teks menerima dd/mm/yyyy, yyyy-mm-dd, serta nama bulan Indonesia. Tanggal numerik Excel mendukung kalender 1900 dan 1904.
- Nomor registrasi unik per skema; nomor sertifikat unik jika diisi. Jika registrasi kosong, identitas arsip menggunakan kombinasi NIK + skema + tanggal uji. Asesmen berulang pada hari yang sama harus menggunakan nomor registrasi berbeda.
- Nomor sertifikat/registrasi lama tidak dibuat ulang. Hasil K/BK tidak ditebak dari scan. Nilai kosong berarti belum tercatat.
- Impor tidak menimpa data yang sudah ada. Perubahan dilakukan melalui detail asesmen.
- Aplikasi ini merupakan konversi alur **arsip/Data Induk**, bukan salinan seluruh portal pendaftaran. Alur pembayaran, ujian daring, bank soal, penerbitan sertifikat baru, dan OCR belum dipindahkan.

## Penyimpanan dan cadangan

`config.json` memuat alamat MySQL, akun database khusus aplikasi, dan `storage_dir`. Password database hanya disimpan di konfigurasi lokal; jangan membagikan file ini. Pengguna aplikasi disimpan dengan bcrypt.

```text
MySQL / lspfi_dms         Data asesmen, master, akun, metadata scan, audit
data/documents/          Berkas scan asli dengan nama internal acak
data/imports/            Excel/CSV sumber yang berhasil diimpor
.local/                 Kredensial awal dan log lokal (jangan dibagikan)
```

Folder arsip dapat berada di hard drive lain melalui `storage_dir` absolut. Untuk memindahkan penyimpanan, hentikan aplikasi, salin seluruh folder arsip, ubah konfigurasi, lalu mulai kembali. Database MySQL tetap dikelola oleh layanan MySQL dan tidak berada di dalam folder scan.

**Pengaturan → Unduh cadangan lengkap** menghasilkan ZIP berisi snapshot data, akun (hash password), audit, scan, sumber impor, dan checksum. Backup dibentuk sebelum unduhan; jika scan referensi hilang/rusak, operasi dibatalkan. Selama pembuatan backup, perubahan melalui aplikasi menunggu agar data dan berkas konsisten. Jangan mengubah database atau file secara langsung selama operasi berjalan.

Untuk Google Drive, unggah ZIP yang telah selesai diunduh atau simpan salinannya ke folder sinkronisasi Drive. Tidak ada koneksi OAuth, unggahan otomatis, atau berbagi dokumen ke pihak lain. **Jangan menyinkronkan folder data aktif MySQL dengan Google Drive.** Cadangan tidak dienkripsi oleh aplikasi; akses folder cadangan harus dibatasi.

### Pemulihan

1. Gunakan komputer/server MySQL pemulihan dengan database `lspfi_dms` yang masih kosong. Semua tabel, akun, dan master harus kosong; aplikasi menolak menimpa arsip aktif.
2. Salin executable dan `config.example.json` menjadi `config.json`. Isi koneksi ke MySQL pemulihan. Buat database `lspfi_dms` kosong dan beri akun aplikasi akses hanya ke database ini. Jangan menjalankan `--init`, karena perintah itu membuat akun awal dan master.
3. Pastikan `storage_dir` menunjuk ke folder kosong. Jalankan:

```powershell
.\LSPFI-Arsip.exe --restore "D:\Cadangan\LSPFI-Arsip-YYYYMMDD-HHMMSS.zip"
```

4. Buka aplikasi. Akun, data, master, audit, dan scan dipulihkan dari cadangan. Bandingkan jumlah asesmen dan buka beberapa scan untuk verifikasi sebelum menggunakan hasil pemulihan.

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
