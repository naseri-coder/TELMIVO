import {test,expect,type Page} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(id:number){
 const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),query_id:'r4d-browser-'+id,user:JSON.stringify({id,first_name:'پرداخت مرورگر'})});
 const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
 const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
 p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
 return p.toString();
}

// Browser/API contract scenarios. The native Stars journey above still exercises
// the disposable Go backend; these routes isolate provider state rendering and
// never claim to execute PostgreSQL or server-side payment verification.
type PaymentState='AWAITING_PAYMENT'|'PAID'|'FAILED'|'UNKNOWN'|'REFUNDED';
async function paymentBackend(page:Page,options:{external?:boolean;checkoutState?:PaymentState;initialState?:PaymentState}={}){
 const amount={asset:options.external?'USD':'XTR',amount_atomic:'7'};
 let state:PaymentState|undefined=options.initialState;
 let orderState=state==='PAID'||state==='REFUNDED'?'CONFIRMED':'PLACED';
 const effects={checkout:0,reconcile:0,refund:0};
 const unexpected:string[]=[];
 let releaseReconcile:()=>void=()=>{};
 const reconciliationEvidence=new Promise<void>(resolve=>{releaseReconcile=resolve});
 function payment(){return {id:'payment-r4d',order_id:'order-r4d',rail:options.external?'EXTERNAL':'TELEGRAM_STARS',provider:options.external?'bridge.test':'telegram',state,amount,revision:1,...(state==='AWAITING_PAYMENT'?{checkout_url:'https://payments.example.test/synthetic'}:{}),...(state==='FAILED'?{outcome_code:'CHECKOUT_REJECTED'}:{})}}
 await page.route('https://telegram.org/**',route=>route.abort());
 await page.route('**/api/v1/**',async route=>{
  const request=route.request(),url=new URL(request.url()),path=url.pathname;
  let data:unknown;
  if(path.endsWith('/auth/me'))data={user_id:'r4d-ui-user',name:'آزمایش پرداخت',is_admin:false,synthetic_mode:true};
  else if(path.endsWith('/workspaces'))data=[{id:'r4d-ui-workspace',name:'پرداخت آزمایشی',role_key:'OWNER'}];
  else if(path.endsWith('/auth/permissions'))data=['commerce.manage'];
  else if(path.endsWith('/definitions'))data={items:[]};
  else if(path.endsWith('/orders'))data={items:[{id:'order-r4d',contact:'contact-r4d',state:orderState,revision:1,total:amount,lines:[{product:'product-r4d',name:'محصول آزمایشی',product_revision:1,quantity:1,digital:!options.external,price:amount,total:amount}]}]};
  else if(path.endsWith('/history'))data=[{id:'history-r4d',state:orderState,created_at:'2026-10-01T00:00:00Z',cursor:'1'}];
  else if(path.endsWith('/payments')&&request.method()==='GET')data=state?[payment()]:[];
  else if(path.endsWith('/checkout')&&request.method()==='POST'){
   expect(request.postDataJSON().idempotency_key).toBeTruthy();
   expect(request.postDataJSON().rail).toBe(options.external?'EXTERNAL':'TELEGRAM_STARS');
   effects.checkout++;state=options.checkoutState??'AWAITING_PAYMENT';data=payment();
  }else if(path.endsWith('/reconcile')&&request.method()==='POST'){
   effects.reconcile++;
   await reconciliationEvidence;
   state='PAID';orderState='CONFIRMED';data=payment();
  }else if(path.endsWith('/refund')&&request.method()==='POST'){
   expect(request.postDataJSON().idempotency_key).toBeTruthy();
   effects.refund++;state='REFUNDED';data={id:'refund-r4d',payment_id:'payment-r4d',state:'REFUNDED',amount,revision:1};
  }else{
   unexpected.push(request.method()+' '+path);
   await route.fulfill({status:404,json:{error:{message:'مسیر آزمایشی تعریف نشده'}}});return;
  }
  await route.fulfill({json:data});
 });
 return {effects,unexpected,verifyPayment(){state='PAID';orderState='CONFIRMED'},releaseReconcile};
}

async function commerce(page:Page,width:number){
 await page.setViewportSize({width,height:844});
 await page.goto('/customer-bots/r4d-ui-bot/commerce');
 await expect(page.getByRole('heading',{name:'محصول و سفارش',exact:true})).toBeVisible();
 return page.getByRole('heading',{name:'order-r4d',exact:true}).locator('..');
}
async function rtl(page:Page){
 await expect(page.locator('html')).toHaveAttribute('dir','rtl');
 await expect(page.locator('html')).toHaveAttribute('lang','fa-IR');
 expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBeTruthy();
}

