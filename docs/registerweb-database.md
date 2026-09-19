# Database RegisterWeb di LSPFI DMS

Sumber: `lspfi`. Diperiksa: 2026-09-19T09:37:09Z. Tabel salinan memakai awalan `rw_` dalam `lspfi_dms`. Struktur berdasarkan database aktual, bukan hanya model Prisma.

| Tabel sumber | Tabel salinan | Baris | Kolom | Relasi |
|---|---|---:|---:|---:|
| activitylog | rw_activitylog | 71 | 6 | 1 |
| asesiprofile | rw_asesiprofile | 43 | 27 | 1 |
| asesmenmandiri | rw_asesmenmandiri | 24 | 8 | 2 |
| banksoal | rw_banksoal | 66 | 21 | 0 |
| certificatesequence | rw_certificatesequence | 5 | 4 | 0 |
| elemenkompetensi | rw_elemenkompetensi | 209 | 4 | 1 |
| kriteriaunjukkerja | rw_kriteriaunjukkerja | 618 | 4 | 1 |
| legacyarchiveimport | rw_legacyarchiveimport | 0 | 6 | 2 |
| paketsoal | rw_paketsoal | 0 | 10 | 0 |
| paketsoalitem | rw_paketsoalitem | 0 | 4 | 0 |
| parameterbnsp | rw_parameterbnsp | 40 | 7 | 0 |
| pendaftaran | rw_pendaftaran | 44 | 23 | 2 |
| registrationsequence | rw_registrationsequence | 5 | 4 | 0 |
| skema | rw_skema | 11 | 9 | 0 |
| unitkompetensi | rw_unitkompetensi | 79 | 4 | 1 |
| user | rw_user | 46 | 7 | 0 |

## Relasi putus yang sudah ada di sumber

Seluruh baris tetap disalin tanpa memperbaiki identitas secara otomatis. Pemeriksaan foreign key hanya dinonaktifkan pada koneksi penyalinan tabel rw_ selama pengisian data historis, kemudian diaktifkan kembali sebelum commit.

| Tabel | Kolom | Tabel induk | Baris tanpa induk |
|---|---|---|---:|
| elemenkompetensi | unit_kompetensi_id | unitkompetensi | 6 |
| kriteriaunjukkerja | elemen_kompetensi_id | elemenkompetensi | 3 |

## activitylog

Engine: InnoDB. Baris: 71.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| user_id | int | true | NULL |  |
| action | varchar(191) | false | NULL |  |
| description | text | false | NULL |  |
| ip_address | varchar(191) | true | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |

Indeks:

- `ActivityLog_action_idx`: `action` (urutan 1, unik false).
- `ActivityLog_user_id_idx`: `user_id` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `user_id` → `user.id`; ON UPDATE CASCADE; ON DELETE SET NULL.

SHA-256 isi salinan: `044d69afa68e6ece77749d3ed6f74fa681d21be516ef6dceceade2825e452fcb`.

## asesiprofile

Engine: InnoDB. Baris: 43.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| user_id | int | false | NULL |  |
| nama_lengkap | varchar(191) | false | NULL |  |
| nik | varchar(191) | false | NULL |  |
| tempat_lahir | varchar(191) | false | NULL |  |
| tanggal_lahir | date | false | NULL |  |
| jenis_kelamin | varchar(191) | false | NULL |  |
| kebangsaan | varchar(191) | false | NULL |  |
| alamat_rumah | text | false | NULL |  |
| kode_pos_rumah | varchar(191) | true | NULL |  |
| telp_rumah | varchar(191) | true | NULL |  |
| telp_hp | varchar(191) | false | NULL |  |
| email_pribadi | varchar(191) | false | NULL |  |
| kota_kabupaten | varchar(191) | false | NULL |  |
| provinsi | varchar(191) | false | NULL |  |
| kualifikasi_pendidikan | varchar(191) | false | NULL |  |
| status_pekerjaan | varchar(191) | false | NULL |  |
| nama_institusi | varchar(191) | true | NULL |  |
| jabatan | varchar(191) | true | NULL |  |
| tahun_met | varchar(191) | true | NULL |  |
| alamat_kantor | text | true | NULL |  |
| kode_pos_kantor | varchar(191) | true | NULL |  |
| telp_kantor | varchar(191) | true | NULL |  |
| fax_kantor | varchar(191) | true | NULL |  |
| email_kantor | varchar(191) | true | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | NULL |  |

