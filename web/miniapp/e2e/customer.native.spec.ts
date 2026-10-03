import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';
function signed(){const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),query_id:'r3a-browser',user:JSON.stringify({id:95000001,first_name:'مشتری R3A'})});const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();p.set('hash',createHmac('sha256',key).update(check).digest('hex'));return p.toString();}
test.use({viewport:{width:390,height:844}});
test('real Persian mobile customer journey, wallet pagination, settings and persistent workspace',async({page})=>{
 page.on('request',request=>expect(new URL(request.url()).hostname).toBe('127.0.0.1'));
 await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}};},signed());
 await page.goto('/');await expect(page.getByRole('heading',{name:'مدیریت کسب‌وکارت، یک‌جا.'})).toBeVisible();
 await expect(page.locator('html')).toHaveAttribute('dir','rtl');await expect(page.locator('html')).toHaveAttribute('lang','fa-IR');
 const original=await (await page.request.get('/api/v1/auth/me')).json();expect(original.is_admin).toBe(false);expect(original.can_admin_policies).toBe(false);
 const workspaces=await (await page.request.get('/api/v1/workspaces')).json();expect(workspaces).toHaveLength(2);const first=workspaces[0].id;const second=workspaces.find((w:{name:string})=>w.name==='فضای دوم R3A').id;
 await page.getByRole('button',{name:'همه',exact:true}).click();await expect(page.getByRole('link',{name:'🛡 پنل مدیریت',exact:true})).toHaveCount(0);await page.getByLabel('فضای کاری',{exact:true}).selectOption(second);
 await expect(page.locator('.stat-number').filter({hasText:'۱'})).toBeVisible();const headers={'X-Workspace-ID':second,Origin:'http://127.0.0.1:5173'};
 const summary=await page.request.get('/api/v1/customer/overview',{headers});expect(summary.ok()).toBeTruthy();expect((await summary.json()).groups).toBe(1);
 const navigate=async(name:string)=>{await page.getByRole('button',{name:'همه',exact:true}).click();await page.getByRole('navigation',{name:'منوی مشتری'}).getByRole('link',{name,exact:true}).click();};
 await navigate('پلن‌ها و دسترسی‌ها');await expect(page.getByRole('heading',{name:'پلن‌های قابل بررسی'})).toBeVisible();
 const access=await page.request.get('/api/v1/customer/access',{headers});const runtime=await page.request.post('/api/v1/access/decision',{headers,data:{feature:'downloads'}});expect(await access.json()).toEqual(await runtime.json());
 await navigate('کیف پول و امور مالی');await expect(page.locator('.balance-number').filter({hasText:'۲۱.00'})).toBeVisible();await expect(page.locator('tbody tr')).toHaveCount(20);await page.getByRole('button',{name:'نمایش سوابق بیشتر'}).click();await expect(page.locator('tbody tr')).toHaveCount(21);await expect(page.getByRole('button',{name:'نمایش سوابق بیشتر'})).toHaveCount(0);await expect(page.getByRole('button',{name:'اصلاح اعتبار آزمایشی'})).toHaveCount(0);await page.screenshot({path:'test-results/r3a-mobile-wallet.png',fullPage:true});
 await navigate('تنظیمات');await page.getByLabel('نام فضای کاری').fill('فضای دوم ذخیره‌شده');await page.getByRole('button',{name:'ذخیره تغییرات',exact:true}).click();await expect(page.getByRole('status')).toHaveText('تغییرات ذخیره شد.');
 await page.reload();await expect(page.getByLabel('نام فضای کاری')).toHaveValue('فضای دوم ذخیره‌شده');expect((await (await page.request.get('/api/v1/auth/me')).json()).user_id).toBe(original.user_id);
 await page.getByRole('button',{name:'همه',exact:true}).click();await expect(page.getByLabel('فضای کاری',{exact:true})).toHaveValue(second);await page.getByRole('link',{name:'داشبورد',exact:true}).click();await expect(page.locator('.stat-number').filter({hasText:'۱'})).toBeVisible();
 const denied=await page.request.get('/api/v1/access-policies?kind=TRIAL',{headers});expect(denied.status()).toBe(403);await page.goto('/admin/policies');await expect(page.getByRole('alert')).toContainText('برای این عملیات دسترسی لازم را ندارید.');
 await page.goto('/');await page.getByRole('button',{name:'همه',exact:true}).click();await page.getByLabel('فضای کاری',{exact:true}).selectOption(first);await expect(page.locator('.stat-number')).toHaveText(['۰','۰','۰']);
 await page.reload();await expect(page.locator('.stat-number')).toHaveText(['۰','۰','۰']);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=window.innerWidth)).toBeTruthy();
 await page.screenshot({path:'test-results/r3a-mobile-dashboard.png',fullPage:true});
});