for(const width of [390,1280]){
 test(`R4D mock backend SUCCESS requires verified state ${width}`,async({page})=>{
  const backend=await paymentBackend(page);
  const card=await commerce(page,width);
  await card.getByRole('button',{name:'Checkout با Telegram Stars',exact:true}).click();
  await expect(card.getByText('AWAITING_PAYMENT · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  await expect(card.getByRole('link',{name:'باز کردن صفحه پرداخت'})).toBeVisible();
  await expect(card.getByText(/^PAID ·/)).toHaveCount(0);
  await expect(card.getByRole('button',{name:'FULFILLED',exact:true})).toHaveCount(0);
  await page.reload();
  await expect(card.getByText(/^AWAITING_PAYMENT ·/)).toBeVisible();
  backend.verifyPayment();
  await page.reload();
  await expect(card.getByText('PAID · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  await expect(card.getByText(/^CONFIRMED ·/)).toBeVisible();
  await expect(card.getByText(/^FULFILLED ·/)).toHaveCount(0);
  await expect(card.getByRole('button',{name:'بازپرداخت کامل',exact:true})).toBeVisible();
  expect(backend.effects).toEqual({checkout:1,reconcile:0,refund:0});
  expect(backend.unexpected).toEqual([]);await rtl(page);
 });
 test(`R4D mock backend FAILURE never fulfills ${width}`,async({page})=>{
  const backend=await paymentBackend(page,{checkoutState:'FAILED'});
  const card=await commerce(page,width);
  await card.getByRole('button',{name:'Checkout با Telegram Stars',exact:true}).click();
  await expect(card.getByText('FAILED · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  await expect(card.getByText('CHECKOUT_REJECTED',{exact:true})).toBeVisible();
  await page.reload();
  await expect(card.getByText(/^FAILED ·/)).toBeVisible();
  await expect(card.getByText(/^PLACED ·/)).toBeVisible();
  await expect(card.getByText(/^(PAID|CONFIRMED|FULFILLED) ·/)).toHaveCount(0);
  await expect(card.getByRole('button',{name:'بازپرداخت کامل',exact:true})).toHaveCount(0);
  expect(backend.effects).toEqual({checkout:1,reconcile:0,refund:0});
  expect(backend.unexpected).toEqual([]);await rtl(page);
 });
 test(`R4D mock backend UNKNOWN waits for reconciliation ${width}`,async({page})=>{
  const backend=await paymentBackend(page,{external:true,checkoutState:'UNKNOWN'});
  const card=await commerce(page,width);
  await card.getByLabel('کلید provider پرداخت خارجی').fill('bridge.test');
  await card.getByRole('button',{name:'ایجاد checkout خارجی',exact:true}).click();
  await expect(card.getByText(/^UNKNOWN · EXTERNAL \/ bridge.test ·/)).toBeVisible();
  await page.reload();
  await expect(card.getByText(/^UNKNOWN ·/)).toBeVisible();
  await expect(card.getByText(/^(PAID|CONFIRMED|FULFILLED) ·/)).toHaveCount(0);
  await card.getByRole('button',{name:'Reconcile',exact:true}).click();
  await expect.poll(()=>backend.effects.reconcile).toBe(1);
  await expect(card.getByText(/^UNKNOWN ·/)).toBeVisible();
  await expect(card.getByText(/^PAID ·/)).toHaveCount(0);
  backend.releaseReconcile();
  await expect(card.getByText(/^PAID · EXTERNAL \/ bridge.test ·/)).toBeVisible();
  await expect(card.getByText(/^CONFIRMED ·/)).toBeVisible();
  await page.reload();
  await expect(card.getByText(/^PAID ·/)).toBeVisible();
  await expect(card.getByRole('button',{name:'Reconcile',exact:true})).toHaveCount(0);
  expect(backend.effects).toEqual({checkout:1,reconcile:1,refund:0});
  expect(backend.unexpected).toEqual([]);await rtl(page);
 });
 test(`R4D mock backend REFUND reload never repeats effect ${width}`,async({page})=>{
  const backend=await paymentBackend(page,{initialState:'PAID'});
  const card=await commerce(page,width);
  await expect(card.getByText(/^PAID ·/)).toBeVisible();
  await card.getByRole('button',{name:'بازپرداخت کامل',exact:true}).click();
  await expect(card.getByText('REFUNDED · TELEGRAM_STARS / telegram · 7 XTR',{exact:true})).toBeVisible();
  for(let n=0;n<2;n++){
   await page.reload();
   await expect(card.getByText(/^REFUNDED ·/)).toBeVisible();
   await expect(card.getByText(/^PAID ·/)).toHaveCount(0);
   await expect(card.getByRole('button',{name:'بازپرداخت کامل',exact:true})).toHaveCount(0);
  }
  expect(backend.effects).toEqual({checkout:0,reconcile:0,refund:1});
  expect(backend.unexpected).toEqual([]);await rtl(page);
 });
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
