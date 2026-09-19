'use strict';

function registrationControls(r) {
  const year = r.fields.registration_year || new Date().toLocaleDateString('en-CA', {timeZone:'Asia/Jakarta',year:'numeric'});
  if (!isAdmin()) return r.fields.registration_year ? `<p class="helper">Tahun registrasi: ${esc(year)}</p>` : '';
  return `<div class="info-box"><strong>Penomoran registrasi LSPFI</strong>Format PPL 2605 + urutan minimal 5 digit, per skema dan tahun. Nomor otomatis ditetapkan saat penyimpanan berhasil. Nomor lama dapat dicatat sesuai dokumen sumber.</div>
  ${!r.fields.registration ? `<label class="registration-toggle"><input id="registration-auto" type="checkbox" ${r.generate_registration?'checked':''}> Buat nomor registrasi otomatis saat disimpan</label>` : ''}
  <label>Tahun registrasi<input name="registration_year" type="number" min="1900" max="2100" value="${esc(r.fields.registration_year || (r.generate_registration ? year : ''))}" placeholder="${esc(year)}"></label>
  <p class="helper">Tahun pendaftaran, bukan tahun sertifikat atau tanggal uji. Jika kosong, penomoran memakai tahun berjalan WIB. Nomor yang sudah tersimpan tidak dibuat ulang otomatis.</p>`;
}

function renderRegistrations(data) {
  const missing = state.all.filter(r=>!r.fields.registration);
  $('#content').innerHTML=heading('Nomor registrasi','Petakan penomoran setiap skema dan lengkapi arsip yang belum memiliki nomor.', '<button class="primary" data-action="new">＋ Tambah asesmen</button>')+
    `<div class="info-box"><strong>PPL 2605 00001 · Tahun berjalan ${esc(data.year)}</strong>2605 mengikuti nomor lisensi pada RegisterWeb. Urutan dicatat per skema dan tahun. Karena tahun tidak tercetak pada nomor, DMS melanjutkan di atas urutan tertinggi yang diketahui pada skema tersebut agar nomor tidak berulang lintas tahun. Pratinjau belum memesan nomor.</div>
    <p class="helper">Acuan: counter DMS, arsip asesmen, serta counter dan pendaftaran dalam salinan lokal RegisterWeb. Salinan ini tidak tersinkron langsung dengan portal; bila keduanya menerbitkan nomor, gunakan satu sistem sebagai penerbit.</p>
    <section class="panel"><div class="table-wrap"><table><thead><tr><th>Skema</th><th>Urutan tertinggi</th><th>Pratinjau berikutnya</th><th>Arsip DMS</th><th>Tanpa nomor</th><th>Format lama/lain</th><th></th></tr></thead><tbody>${data.schemes.map(s=>`<tr><td><strong>${esc(s.label)}</strong><small>Kode ${esc(s.scheme)}</small></td><td>${number(s.last)}</td><td>${esc(s.next || 'Batas urutan tercapai')}</td><td>${number(s.total)}</td><td>${number(s.missing)}</td><td>${number(s.legacy)}</td><td><button class="row-button" data-action="registration-scheme" data-scheme="${esc(s.scheme)}">Lihat pemetaan →</button></td></tr>`).join('')}</tbody></table></div></section>
    <section class="panel"><div class="panel-title"><div><h2>Arsip belum memiliki nomor</h2><p>${number(missing.length)} asesmen · buka detail untuk mencatat nomor sumber atau memilih nomor otomatis.</p></div></div>${missing.length?table(missing):empty('Semua arsip sudah memiliki nomor','Nomor pada arsip lama tetap dipertahankan.')}</section>`;
}
