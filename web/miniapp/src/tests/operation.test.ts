import {beforeEach,describe,it,expect,vi} from 'vitest';
import {operationKey,completeOperation} from '../services/operation';
import {mutation,setWorkspace} from '../services/api';
beforeEach(()=>{sessionStorage.clear();vi.restoreAllMocks();setWorkspace('A');});
describe('operation idempotency',()=>{
 it('reuses persisted key after a lost response and component reload',async()=>{const a=await operationKey('A:wallet',{amount:'100'});expect(await operationKey('A:wallet',{amount:'100'})).toBe(a);completeOperation('A:wallet');expect(await operationKey('A:wallet',{amount:'100'})).not.toBe(a);});
 it('separates workspaces and genuinely changed actions',async()=>{const a=await operationKey('A',{body:'hello'});expect(await operationKey('B',{body:'hello'})).not.toBe(a);expect(await operationKey('A',{body:'changed'})).not.toBe(a);});
 it('retry after unknown response sends same key; confirmed success permits a new action',async()=>{const bodies:string[]=[];vi.stubGlobal('fetch',vi.fn(async(_url,init)=>{bodies.push(init.body);if(bodies.length===1)throw new TypeError('connection lost');return {ok:true,json:async()=>({saved:true})};}));await expect(mutation('/wallets/adjustments',{amount_atomic:'100'})).rejects.toThrow();await mutation('/wallets/adjustments',{amount_atomic:'100'});await mutation('/wallets/adjustments',{amount_atomic:'100'});expect(JSON.parse(bodies[0]).idempotency_key).toBe(JSON.parse(bodies[1]).idempotency_key);expect(JSON.parse(bodies[2]).idempotency_key).not.toBe(JSON.parse(bodies[1]).idempotency_key);});
});
it('captures the intended workspace before asynchronous key generation',async()=>{let workspace='';vi.stubGlobal('fetch',vi.fn(async(_url,init)=>{workspace=init.headers['X-Workspace-ID'];return {ok:true,json:async()=>({saved:true})};}));setWorkspace('A');const result=mutation('/wallets/adjustments',{amount_atomic:'100'});setWorkspace('B');await result;expect(workspace).toBe('A');});
