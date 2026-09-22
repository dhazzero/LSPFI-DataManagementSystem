'use strict';

function options(category,value='',placeholder='Pilih referensi',parent=null) {
  const rows=state.meta.masters.filter(m=>m.category===category && (parent===null || !m.parent || m.parent===parent));
  const missing=value && !rows.some(m=>m.code===value);
  return `<option value="">${esc(placeholder)}</option>`+
    (missing?`<option value="${esc(value)}" selected>${esc(value)} · Periksa referensi / provinsi</option>`:'')+
    rows.map(m=>`<option value="${esc(m.code)}" ${m.code===value?'selected':''}>${esc(m.code)} · ${esc(m.label)}</option>`).join('');
}

function assessmentOptions(field,value) {
  if(field.key!=='city') return options(field.category,value);
  const province=state.detail.fields.province || '';
  return options('KABUPATEN',value,province?'Pilih kabupaten / kota':'Pilih provinsi terlebih dahulu',province);
}

function changeAssessmentProvince(province) {
  const city=document.querySelector('#record-form [name="city"]');
  if(!city) return;
  const current=state.meta.masters.find(m=>m.category==='KABUPATEN' && m.code===city.value);
  const keep=current && (!current.parent || current.parent===province) ? city.value : '';
  city.innerHTML=options('KABUPATEN',keep,province?'Pilih kabupaten / kota':'Pilih provinsi terlebih dahulu',province);
  state.detail.fields.province=province;
  state.detail.fields.city=keep;
}

document.addEventListener('change',event=>{
  if(event.target.matches('#record-form [name="province"]')) changeAssessmentProvince(event.target.value);
});
