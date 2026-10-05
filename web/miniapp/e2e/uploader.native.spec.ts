import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(width:number){
  const p=new URLSearchParams({
    auth_date:String(Math.floor(Date.now()/1000)),
    query_id:'uploader-browser-'+width+'-'+Date.now(),
    user:JSON.stringify({id:98010001,first_name:'مدیر آپلودر'})
  });
  const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
  const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
  p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
  return p.toString();
}

for(const width of [390,1280]){
  test(`uploader private lifecycle ${width}`,async({page})=>{
    await page.setViewportSize({width,height:844});
    await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}};},signed(width));
    await page.goto('/uploader');
    await expect(page.getByRole('heading',{name:'آپلودر و فضای فایل',exact:true})).toBeVisible();

    const workflow='گردش‌کار مرورگر '+width;
    await page.getByLabel('نام گردش‌کار',{exact:true}).fill(workflow);
    await page.getByLabel('نوع‌های مجاز (با کاما جدا کنید)',{exact:true}).fill('text/plain');
    await page.getByLabel('حداکثر اندازه هر فایل (بایت)',{exact:true}).fill('1048576');
    await page.getByLabel('مدت نگه‌داری (روز)',{exact:true}).fill('7');
    await Promise.all([
      page.waitForResponse(r=>new URL(r.url()).pathname==='/api/v1/uploader/workflows'&&r.request().method()==='POST'&&r.status()===200),
      page.getByRole('button',{name:'ذخیره گردش‌کار',exact:true}).click()
    ]);
    await page.getByLabel('گردش‌کار',{exact:true}).selectOption({label:workflow});

    const name='browser-'+width+'.txt';
    await page.getByLabel('فایل',{exact:true}).setInputFiles({name,mimeType:'text/plain',buffer:Buffer.from('TELMIVO R4C '+width)});
    await Promise.all([
      page.waitForResponse(r=>/\/api\/v1\/uploader\/uploads\/[^/]+\/content$/.test(new URL(r.url()).pathname)&&r.request().method()==='PUT'&&r.status()===200),
      page.getByRole('button',{name:'شروع آپلود',exact:true}).click()
    ]);

    const card=page.locator('section.card').filter({has:page.getByRole('heading',{name,exact:true})});
    await expect(card.getByText('در انتظار تأیید',{exact:true})).toBeVisible();
    await Promise.all([
      page.waitForResponse(r=>/\/api\/v1\/uploader\/uploads\/[^/]+\/actions$/.test(new URL(r.url()).pathname)&&r.request().method()==='POST'&&r.status()===200),
      card.getByRole('button',{name:'تأیید',exact:true}).click()
    ]);
    await expect(card.getByText('ذخیره‌شده',{exact:true})).toBeVisible();
    await expect(card).toContainText('text/plain');
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();
    await page.screenshot({path:`test-results/uploader-${width}.png`,fullPage:true});
  });
}
