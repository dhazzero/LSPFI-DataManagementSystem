'use strict';

const masterLabels = {
  SKEMA: 'Skema sertifikasi', PENDIDIKAN: 'Pendidikan', PEKERJAAN: 'Pekerjaan',
  PROVINSI: 'Provinsi', KABUPATEN: 'Kabupaten / kota',
  SUMBER_ANGGARAN: 'Sumber anggaran', KEMENTERIAN: 'Kementerian'
};
const masterColumns = [{key:'code',label:'Kode'}, {key:'label',label:'Nama referensi'}, {key:'parent',label:'Induk'}];
const masterPageSize = 50;

function getMasterParentText(m) {
  if (!m?.parent) return '—';
  const province = m.category === 'KABUPATEN' && state.meta.masters.find(p => p.category === 'PROVINSI' && p.code === m.parent);
  return province ? `${m.parent} · ${province.label}` : m.parent;
}

function renderMasterParentField(category, value='') {
  if (category === 'KABUPATEN') {
    const provinces = state.meta.masters.filter(m => m.category === 'PROVINSI');
    return `<label>Provinsi induk *<select name="parent" required><option value="">Pilih provinsi</option>${provinces.map(p => `<option value="${esc(p.code)}" ${p.code===value?'selected':''}>${esc(p.code)} · ${esc(p.label)}</option>`).join('')}</select></label>`;
  }
  if (category === 'PROVINSI') return '<input type="hidden" name="parent" value="">';
  return `<label>Kode induk (opsional)<input name="parent" maxlength="100" value="${esc(value)}" placeholder="Boleh dikosongkan"></label>`;
}

function getSortedMasters() {
  const category = state.masterCategory || 'SKEMA';
  const search = (state.masterSearch || '').trim().toLocaleLowerCase('id');
  const sort = state.masterSort;
  const all = state.meta.masters.filter(m => m.category === category);
  const sorted = all.filter(m => !search || [m.code,m.label,getMasterParentText(m)].some(v => v.toLocaleLowerCase('id').includes(search)));
  sorted.sort((a,b) => {
    const av = sort.column === 'parent' ? getMasterParentText(a) : a[sort.column];
    const bv = sort.column === 'parent' ? getMasterParentText(b) : b[sort.column];
    const comparison = String(av || '').localeCompare(String(bv || ''),'id',{numeric:true,sensitivity:'base'});
    return sort.order === 'desc' ? -comparison : comparison;
  });
  return {all,sorted,search,sort};
}

function renderMasterTable() {
  const {all,sorted,search,sort} = getSortedMasters();
  const pages = Math.max(1,Math.ceil(sorted.length/masterPageSize));
  state.masterPage = Math.max(1,Math.min(state.masterPage || 1,pages));
  const rows = sorted.slice((state.masterPage-1)*masterPageSize,state.masterPage*masterPageSize);
  $('#master-count-chip').textContent = `${number(sorted.length)}${search?' dari '+number(all.length):''} referensi`;
  $('.master-table').innerHTML = `<table><thead><tr>${masterColumns.map(c => `<th><button type="button" class="th-sort-btn" data-action="master-sort" data-column="${c.key}" aria-label="Urutkan ${c.label}">${c.label} ${sort.column===c.key?(sort.order==='asc'?'▲':'▼'):'⇅'}</button></th>`).join('')}${admin('<th>Tindakan</th>')}</tr></thead><tbody>${rows.map(m => `<tr><td>${esc(m.code)}</td><td>${esc(m.label)}</td><td>${esc(getMasterParentText(m))}</td>${admin(`<td><button type="button" class="row-button" data-action="edit-master" data-id="${m.id}" aria-label="Edit ${esc(m.label)}">Edit</button> <button type="button" class="row-button error-text" data-action="delete-master" data-id="${m.id}" data-code="${esc(m.code)}" aria-label="Hapus ${esc(m.label)}">Hapus</button></td>`)}</tr>`).join('')}</tbody></table>${rows.length?'':empty('Referensi tidak ditemukan',search?'Coba kata kunci lain atau kosongkan pencarian.':'Tambahkan referensi melalui formulir di samping.')}`;
  $('#master-pagination').innerHTML = `<span>${number(sorted.length)} referensi · ${state.masterPage} / ${pages}</span><div class="actions"><button type="button" data-action="master-page" data-delta="-1" ${state.masterPage<=1?'disabled':''}>← Sebelumnya</button><button type="button" data-action="master-page" data-delta="1" ${state.masterPage>=pages?'disabled':''}>Berikutnya →</button></div>`;
  $('#master-sort-select').value = `${sort.column}-${sort.order}`;
}

