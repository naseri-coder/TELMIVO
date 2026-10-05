import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(){
  const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),query_id:'ai2-browser',user:JSON.stringify({id:98014001,first_name:'دستیار دو'})});
  const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
  const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
  p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
  return p.toString();
}

for(const width of [390,1280])test(`AI Assistant 2.0 journey ${width}`,async({page})=>{
  await page.setViewportSize({width,height:844});
  await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}}},signed());
  await page.goto('/customer-bots');
  await page.getByRole('heading',{name:'R4F browser bot',exact:true}).locator('..').getByRole('link',{name:'مدیریت پلاگین‌ها',exact:true}).click();
  const base=new URL(page.url()).pathname;
  await page.goto(base+'/assistant');

  await expect(page.getByRole('heading',{name:'دستیار هوش مصنوعی',exact:true})).toBeVisible();
  await expect(page.getByText(/Assistant 2\.0 · Hybrid retrieval · bounded cost controls/)).toBeVisible();
  await expect(page.getByLabel('مدل تأییدشده',{exact:true})).toHaveValue(/.+/);
  await expect(page.getByText(/ورودی\/خروجی:/)).toBeVisible();

  await page.getByLabel('روش بازیابی دانش',{exact:true}).selectOption('HYBRID');
  await page.getByLabel('حداکثر قطعه دانش در هر درخواست',{exact:true}).fill('3');
  await page.getByLabel('سقف هزینه هر درخواست (microunit، صفر = بدون سقف محلی)',{exact:true}).fill('100000');
  await page.getByLabel('R4F browser knowledge',{exact:true}).check();
  await page.getByRole('button',{name:'ذخیره دستیار',exact:true}).click();

  await page.reload();
  await expect(page.getByLabel('روش بازیابی دانش',{exact:true})).toHaveValue('HYBRID');
  await expect(page.getByLabel('حداکثر قطعه دانش در هر درخواست',{exact:true})).toHaveValue('3');
  await expect(page.getByLabel('سقف هزینه هر درخواست (microunit، صفر = بدون سقف محلی)',{exact:true})).toHaveValue('100000');
  await expect(page.getByLabel('R4F browser knowledge',{exact:true})).toBeChecked();

  await expect(page.getByRole('heading',{name:'ورود دانش از TELMIVO',exact:true})).toBeVisible();
  await expect(page.getByLabel('نوع منبع',{exact:true})).toHaveValue('FILE_MANAGER');
  await expect(page.getByLabel('مراجع مجاز برای ابزار file.metadata (هر خط یک reference)',{exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  await page.screenshot({path:`test-results/ai2-${width}.png`,fullPage:true});
});
