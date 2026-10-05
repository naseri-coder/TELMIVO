import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';

function signed(){
  const p=new URLSearchParams({
    auth_date:String(Math.floor(Date.now()/1000)),
    query_id:'channels-browser-'+Date.now(),
    user:JSON.stringify({id:98009001,first_name:'مدیر کانال'})
  });
  const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');
  const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();
  p.set('hash',createHmac('sha256',key).update(check).digest('hex'));
  return p.toString();
}

for(const width of [390,1280]){
  test(`channel manager draft journey ${width}`,async({page})=>{
    await page.setViewportSize({width,height:844});
    await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}};},signed());
    await page.goto('/channels');
    await expect(page.getByRole('heading',{name:'مدیریت کانال‌ها',exact:true})).toBeVisible();
    await expect(page.getByRole('heading',{name:'کانال مرورگر',exact:true})).toBeVisible();
    await page.getByRole('link',{name:'مدیریت کانال',exact:true}).click();
    await expect(page.getByRole('heading',{name:'کانال مرورگر',exact:true})).toBeVisible();
    const body='پیش‌نویس مرورگر '+width;
    await page.getByLabel('متن پست',{exact:true}).fill(body);
    await Promise.all([
      page.waitForResponse(r=>/\/api\/v1\/channels\/[^/]+\/posts$/.test(new URL(r.url()).pathname)&&r.request().method()==='POST'&&r.status()===200),
      page.getByRole('button',{name:'ذخیره پیش‌نویس',exact:true}).click()
    ]);
    await expect(page.getByText(body,{exact:true})).toBeVisible();
    await expect(page.getByText('پیش‌نویس',{exact:true})).toBeVisible();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();
    await page.screenshot({path:`test-results/channels-${width}.png`,fullPage:true});
  });
}
