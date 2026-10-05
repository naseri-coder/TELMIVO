import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(){
  const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),query_id:'automation2-browser',user:JSON.stringify({id:98013001,first_name:'اتوماسیون دو'})});
  const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
  const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
  p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
  return p.toString();
}

for(const width of [390,1280])test(`Automation 2.0 schedule journey ${width}`,async({page})=>{
  await page.setViewportSize({width,height:844});
  await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}}},signed());
  await page.goto('/customer-bots');
  await page.getByRole('heading',{name:'R4E browser bot',exact:true}).locator('..').getByRole('link',{name:'مدیریت پلاگین‌ها',exact:true}).click();
  const base=new URL(page.url()).pathname;
  await page.goto(base+'/automation');
  await page.getByLabel('نام گردش کار',{exact:true}).fill('Cron browser '+width);
  await page.getByLabel('نوع زمان‌بندی',{exact:true}).selectOption('CRON');
  await page.getByLabel('Cron expression',{exact:true}).fill('*/30 * * * *');
  await page.getByLabel('Timezone',{exact:true}).fill('UTC');
  await page.getByLabel('شناسه تلگرام هدف',{exact:true}).fill('98013001');
  await page.getByLabel('نوع اقدام',{exact:true}).selectOption('telegram.send');
  await page.getByLabel('مقدار اقدام',{exact:true}).fill('پیام زمان‌بندی‌شده');
  await page.getByLabel('فعال',{exact:true}).check();
  await page.getByRole('button',{name:'Preview بدون تغییر',exact:true}).click();
  await expect(page.getByRole('status').filter({hasText:'"mutated":false'})).toBeVisible();
  await page.getByRole('button',{name:'ذخیره گردش کار',exact:true}).click();
  const card=page.getByRole('heading',{name:'Cron browser '+width,exact:true}).locator('..');
  await expect(card.getByText(/schedule\.cron — CRON — فعال/)).toBeVisible();
  await card.getByRole('button',{name:'ویرایش',exact:true}).click();
  await expect(page.getByLabel('Cron expression',{exact:true})).toHaveValue('*/30 * * * *');
  await expect(page.getByLabel('شناسه تلگرام هدف',{exact:true})).toHaveValue('98013001');
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
  await page.screenshot({path:`test-results/automation2-${width}.png`,fullPage:true});
});
