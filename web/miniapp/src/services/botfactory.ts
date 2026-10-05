import {api} from './api';
export interface Money{unit:string;scale:number;amount:string}
export interface Setting{type:'string'|'integer'|'boolean';max_length?:number;enum?:string[];minimum?:number;maximum?:number}
export interface Plugin{id:string;name:string;description:string;version:string;scope:string;commercial_status:string;entitled:boolean;reason:string;included:boolean;plan:string;prices?:{daily:Money;monthly:Money};trial_eligible:boolean;trial_seconds:number;dependencies:string[];conflicts:string[];state:string;revision:number;settings:{properties:Record<string,Setting>;required?:string[]};configuration?:Record<string,unknown>;installable:boolean}
export interface Bot{id:string;name:string;status:string}
export interface BotRuntime{bot_id:string;username:string;status:string;ingress_path:string;connected:boolean}
export interface Selection{requested:string[];configurations:Record<string,Record<string,unknown>>;activate:boolean}
export interface Review{requested:string[];required:string[];order:string[];plugins:Plugin[];issues:{code:string;plugin:string;other?:string}[];ready:boolean;digest:string}
// Exact denomination/scale from the commercial catalog, never floats or invented FX.
export function commercialMoney(m:Money){const amount=BigInt(m.amount);const unit=10n**BigInt(m.scale);return `${amount/unit}${m.scale?'.'+(amount%unit).toString().padStart(m.scale,'0'):''} ${m.unit}`;}
export const reviewSelection=(selection:Selection,bot?:string)=>api<Review>(`/customer-bots/${bot?bot+'/':''}preview`,'POST',selection);
export function defaultConfiguration(p:Plugin){return Object.fromEntries(Object.entries(p.settings.properties??{}).map(([key,s])=>[key,s.type==='boolean'?false:s.type==='integer'?(s.minimum??0):(s.enum?.[0]??'')]));}
