# Review penomoran registrasi dan sertifikat

## Ruang lingkup

Review mencakup perubahan lokal pada generator Go, form JavaScript, backup/restore, alias impor, referensi TypeScript, dan utilitas reset admin. Referensi TypeScript tidak dijalankan oleh aplikasi Go; berkas tersebut hanya menjadi acuan.

## Cara kerja

Registrasi berbentuk `PPL 2605 00051`. Angka 2605 adalah prefix tetap. Urutan terpisah per skema dan diteruskan lintas tahun karena tahun tidak muncul pada nomor registrasi. Sertifikat berbentuk `64911 4210 3 0000011 2026`: kode sektor, kode profesi, jenjang, urutan global, dan tahun. Padding 5/7 digit adalah lebar minimum, bukan batas maksimum.

DMS saat ini meneruskan urutan sertifikat lintas skema dan lintas tahun. Contoh: urutan 11 pada 2026 dilanjutkan 12 pada 2027. Ini mempertahankan implementasi dan pengujian yang sudah ditambahkan, tetapi berbeda dari TypeScript yang membuat counter per tahun. Penyamaan penuh dengan reset tahunan memerlukan keputusan bisnis dan perubahan floor, backup, serta pengujian bersama-sama.

Nomor baru memakai nilai tertinggi dari counter lokal, arsip DMS, dan salinan counter/pendaftaran RegisterWeb. Nomor manual yang mengikuti pola juga menaikkan counter. Format historis lain tetap dapat dicatat. Nilai counter tidak diturunkan jika nomor diedit atau arsip dihapus.

Penyimpanan nomor, arsip, counter, dan audit dilakukan dalam transaksi. Kegagalan menyebabkan rollback. Permintaan pembuatan ulang otomatis ditolak bila arsip sudah mempunyai nomor. Penomoran tidak menetapkan keputusan kompetensi K/BK.

## Temuan dan perbaikan

1. **Referensi privat tersedia sebagai aset publik.** Pola embed `web/*` ikut memasukkan folder data dan FileServer melayaninya tanpa login. Manifest memuat data akun. Embed kini dibatasi ke HTML, JS, CSS, dan PNG pada tingkat teratas. Test memastikan URL data mengembalikan 404 dan app.js tetap dapat diakses. Binary telah dibangun ulang; aplikasi lokal telah dijalankan ulang menggunakan build terbaru.
2. **Tahun sertifikat muncul dua kali.** Dua kontrol bernama sama dapat membuat nilai yang dipilih ditimpa saat FormData dikonversi. Sekarang hanya kontrol di bagian penomoran yang dipakai.
3. **Metadata ditebak.** Database kini diperiksa sebelum fallback 11 skema bawaan. Metadata sektor/profesi/jenjang yang tidak lengkap ditolak. Skema tidak dikenal tidak lagi otomatis diberi jenjang 3. Kesalahan query tidak disembunyikan.
4. **Counter global tidak dikunci di database.** Semua penyimpanan DMS kini mengunci baris sentinel `scheme=0, year=0` sebelum pembacaan floor. Baris ini tidak mewakili tahun penerbitan. Ini melengkapi mutex aplikasi dan mengurutkan transaksi DMS yang melalui saveRecord. Counter registrasi juga dibaca dengan locking read agar transaksi bersnapshot lama tidak menggunakan ulang nomor yang sudah dialokasikan. Counter pada baris sentinel juga dibaca dengan FOR UPDATE dan diperbarui agar snapshot transaksi batch yang lebih lama tidak memakai kembali alokasi DMS terbaru. Writer eksternal tetap memerlukan koordinasi terpisah.
5. **Tahun manual bisa bertentangan.** Tahun pada nomor berpola harus sesuai dengan certificate_year. Validasi ini juga dijalankan pada pratinjau impor. Jika field kosong, tahun diambil dari nomor. Untuk nomor otomatis tanpa tahun sertifikat, gunakan tahun berjalan WIB secara independen dari tahun registrasi. Format pola kini hanya menerima kode sektor/profesi numerik dan jenjang 1–9.
6. **Ringkasan menyembunyikan error database.** Error query, scan, dan iterasi kini dikembalikan sebagai kegagalan alih-alih statistik sebagian yang terlihat berhasil.
7. **Teks UI tidak sesuai generator.** Keterangan kini menyebut urutan lintas tahun dan padding minimum.
8. **Reset admin memakai password tetap.** Utilitas kini meminta environment variable LSPFI_ADMIN_PASSWORD sepanjang 12–72 byte, memeriksa parsing konfigurasi dan hashing, serta memakai pembentuk DSN driver MySQL. Password tidak ditampilkan. Skrip diuji berhasil pada database sintetis, termasuk login ulang, dan tidak dijalankan pada database utama. Utilitas mendukung flag `-config`, membatasi database ke `lspfi_dms`, memeriksa role admin, menggunakan timeout, dan mengembalikan status gagal ketika operasi gagal. Setelah reset administratif, restart aplikasi untuk mengakhiri sesi yang masih tersimpan di memori.

