# Git di Windows dan macOS

Git menyinkronkan kode, bukan database MySQL atau berkas lokal yang diabaikan.
Gunakan clone terpisah pada setiap komputer, bukan folder kerja Git yang disinkronkan melalui layanan cloud.

## Pengaturan sekali pada setiap clone

Jalankan dari direktori proyek di terminal IDE, Terminal macOS, atau PowerShell:

```sh
git config --local core.autocrlf input
```

`.gitattributes` menentukan LF untuk teks dan CRLF untuk salinan kerja `.cmd`/`.bat`.
Git menyimpan teks yang dinormalisasi sebagai LF dalam commit. `.editorconfig` membantu editor mengikuti aturan yang sama.
Pengaturan `--local` hanya berlaku untuk clone ini; tidak mengubah proyek lain.

## Commit pertama setelah menambahkan aturan

Simpan semua berkas di editor. Perintah berikut memasukkan seluruh perubahan tracked ke staging;
jalankan jika semua perubahan tersebut memang ingin disertakan:

```sh
git add --renormalize .
git add .gitattributes .editorconfig docs/git-windows-macos.md
git status
git diff --cached --stat
git diff --cached
```

Tambahkan berkas baru lain yang relevan dengan `git add <path>` atau centang melalui IDE.
Pastikan skrip macOS memiliki executable bit di Git:

```sh
git update-index --chmod=+x Mulai-LSPFI.command scripts/build.sh scripts/start.sh
```

Setelah meninjau perubahan dan menjalankan build/pengujian:

```sh
git commit -m "Update aplikasi dan standarkan line ending lintas platform"
git fetch origin
git rebase origin/main
git push origin main
git status -sb
```

Contoh ini untuk branch `main`. Rebase memerlukan working tree bersih; commit perubahan yang hendak disimpan terlebih dahulu.
Jika konflik muncul, selesaikan berkas, `git add <path>`, lalu `git rebase --continue`.
Untuk membatalkan rebase, gunakan `git rebase --abort`. Jangan force-push untuk mengatasi penolakan push biasa.
Jika remote berubah lagi sebelum push, ulangi fetch/rebase setelah memastikan working tree bersih.

## Kebiasaan setiap berpindah komputer

1. Pada komputer asal: simpan berkas, tinjau `git status`/diff, build dan uji, commit, lalu push.
2. Pada komputer tujuan, sebelum mengedit: `git status`, kemudian `git pull --ff-only` jika working tree bersih.
3. Jika pull ditolak karena riwayat bercabang, simpan pekerjaan lokal dengan commit dan lakukan fetch/rebase seperti di atas.
4. Build ulang lalu restart aplikasi; aset web ikut dibundel dalam executable.

## Menjalankan aplikasi

Pasang Go minimal sesuai `go.mod` (saat ini 1.26.0) untuk build, dan siapkan MySQL serta `config.json` lokal sesuai README.
Executable harus dibangun untuk OS/arsitektur komputer tujuan.

macOS:

```sh
chmod +x Mulai-LSPFI.command scripts/build.sh scripts/start.sh
./scripts/build.sh
./scripts/start.sh
```

Windows PowerShell:

```powershell
.\scripts\build.ps1
.\scripts\start.ps1
```

Kedua skrip build menjalankan test dan vet. Pengujian integrasi MySQL memerlukan konfigurasi tambahan yang dijelaskan di README.
Jika `config.json` belum ada, salin dari `config.example.json`, isi koneksi dan path lokal, lalu ikuti persiapan database di README.
Jangan menimpa konfigurasi instalasi yang sudah berjalan. Path absolut Windows perlu disesuaikan di macOS dan sebaliknya.

`config.json`, `.local/`, `data/`, dan executable diabaikan Git.
Push/pull tidak membawa akun, data asesi, scan, atau database ke komputer lain.
Pemindahan data lengkap memerlukan backup/restore MySQL dan folder penyimpanan yang konsisten saat aplikasi dihentikan.
Fitur cadangan referensi aplikasi tidak menyertakan data asesi dan scan; lihat batas cakupan di README.