function renderMasters(category=state.masterCategory || 'SKEMA') {
  state.masterCategory = category;
  $('#content').innerHTML = heading('Data referensi','Kelola pilihan yang digunakan pada formulir asesmen dan pemeriksaan impor Excel.') +
    `<div class="two-col master-layout"><section class="panel"><div class="toolbar master-toolbar"><select id="master-category" aria-label="Kategori master">${Object.entries(masterLabels).map(([key,label]) => `<option value="${key}" ${key===category?'selected':''}>${label}</option>`).join('')}</select><input id="master-search" type="search" aria-label="Cari referensi" placeholder="Cari kode, nama, atau induk…" value="${esc(state.masterSearch)}"><select id="master-sort-select" aria-label="Urutan data referensi">${masterColumns.map(c => `<option value="${c.key}-asc">${c.label} ↑</option><option value="${c.key}-desc">${c.label} ↓</option>`).join('')}</select><span class="chip" id="master-count-chip"></span></div><div class="master-table table-wrap"></div><div class="pagination" id="master-pagination"></div></section><section class="panel"><div class="panel-title"><h2 id="master-panel-title">${isAdmin()?'Tambah '+masterLabels[category].toLowerCase():'Tentang referensi'}</h2></div><div class="panel-body">${admin(`<form id="master-form"><input type="hidden" name="id" value="0"><input type="hidden" name="category" value="${category}"><label>Kode *<input name="code" required maxlength="100" placeholder="Kode sesuai sumber referensi"></label><label>Nama referensi *<input name="label" required maxlength="255" placeholder="Nama ${masterLabels[category].toLowerCase()}"></label>${renderMasterParentField(category)}<p id="master-error" class="error" role="alert"></p><div class="actions master-form-actions"><button class="primary" type="submit" id="master-form-submit">Simpan referensi</button><button type="button" id="master-form-cancel" class="hidden" data-action="cancel-master">Batal edit</button></div></form>`)}<div class="info-box"><strong>${esc(masterLabels[category])}</strong>Kode harus unik dalam kategori ini. Gunakan Edit untuk mengubah referensi yang sudah ada. Kode dan induk yang masih digunakan dilindungi agar data asesmen tetap terhubung.${category==='PENDIDIKAN'?'<br>Kode induk boleh dikosongkan untuk pendidikan.':''}${category==='SKEMA'?'<br>Penomoran sertifikat otomatis juga memerlukan metadata sektor, profesi, dan jenjang skema.':''}</div></div></section></div>`;
  renderMasterTable();
}

function resetMasterForm() {
  const form = $('#master-form');
  if (!form) return;
  form.reset();
  form.elements.namedItem('id').value = '0';
  $('#master-error').textContent = '';
  $('#master-panel-title').textContent = 'Tambah '+masterLabels[state.masterCategory].toLowerCase();
  $('#master-form-submit').textContent = 'Simpan referensi';
  $('#master-form-cancel').classList.add('hidden');
}

async function handleMasterAction(button) {
  const action = button.dataset.action;
  if (action === 'master-sort') {
    const column = button.dataset.column;
    state.masterSort = {column,order:state.masterSort.column===column && state.masterSort.order==='asc'?'desc':'asc'};
    state.masterPage = 1;
    renderMasterTable();
  } else if (action === 'master-page') {
    state.masterPage += Number(button.dataset.delta);
    renderMasterTable();
  } else if (action === 'edit-master') {
    const m = state.meta.masters.find(m => m.id === Number(button.dataset.id));
    const form = $('#master-form');
    if (!m || !form) return;
    for (const name of ['id','category','code','label','parent']) form.elements.namedItem(name).value = m[name] || '';
    $('#master-error').textContent = '';
    $('#master-panel-title').textContent = `Edit ${masterLabels[m.category].toLowerCase()} · ${m.code}`;
    $('#master-form-submit').textContent = 'Perbarui referensi';
    $('#master-form-cancel').classList.remove('hidden');
    form.elements.namedItem('label').focus();
    form.scrollIntoView({behavior:'smooth',block:'nearest'});
  } else if (action === 'cancel-master') {
    resetMasterForm();
  } else if (action === 'delete-master') {
    if (!confirm(`Hapus referensi ${button.dataset.code}? Referensi yang masih digunakan tidak dapat dihapus.`)) return;
    await api('/api/master/'+encodeURIComponent(button.dataset.id),{method:'DELETE'});
    state.meta = await api('/api/meta');
    renderMasters();
    toast('Referensi berhasil dihapus.');
  }
}

async function submitMaster(form,values) {
  $('#master-error').textContent = '';
  values.id = Number(values.id || 0);
  await api('/api/master',{method:'POST',body:values});
  state.meta = await api('/api/meta');
  state.masterSearch = '';
  state.masterPage = 1;
  renderMasters(values.category);
  toast(values.id?'Referensi berhasil diperbarui.':'Referensi baru berhasil disimpan.');
}