## Backup dan batas integrasi

Backup referensi menyertakan certificate_sequences dan floor tertinggi meskipun data peserta dikecualikan. Restore backup lama yang belum memiliki tabel counter tetap didukung. Counter sertifikat mempertahankan urutan global lintas tahun setelah pemulihan.

Tabel rw_* adalah salinan sumber. Penguncian lokal tidak memesan nomor di aplikasi RegisterWeb aktif. Jika kedua aplikasi menerbitkan nomor secara bersamaan, dibutuhkan satu layanan/counter bersama atau pembagian rentang nomor. Jangan menganggap salinan data sebagai sinkronisasi real-time.

Preview merupakan perkiraan dan belum memesan nomor. Metadata fallback adalah snapshot bawaan; perubahan master sumber diprioritaskan jika tabel sumber tersedia. Nomor legacy tetap perlu ditinjau berdasarkan dokumen aslinya.

## Verifikasi akhir — 20 September 2026

- Seluruh **20 test Go lulus**, termasuk semua pengujian integrasi MySQL; tidak ada yang dilewati pada pengujian final.
- `go test -race -count=1 ./...` dengan MySQL terisolasi lulus. Delapan transaksi bersnapshot lama diuji bersamaan lintas skema dan tahun; counter registrasi maupun sertifikat tidak berulang.
- Pengujian mencakup metadata database yang mengalahkan fallback, kode skema, metadata tidak lengkap, tahun tidak cocok, nomor manual, nomor duplikat, rollback, larangan pembuatan ulang, batas counter, dan backup/restore referensi, termasuk backup lama tanpa tabel counter sertifikat.
- `go vet ./...`, pemeriksaan sintaks JavaScript, dan `git diff --check` lulus.
- Chrome headless menguji login, penambahan asesmen, nomor otomatis, edit tanpa mengubah nomor, pergantian tab, toggle manual/otomatis, penolakan skema tidak dikenal, percobaan ulang setelah gagal, pelengkapan sertifikat arsip lama, serta persistensi setelah reload.
- Screenshot desktop diperiksa; ukuran teks pratinjau sudah diperbaiki menggunakan CSS eksternal yang diizinkan CSP. Pemeriksaan lebar layar 390 piksel tidak menemukan overflow halaman.
- Tidak ada error JavaScript pada skenario browser. URL berkas referensi di folder data menghasilkan HTTP 404.
- Binary `lspfi-dms` dibangun ulang. Layanan utama berjalan di `http://127.0.0.1:4080`; halaman/aset publik dan health check berhasil, API penomoran tanpa login menghasilkan 401.

Pengujian perubahan data dijalankan pada instance MySQL terpisah di port 13316 dengan direktori data sementara. Server serta data MySQL uji sudah dibersihkan setelah verifikasi. Database asli tidak dipakai untuk pengujian tulis. Restart aplikasi utama menjalankan migrasi idempotent dan mempertahankan arsip yang ada; sesi login perlu dibuat ulang.

## Menjalankan ulang tes

Untuk suite Go, sediakan file environment berisi DATABASE_URL MySQL dengan akun yang dapat membuat/menghapus database uji. Gunakan server pengujian terpisah. Test membuat database berawalan `lspfi_dms_test_` dan menghapusnya setelah selesai.

```bash
LSPFI_TEST_SOURCE_ENV=/path/to/test.env go test -race -count=1 ./...
go vet ./...
```

Tes browser tersedia di `scripts/browser-numbering.test.cjs`. Gunakan **database disposable yang belum memiliki asesmen**, akun admin uji, serta master skema 24, 25, dan 99; skema 99 sengaja tidak diberi metadata sertifikat. Skrip membuat empat arsip sintetis dan mengharapkan urutan awal 1. Jangan menjalankannya pada data produksi.

Environment yang dibutuhkan:

- `LSPFI_BROWSER_TEST_URL`: URL server aplikasi uji.
- `LSPFI_BROWSER_TEST_PASSWORD`: password admin uji; tidak dicetak oleh skrip.
- `PLAYWRIGHT_MODULE`: opsional, lokasi instalasi Playwright jika tidak tersedia di node_modules.
- `CHROME_PATH`: opsional, path executable Chrome lokal.
- `LSPFI_BROWSER_TEST_SCREENSHOT`: opsional, path PNG hasil pemeriksaan.

```bash
node scripts/browser-numbering.test.cjs
```

## Batas yang tetap berlaku

Pengujian ini memverifikasi operasi DMS lokal. Penomoran bersama aplikasi RegisterWeb yang masih aktif membutuhkan koordinasi counter eksternal dan belum menjadi fitur sinkronisasi. Urutan DMS tetap berlanjut lintas tahun; tidak diubah menjadi reset tahunan.

Screenshot hasil pengujian browser disimpan lokal di `.local/browser-numbering-final.png`.
