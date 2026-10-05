import {operationKey,completeOperation} from './operation';
export interface User{user_id:string;name:string;is_admin:boolean;synthetic_mode:boolean;can_adjust_wallet:boolean;can_admin_policies:boolean;can_admin_commercial?:boolean;}
export interface Workspace{id:string;name:string;role_key:string;}
export interface Decision{allowed:boolean;reason:string;limit?:number;usage:number;}
export type Row=Record<string,string|number|boolean|null>;
export class APIError extends Error{constructor(public status:number,message:string){super(message);}}
let workspaceID='';
export function setWorkspace(id:string){workspaceID=id;}
export async function api<T>(path:string,method='GET',body?:unknown,requestWorkspace=workspaceID):Promise<T>{const res=await fetch(`/api/v1${path}`,{method,credentials:'same-origin',headers:{'Content-Type':'application/json',...(requestWorkspace?{'X-Workspace-ID':requestWorkspace}:{})},...(body===undefined?{}:{body:JSON.stringify(body)})});const data=await res.json();if(!res.ok)throw new APIError(res.status,data.error?.message??'');return data as T;}
declare global{interface Window{Telegram?:{WebApp?:{initData:string;ready:()=>void;expand:()=>void}}}}
let bootPromise:Promise<{user:User;workspaces:Workspace[]}>|undefined;
export function bootstrap(){if(!bootPromise)bootPromise=(async()=>{let user:User;try{user=await api<User>('/auth/me');}catch(e){if(!(e instanceof APIError)||e.status!==401)throw e;const initData=window.Telegram?.WebApp?.initData;if(!initData)throw e;await api('/auth/telegram','POST',{init_data:initData});user=await api<User>('/auth/me');}const workspaces=await api<Workspace[]>('/workspaces');if(workspaces[0])setWorkspace(workspaces[0].id);window.Telegram?.WebApp?.ready();window.Telegram?.WebApp?.expand();return {user,workspaces};})();return bootPromise;}
export async function mutation<T>(path:string,body:Record<string,unknown>):Promise<T>{const target=workspaceID;const scope=target+':'+path;const idempotency_key=await operationKey(scope,body);const result=await api<T>(path,'POST',{...body,idempotency_key},target);completeOperation(scope);return result;}
export async function uploadBinary<T>(path:string,file:File):Promise<T>{const res=await fetch(`/api/v1${path}`,{method:'PUT',credentials:'same-origin',headers:{'X-Workspace-ID':workspaceID,'Content-Type':file.type||'application/octet-stream'},body:file});const data=await res.json();if(!res.ok)throw new APIError(res.status,data.error?.message??'');return data as T;}
export async function downloadBinary(path:string):Promise<Blob>{const res=await fetch(`/api/v1${path}`,{credentials:'same-origin',headers:{'X-Workspace-ID':workspaceID}});if(!res.ok){let message='';try{const data=await res.json();message=data.error?.message??'';}catch{}throw new APIError(res.status,message);}return res.blob();}

