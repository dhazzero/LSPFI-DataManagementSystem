// Isolated browser regression: API calls are mocked; no database is modified.
const { chromium } = require(process.env.PLAYWRIGHT_MODULE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
(async () => {
  const browser = await chromium.launch({headless:true, ...(process.env.CHROME_PATH ? {executablePath:process.env.CHROME_PATH} : {})});
  try {
    const page = await browser.newPage();
    const errors = [], saves = [];
    let role = 'admin';
    const fields = ['name','nik','birth_place','birth_date','gender','address','province','city','phone','email','company','education','occupation','scheme','registration','test_date','tuk','result','assessor','assessor_registration','schedule','funding','ministry','certificate','blanko','met_year','plenary_date'].map(key=>({key,label:key,kind:({province:1,city:1,education:1,occupation:1,scheme:1})[key]?'master':'',category:({province:'PROVINSI',city:'KABUPATEN',education:'PENDIDIKAN',occupation:'PEKERJAAN',scheme:'SKEMA'})[key]}));
    const categories = ['SKEMA','PENDIDIKAN','PEKERJAAN','PROVINSI','KABUPATEN','SUMBER_ANGGARAN','KEMENTERIAN'];
    const masters = categories.map((category,i)=>({id:i+1,category,code:String(i+1),label:category+' awal',parent:category==='KABUPATEN'?'4':''}));
    page.on('pageerror', e=>errors.push(e.message));
    await page.route('http://dms.test/**', async route=>{
      const p = new URL(route.request().url()).pathname;
      if(p==='/api/test-invalid-json') return route.fulfill({status:502,contentType:'text/html',body:'Bad gateway'});
      if(p==='/api/test-offline') return route.abort('connectionrefused');
      if(p==='/api/me') return route.fulfill({json:{username:'test-user',role}});
      if(p==='/api/meta') return route.fulfill({json:{masters,fields}});
      if(p==='/api/master') {
        const row=route.request().postDataJSON();
        if(row.code==='DUP') return route.fulfill({status:409,json:{error:'Kode sudah digunakan. Gunakan Edit.'}});
        saves.push(row);
        assert.equal(typeof row.id,'number');
        const existing=masters.find(m=>m.id===row.id);
        if(existing) Object.assign(existing,row); else masters.push({...row,id:masters.length+1});
        return route.fulfill({json:{ok:true}});
      }
      const file=path.join(__dirname,'../web',p==='/'?'index.html':p);
      if(fs.existsSync(file)) return route.fulfill({path:file});
      return route.fulfill({json:[]});
    });
    await page.goto('http://dms.test/#masters');
    const field=name=>page.locator(`#master-form [name="${name}"]`);
    for(const category of categories) {
      await page.locator('#master-category').selectOption(category);
      await page.locator('[data-action="edit-master"]').first().click();
      await field('label').fill(category+' diperbarui');
      await page.locator('#master-form-submit').click();
      await page.waitForFunction(label=>[...document.querySelectorAll('.master-table td')].some(e=>e.textContent===label),category+' diperbarui',{timeout:3000});
      assert.equal(saves.at(-1).category,category);
      assert.ok(saves.at(-1).id>0);
      await field('code').fill('NEW-'+category);
      await field('label').fill(category+' baru');
      if(category==='KABUPATEN') await field('parent').selectOption('4');
      await page.locator('#master-form-submit').click();
      await page.waitForFunction(label=>[...document.querySelectorAll('.master-table td')].some(e=>e.textContent===label),category+' baru',{timeout:3000});
      assert.equal(saves.at(-1).id,0);
      await page.locator('[data-action="edit-master"]').first().click();
      await page.locator('#master-form-cancel').click();
      assert.equal(await field('id').inputValue(),'0');
    }
    assert.equal(saves.length,14);
    await page.locator('#master-category').selectOption('PENDIDIKAN');
    await field('code').fill('DUP');
    await field('label').fill('Data belum tersimpan');
    await page.locator('#master-form-submit').click();
    await page.locator('#master-error').filter({hasText:'Kode sudah digunakan'}).waitFor();
    assert.equal(await field('label').inputValue(),'Data belum tersimpan');
    assert.equal(await page.locator('#master-form-submit').isEnabled(),true);
    // Navigation remembers the selected category; searching does not destroy the edit form.
    await page.locator('[data-nav="settings"]').click();
    await page.locator('[data-nav="masters"]').click();
    await page.waitForFunction(()=>document.querySelector('#master-category')?.value==='PENDIDIKAN');
    for(let i=0;i<120;i++) masters.push({id:100+i,category:'PENDIDIKAN',code:'BULK'+i,label:'Pendidikan '+i,parent:''});
    await page.reload();
    await page.locator('#master-category').selectOption('PENDIDIKAN');
    assert.equal(await page.locator('.master-table tbody tr').count(),50);
    await page.locator('[data-action="master-page"][data-delta="1"]').click();
    assert.match(await page.locator('#master-pagination').innerText(),/2 \/ 3/);
    await page.locator('#master-search').fill('BULK119');
    assert.equal(await page.locator('.master-table tbody tr').count(),1);
    await page.locator('#master-category').selectOption('PROVINSI');
    assert.equal(await page.locator('#master-search').inputValue(),'');
    masters.push({id:500,category:'PROVINSI',code:'32',label:'Jawa Barat',parent:''}, {id:501,category:'KABUPATEN',code:'3201',label:'Bogor',parent:'32'});
    await page.locator('[data-nav="dashboard"]').click();
    await page.locator('[data-action="new"]').click();
    const province=page.locator('#record-form [name="province"]');
    const city=page.locator('#record-form [name="city"]');
    await province.selectOption('32');
    await city.selectOption('3201');
    assert.equal(await city.locator('option[value="5"]').count(),0);
    await province.selectOption('4');
    assert.equal(await city.inputValue(),'');
    assert.equal(await city.locator('option[value="3201"]').count(),0);
    await page.locator('[data-action="close"]').click();
    assert.match(await page.evaluate(()=>api('/api/test-invalid-json').catch(e=>e.message)),/Respons server.*502/);
    assert.match(await page.evaluate(()=>api('/api/test-offline').catch(e=>e.message)),/Tidak dapat terhubung/);
    await page.locator('[data-nav="masters"]').click();
    await page.locator('#master-category').selectOption('PENDIDIKAN');
    if(process.env.LSPFI_BROWSER_TEST_SCREENSHOT) await page.screenshot({path:process.env.LSPFI_BROWSER_TEST_SCREENSHOT,fullPage:true});
    await page.setViewportSize({width:390,height:844});
    assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth),true,'reference mobile overflow');
    await page.setViewportSize({width:1280,height:720});
    role='viewer';
    await page.goto('http://dms.test/#masters');
    await page.reload();
    await page.locator('#master-category').waitFor();
    assert.equal(await page.locator('#master-form').count(),0);
    assert.equal(await page.locator('[data-action="edit-master"]').count(),0);
    assert.deepEqual(errors,[]);
    console.log('PASS: reference CRUD UI, errors, pagination, navigation, province/city selection and viewer access');
  } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exitCode=1;});
