import {useEffect,useState} from 'react';
import {api} from './api';
import {useSession} from '../app/context';
export interface Resource{limit:number;used:number;reserved:number;remaining:number;unit:string;window:string;reset_at?:string;safety_cap?:number;policy_key:string;policy_version:number}
export interface Access{allowed:boolean;reason:string;feature:string;trials:{key:string;version:number;state:string;ends_at:string;fallback:string}[];resources:Record<string,Resource>;paywall?:string;offers:{key:string;version:number;definition:{destination:string;price_key:string;price_version:number}}[]}
export interface Overview{workspace_id:string;name:string;plan:string;plan_label:string;channels:number;groups:number;bots:number;access:Access}
export interface Balance{asset:string;balance_atomic:string}
export interface History{items:{id:string;asset:string;amount_atomic:string;reason:string;created_at:string}[];next_cursor:string}
export interface Settings{name:string;can_edit:boolean}
export function useCustomer<T>(path:string){const {workspace}=useSession();const [data,setData]=useState<T>();const [error,setError]=useState('');const [attempt,setAttempt]=useState(0);useEffect(()=>{let active=true;api<T>(path,'GET',undefined,workspace.id).then(value=>{if(active){setData(value);setError('');}}).catch(e=>{if(active)setError(e instanceof Error?e.message:'');});return()=>{active=false;};},[path,workspace.id,attempt]);return {data,error,retry:()=>{setData(undefined);setError('');setAttempt(a=>a+1);}};}
