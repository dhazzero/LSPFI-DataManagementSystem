'use strict';

function registrationControls(r) {
  const currentYear = new Date().toLocaleDateString('en-CA', {timeZone:'Asia/Jakarta',year:'numeric'});
  const year = r.fields.registration_year || currentYear;
  const certYear = r.fields.certificate_year || currentYear;
  if (!isAdmin()) {
    return (r.fields.registration_year ? `<p class="helper">Tahun registrasi: ${esc(year)}</p>` : '') +
           (r.fields.certificate_year ? `<p class="helper">Tahun sertifikat: ${esc(certYear)}</p>` : '');
  }
  return `<div class="info-box"><strong>Penomoran LSPFI (Registrasi & Sertifikat)</strong>
  • Format Registrasi: <code>PPL 2605 XXXXX</code> (minimal 5 digit urutan per skema, berlanjut lintas tahun).<br>
  • Format Sertifikat: <code>[sektor] [profesi] [jenjang] XXXXXXX [tahun]</code> (minimal 7 digit urutan global, berlanjut lintas tahun).<br>
  Nomor otomatis ditetapkan saat penyimpanan berhasil. Nomor lama dapat dicatat sesuai dokumen sumber.</div>
  ${!r.fields.registration ? `<label class="registration-toggle"><input id="registration-auto" type="checkbox" ${r.generate_registration?'checked':''}> Buat nomor registrasi otomatis saat disimpan</label>` : ''}
  <label>Tahun registrasi<input name="registration_year" type="number" min="1900" max="2100" value="${esc(r.fields.registration_year || (r.generate_registration ? year : ''))}" placeholder="${esc(year)}"></label>
  ${!r.fields.certificate ? `<label class="registration-toggle"><input id="certificate-auto" type="checkbox" ${r.generate_certificate?'checked':''}> Buat nomor sertifikat otomatis saat disimpan</label>` : ''}
  <label>Tahun sertifikat<input name="certificate_year" type="number" min="1900" max="2100" value="${esc(r.fields.certificate_year || (r.generate_certificate ? certYear : ''))}" placeholder="${esc(certYear)}"></label>
  <p class="helper">Tahun pendaftaran & sertifikasi. Jika kosong, penomoran memakai tahun berjalan WIB. Nomor yang sudah tersimpan tidak dibuat ulang otomatis.</p>`;
}

