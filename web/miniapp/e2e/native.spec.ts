import {test,expect} from '@playwright/test';
import {createHmac} from 'node:crypto';
import labels from '../src/locales/fa/policies.json' with {type:'json'};
function signed(id:number){const p=new URLSearchParams({auth_date:String(Math.floor(Date.now()/1000)),user:JSON.stringify({id,first_name:'Synthetic Native'})});const check=[...p.entries()].sort(([a],[b])=>a.localeCompare(b)).map(([k,v])=>`${k}=${v}`).join('\n');const key=createHmac('sha256','WebAppData').update('synthetic-signing-only').digest();p.set('hash',createHmac('sha256',key).update(check).digest('hex'));return p.toString();}
test('native login, policy mutation, Preview equivalence and permission denial',async({page,browser})=>{
 page.on('request',request=>expect(new URL(request.url()).hostname).toBe('127.0.0.1'));
 await page.addInitScript(raw=>{window.Telegram={WebApp:{initData:raw,ready(){},expand(){}}};},signed(94000001));
 await page.goto('/plans');await expect(page.getByRole('heading',{name:labels.policyTitle})).toBeVisible();
 const wsResponse=await page.request.get('/api/v1/workspaces');expect(wsResponse.ok()).toBeTruthy();const ws=(await wsResponse.json())[0].id;
 const headers={'X-Workspace-ID':ws,Origin:'http://127.0.0.1:5173'};
 const perms=await page.request.get('/api/v1/auth/permissions');expect(await perms.json()).toContain('platform.access_policies.manage');
 await page.getByRole('tab',{name:labels.policyQuotas,exact:true}).click();
 const card=page.locator('.card').filter({has:page.getByRole('heading',{name:/^downloader_free /})});
 await card.getByRole('button',{name:labels.policyEdit,exact:true}).click();
 await page.getByLabel(labels.policyReason,{exact:true}).fill('native R2B browser mutation');
 const limit=page.getByLabel(labels.policyLimit,{exact:true});await limit.fill(String(Number(await limit.inputValue())+1));
 await page.getByRole('button',{name:labels.policySave,exact:true}).click();await expect(page.getByRole('status').filter({hasText:labels.policySaved})).toBeVisible();
 const policiesResponse=await page.request.get('/api/v1/access-policies?kind=QUOTA',{headers});const saved=(await policiesResponse.json()).find((p:{key:string})=>p.key==='downloader_free');expect(saved.reason).toBe('native R2B browser mutation');
 await page.getByRole('tab',{name:labels.policyPreview,exact:true}).click();await page.getByRole('button',{name:labels.policyInspect,exact:true}).click();await expect(page.locator('pre[role="status"]')).toBeVisible();
 const request={feature:'downloads',requested:1};const runtime=await page.request.post('/api/v1/access/decision',{headers,data:request});const preview=await page.request.post('/api/v1/access-policies/preview',{headers,data:request});expect(runtime.ok()).toBeTruthy();expect(preview.ok()).toBeTruthy();expect(await runtime.json()).toEqual(await preview.json());expect(JSON.parse(await page.locator('pre[role="status"]').innerText())).toEqual(await runtime.json());
 const ordinary=await browser.newContext();const login=await ordinary.request.post('http://127.0.0.1:5173/api/v1/auth/telegram',{headers:{Origin:headers.Origin},data:{init_data:signed(94000002)}});expect(login.ok()).toBeTruthy();const workspaces=await ordinary.request.get('http://127.0.0.1:5173/api/v1/workspaces');const other=(await workspaces.json())[0].id;
 const denied=await ordinary.request.post('http://127.0.0.1:5173/api/v1/access-policies',{headers:{...headers,'X-Workspace-ID':other},data:{...saved,reason:'unauthorized',expected_version:saved.version}});expect(denied.status()).toBe(403);await ordinary.close();
});
