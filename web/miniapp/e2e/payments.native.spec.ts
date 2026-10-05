import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(id:number){
 const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),query_id:'r4d-browser-'+id,user:JSON.stringify({id,first_name:'پرداخت مرورگر'})});
 const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
 const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
 p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
 return p.toString();
}

for(const width of [390,1280]){
 test(`R4D Stars checkout Mini App ${width}`,async({page})=>{
  await page.setViewportSize({width,height:844});
  await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}}},signed(98012001));
  await page.goto('/customer-bots');
  const card=page.getByRole('heading',{name:'R4D Payments browser bot',exact:true}).locator('..');
  await card.getByRole('link',{name:'مدیریت پلاگین‌ها',exact:true}).click();
  const base=new URL(page.url()).pathname;
  await page.goto(base+'/commerce');
  await expect(page.getByRole('heading',{name:'محصول و سفارش',exact:true})).toBeVisible();
  await expect(page.getByText('محصول ستاره‌ای مرورگر',{exact:true})).toBeVisible();
  await expect(page.getByRole('button',{name:'Checkout با Telegram Stars',exact:true})).toBeVisible();
  await page.getByRole('button',{name:'Checkout با Telegram Stars',exact:true}).click();
  await expect(page.getByText('AWAITING_PAYMENT · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  await expect(page.getByRole('link',{name:'باز کردن صفحه پرداخت',exact:true})).toHaveAttribute('href','https://t.me/$synthetic_invoice');
  await page.reload();
  await expect(page.getByText('AWAITING_PAYMENT · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
 });
}
