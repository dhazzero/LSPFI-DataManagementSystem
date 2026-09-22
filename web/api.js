'use strict';

async function api(path, options={}) {
  const request = {...options};
  if(request.body && !(request.body instanceof FormData)) {
    request.headers = {'Content-Type':'application/json',...request.headers};
    request.body = JSON.stringify(request.body);
  }
  let response;
  try {
    response = await fetch(path,request);
  } catch {
    throw new Error('Tidak dapat terhubung ke aplikasi. Pastikan server masih berjalan, lalu coba kembali.');
  }
  if(response.status===401 && path!=='/api/login') showLogin();
  let data;
  try {
    data = await response.json();
  } catch {
    throw new Error(`Respons server tidak dapat dibaca (HTTP ${response.status}). Muat ulang aplikasi atau periksa log server.`);
  }
  if(!response.ok) {
    const issues = Array.isArray(data?.issues) ? data.issues : [];
    throw new Error([data?.error || `Permintaan gagal (HTTP ${response.status}).`,...issues].join('\n'));
  }
  return data;
}