function renderRegistrations(data) {
  const missing = state.all.filter(r=>!r.fields.registration);
  const cert = data.certificate || {};
  const missingCert = state.all.filter(r=>!r.fields.certificate);
  const schemesWithCert = (data.schemes || []).filter(s => s.cert_next);
  const initialScheme = cert.default_scheme || (schemesWithCert[0] ? schemesWithCert[0].scheme : '');
  const initialCertPreview = cert.next || (schemesWithCert[0] ? schemesWithCert[0].cert_next : '—');
  $('#content').innerHTML=heading('Nomor registrasi & sertifikat','Petakan penomoran setiap skema dan lengkapi arsip yang belum memiliki nomor.', '<button class="primary" data-action="new">＋ Tambah asesmen</button>')+
    `<div class="info-box"><strong>PPL 2605 00001 · Sertifikat 7 digit · Tahun berjalan ${esc(data.year)}</strong>2605 mengikuti nomor lisensi pada RegisterWeb. Urutan registrasi berlanjut lintas tahun per skema, sedangkan nomor sertifikat berurutan global lintas seluruh skema. Pratinjau belum memesan nomor.</div>
    <section class="panel">
      <div class="panel-title">
        <div>
          <h2>Penomoran sertifikat global (Lintas Skema)</h2>
          <p>Pola: <code>[kode sektor] [kode profesi] [jenjang] [urutan 7 digit] [tahun]</code> · Berdasarkan RegisterWeb</p>
        </div>
      </div>
      <div class="stats">
        <div class="stat"><div class="stat-top">Urutan sertifikat tertinggi</div><strong>${number(cert.last||0)}</strong><small>Urutan lintas tahun</small></div>
        <div class="stat"><div class="stat-top">Pratinjau berikutnya</div><strong class="certificate-preview" id="cert-preview-text">${esc(initialCertPreview)}</strong>${schemesWithCert.length > 1 ? `<div class="cert-picker-wrap"><select id="cert-scheme-selector" class="cert-scheme-select" aria-label="Pilih skema untuk pratinjau sertifikat">${schemesWithCert.map(s=>`<option value="${esc(s.scheme)}" data-cert="${esc(s.cert_next)}" ${s.scheme===initialScheme?'selected':''}>${esc(s.scheme)} · ${esc(s.label)}</option>`).join('')}</select></div>` : schemesWithCert.length === 1 ? `<small>${esc(schemesWithCert[0].label)} (Kode ${esc(schemesWithCert[0].scheme)})</small>` : `<small>Urutan ke-${number((cert.last||0)+1)} tahun ${esc(data.year)}</small>`}</div>
        <div class="stat"><div class="stat-top">Memiliki sertifikat</div><strong>${number((cert.total||0)-(cert.missing||0))}</strong><small>Termasuk ${number(cert.legacy||0)} nomor format lama/lain</small></div>
        <div class="stat"><div class="stat-top">Belum ada sertifikat</div><strong>${number(cert.missing||0)}</strong><small>Arsip tanpa nomor sertifikat</small></div>
      </div>
    </section>
    <section class="panel"><div class="panel-title"><div><h2>Penomoran per skema</h2><p>Pola Registrasi: <code>PPL 2605 XXXXX</code> · Pola Sertifikat: <code>[kode sektor] [kode profesi] [jenjang] [urutan 7 digit] [tahun]</code></p></div></div><div class="table-wrap"><table><thead><tr><th>Skema</th><th>Urutan registrasi</th><th>Pratinjau registrasi</th><th>Pratinjau sertifikat</th><th>Arsip DMS</th><th>Tanpa registrasi</th><th>Format lama/lain</th><th></th></tr></thead><tbody>${data.schemes.map(s=>`<tr><td><strong>${esc(s.label)}</strong><small>Kode ${esc(s.scheme)}</small></td><td>${number(s.last)}</td><td><code>${esc(s.next || 'Batas urutan tercapai')}</code></td><td><code>${esc(s.cert_next || '—')}</code></td><td>${number(s.total)}</td><td>${number(s.missing)}</td><td>${number(s.legacy)}</td><td><button class="row-button" data-action="registration-scheme" data-scheme="${esc(s.scheme)}">Lihat pemetaan →</button></td></tr>`).join('')}</tbody></table></div></section>
    <section class="panel"><div class="panel-title"><div><h2>Arsip belum memiliki nomor registrasi</h2><p>${number(missing.length)} asesmen · buka detail untuk mencatat nomor sumber atau memilih nomor otomatis.</p></div></div>${missing.length?table(missing):empty('Semua arsip sudah memiliki nomor registrasi','Nomor pada arsip lama tetap dipertahankan.')}</section>
    <section class="panel"><div class="panel-title"><div><h2>Arsip belum memiliki nomor sertifikat</h2><p>${number(missingCert.length)} asesmen · buka detail untuk mencatat nomor sumber atau memilih nomor otomatis.</p></div></div>${missingCert.length?table(missingCert):empty('Semua arsip sudah memiliki nomor sertifikat','Nomor pada arsip lama tetap dipertahankan.')}</section>`;
}

document.addEventListener('change', event => {
  if (event.target && event.target.id === 'cert-scheme-selector') {
    const opt = event.target.selectedOptions && event.target.selectedOptions[0];
    const previewEl = document.getElementById('cert-preview-text');
    if (previewEl && opt && opt.dataset.cert) {
      previewEl.textContent = opt.dataset.cert;
    }
  }
});