Indeks:

- `AsesiProfile_nik_key`: `nik` (urutan 1, unik true).
- `AsesiProfile_user_id_key`: `user_id` (urutan 1, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `user_id` → `user.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `07ea894ed658e27fb9f94f8f5a55db58166a13a6b6d91c7a03764b4c969b9759`.

## asesmenmandiri

Engine: InnoDB. Baris: 24.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| pendaftaran_id | int | false | NULL |  |
| kuk_id | int | false | NULL |  |
| status_k_bk | varchar(191) | false | NULL |  |
| bukti_file_id | varchar(191) | true | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | NULL |  |
| keterangan | text | true | NULL |  |

Indeks:

- `AsesmenMandiri_kuk_id_fkey`: `kuk_id` (urutan 1, unik false).
- `AsesmenMandiri_pendaftaran_id_kuk_id_key`: `pendaftaran_id` (urutan 1, unik true).
- `AsesmenMandiri_pendaftaran_id_kuk_id_key`: `kuk_id` (urutan 2, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `kuk_id` → `kriteriaunjukkerja.id`; ON UPDATE CASCADE; ON DELETE CASCADE.
- `pendaftaran_id` → `pendaftaran.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `24517ebf78799de1ee6b85c183fe7e53b44778dbf80e11ce2936adf3d76254b8`.

## banksoal

Engine: InnoDB. Baris: 66.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| kode_soal | varchar(191) | false | NULL |  |
| skema_id | int | false | NULL |  |
| unit_kompetensi_id | int | false | NULL |  |
| elemen_kompetensi_id | int | true | NULL |  |
| kuk_id | int | true | NULL |  |
| metode_pengumpulan_bukti | varchar(191) | false | NULL |  |
| jenjang_kkni | int | false | NULL |  |
| pertanyaan | text | false | NULL |  |
| opsi_jawaban | json | true | NULL |  |
| kunci_jawaban_panduan | text | true | NULL |  |
| instruksi_praktik | text | true | NULL |  |
| peralatan_bahan | text | true | NULL |  |
| alokasi_waktu_menit | int | true | NULL |  |
| standar_kinerja | text | true | NULL |  |
| rubrik_penilaian | text | true | NULL |  |
| bobot_soal | double | false | 1 |  |
| status_soal | varchar(191) | false | AKTIF |  |
| created_by_user_id | int | true | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED on update CURRENT_TIMESTAMP(3) |

Indeks:

- `BankSoal_jenjang_kkni_idx`: `jenjang_kkni` (urutan 1, unik false).
- `BankSoal_metode_pengumpulan_bukti_idx`: `metode_pengumpulan_bukti` (urutan 1, unik false).
- `BankSoal_skema_id_idx`: `skema_id` (urutan 1, unik false).
- `BankSoal_unit_kompetensi_id_idx`: `unit_kompetensi_id` (urutan 1, unik false).
- `kode_soal`: `kode_soal` (urutan 1, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

SHA-256 isi salinan: `00e4ae0c74de20da504b73520307af9eecdce62592bb10cc7027a6cd35599d47`.

## certificatesequence

Engine: InnoDB. Baris: 5.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| year | int | false | NULL |  |
| skema_id | int | false | NULL |  |
| last_seq | int | false | 0 |  |

Indeks:

- `CertificateSequence_year_skema_id_key`: `year` (urutan 1, unik true).
- `CertificateSequence_year_skema_id_key`: `skema_id` (urutan 2, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

SHA-256 isi salinan: `d1adec92c56f9869413920c8341fcdde9200399e0ff37692be435740a38bfc4e`.

## elemenkompetensi

Engine: InnoDB. Baris: 209.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| unit_kompetensi_id | int | false | NULL |  |
| urutan_elemen | int | false | NULL |  |
| nama_elemen | text | false | NULL |  |

Indeks:

- `ElemenKompetensi_unit_kompetensi_id_idx`: `unit_kompetensi_id` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `unit_kompetensi_id` → `unitkompetensi.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `9be8fcc069183e99ad8574682cbaa01cb9a3bc040375cd68f4ca85a38dbb04bd`.

## kriteriaunjukkerja

Engine: InnoDB. Baris: 618.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| elemen_kompetensi_id | int | false | NULL |  |
| urutan_kuk | varchar(191) | false | NULL |  |
| pernyataan_kuk | text | false | NULL |  |

Indeks:

- `KriteriaUnjukKerja_elemen_kompetensi_id_idx`: `elemen_kompetensi_id` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `elemen_kompetensi_id` → `elemenkompetensi.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `97060ba6a890ad18ac383acf525a312b1e3c7740be7c0627f61581e60d522089`.

## legacyarchiveimport

Engine: InnoDB. Baris: 0.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | varchar(64) | false | NULL |  |
| asesi_id | int | false | NULL |  |
| skema_id | int | false | NULL |  |
| reviewed_by | int | false | NULL |  |
| review_hash | varchar(64) | false | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |

Indeks:

- `LegacyArchiveImport_asesi_id_idx`: `asesi_id` (urutan 1, unik false).
- `LegacyArchiveImport_skema_id_idx`: `skema_id` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `asesi_id` → `asesiprofile.id`; ON UPDATE CASCADE; ON DELETE RESTRICT.
- `skema_id` → `skema.id`; ON UPDATE CASCADE; ON DELETE RESTRICT.

SHA-256 isi salinan: `4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945`.

## paketsoal

Engine: InnoDB. Baris: 0.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| kode_paket | varchar(191) | false | NULL |  |
| nama_paket | varchar(191) | false | NULL |  |
| skema_id | int | false | NULL |  |
| durasi_menit | int | false | 60 |  |
| passing_grade | double | false | 70 |  |
| keterangan | text | true | NULL |  |
| status | varchar(191) | false | AKTIF |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED on update CURRENT_TIMESTAMP(3) |

Indeks:

- `kode_paket`: `kode_paket` (urutan 1, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

SHA-256 isi salinan: `4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945`.

## paketsoalitem

Engine: InnoDB. Baris: 0.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| paket_soal_id | int | false | NULL |  |
| bank_soal_id | int | false | NULL |  |
| urutan | int | false | 1 |  |

Indeks:

- `PaketSoalItem_paket_soal_id_bank_soal_id_key`: `paket_soal_id` (urutan 1, unik true).
- `PaketSoalItem_paket_soal_id_bank_soal_id_key`: `bank_soal_id` (urutan 2, unik true).
- `PRIMARY`: `id` (urutan 1, unik true).

SHA-256 isi salinan: `4f53cda18c2baa0c0354bb5f9a3ecbe5ed12ab4d8e11ba873c2f11161202b945`.

## parameterbnsp

Engine: InnoDB. Baris: 40.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| kategori | varchar(191) | false | NULL |  |
| kode | varchar(191) | false | NULL |  |
| label | varchar(191) | false | NULL |  |
| parent_kode | varchar(191) | true | NULL |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | NULL |  |

Indeks:

- `ParameterBnsp_kategori_idx`: `kategori` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

SHA-256 isi salinan: `ceb903fbe989f82bb6596f1c65cbcd04a629445abbc9432ddb92a298b9f8115e`.

## pendaftaran

Engine: InnoDB. Baris: 44.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| nomor_registrasi | varchar(191) | true | NULL |  |
| user_id | int | false | NULL |  |
| skema_id | int | false | NULL |  |
| tujuan_asesmen | varchar(191) | false | NULL |  |
| sumber_anggaran | varchar(191) | true | NULL |  |
| instansi_pemberi_anggaran | varchar(191) | true | NULL |  |
| kementerian | varchar(191) | true | NULL |  |
| nama_tuk | varchar(191) | true | NULL |  |
| jenis_tuk | varchar(191) | true | NULL |  |
| tanggal_uji | date | true | NULL |  |
| no_blanko_sertifikat | varchar(191) | true | NULL |  |
| no_sertifikat | varchar(191) | true | NULL |  |
| tahun_sertifikat | varchar(191) | true | NULL |  |
| tanggal_pleno | date | true | NULL |  |
| kode_jadwal | varchar(191) | true | NULL |  |
| no_reg_asesor | varchar(191) | true | NULL |  |
| nama_asesor | varchar(191) | true | NULL |  |
| keputusan_asesmen | varchar(191) | true | NULL |  |
| status_pendaftaran | varchar(191) | false | MENGISI_APL01 |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | NULL |  |
| metode_asesmen | varchar(255) | true | NULL |  |

Indeks:

- `Pendaftaran_skema_id_idx`: `skema_id` (urutan 1, unik false).
- `Pendaftaran_user_id_idx`: `user_id` (urutan 1, unik false).
- `PRIMARY`: `id` (urutan 1, unik true).

Relasi:

- `skema_id` → `skema.id`; ON UPDATE CASCADE; ON DELETE RESTRICT.
- `user_id` → `user.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `d92e564ae53030c56f4f993a470ff257d2ac78bbebfb7737672167506ea39f60`.

## registrationsequence

Engine: InnoDB. Baris: 5.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| year | int | false | NULL |  |
| skema_id | int | false | NULL |  |
| last_seq | int | false | 0 |  |

Indeks:

- `PRIMARY`: `id` (urutan 1, unik true).
- `RegistrationSequence_year_skema_id_key`: `year` (urutan 1, unik true).
- `RegistrationSequence_year_skema_id_key`: `skema_id` (urutan 2, unik true).

SHA-256 isi salinan: `10c1113de8e50acd3debc9e8ed7985542bb6970996b11271246d6e69320cc731`.

## skema

Engine: InnoDB. Baris: 11.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| kode_skema | varchar(191) | true | NULL |  |
| nama_skema | varchar(191) | false | NULL |  |
| jenjang | int | false | NULL |  |
| kode_sektor | varchar(191) | true | NULL |  |
| kode_profesi | varchar(191) | true | NULL |  |
| status | varchar(191) | false | Aktif |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |

Indeks:

- `PRIMARY`: `id` (urutan 1, unik true).
- `Skema_kode_skema_key`: `kode_skema` (urutan 1, unik true).

SHA-256 isi salinan: `02fe5cdc31a33e3993dcb4028ade5e25a7f61341a3c18c7daac6a7fff7d3d7f1`.

## unitkompetensi

Engine: InnoDB. Baris: 79.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| skema_id | int | false | NULL |  |
| kode_unit | varchar(191) | false | NULL |  |
| judul_unit | text | false | NULL |  |

Indeks:

- `PRIMARY`: `id` (urutan 1, unik true).
- `UnitKompetensi_skema_id_idx`: `skema_id` (urutan 1, unik false).

Relasi:

- `skema_id` → `skema.id`; ON UPDATE CASCADE; ON DELETE CASCADE.

SHA-256 isi salinan: `3296a1e61b3ce583c1d8098ea29fb17f76e833fbce99ed2c41296433336f787f`.

## user

Engine: InnoDB. Baris: 46.

| Kolom | Tipe | Nullable | Default | Tambahan |
|---|---|---|---|---|
| id | int | false | NULL | auto_increment |
| name | varchar(191) | false | NULL |  |
| email | varchar(191) | false | NULL |  |
| password | varchar(191) | false | NULL |  |
| role | varchar(191) | false | asesi |  |
| createdAt | datetime(3) | false | CURRENT_TIMESTAMP(3) | DEFAULT_GENERATED |
| updatedAt | datetime(3) | false | NULL |  |

Indeks:

- `PRIMARY`: `id` (urutan 1, unik true).
- `User_email_key`: `email` (urutan 1, unik true).

SHA-256 isi salinan: `7dc718155ab1e4f32654a1305c130bcbb0086243748010d52316a54b43430d15`.
