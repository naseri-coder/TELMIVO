import type {Plugin} from './botfactory';
export interface Quota {allowed:boolean;code:string;usage:string;limit:{value:string;unlimited:boolean};bucket:string}
export interface StorePlugin extends Plugin {
 dependency_blocked:boolean;conflict_blocked:boolean;
 store:{metadata:{visible:boolean;category:string;summary:string;features:string;icon:string;operational:string};sale_enabled:boolean;trial_state:string;decision:{allowed:boolean;code:string;sources:string[]};quotas:Record<string,Quota>;contributions:{id:string;source:string;state:string;scope:string}[]}
}
export interface CatalogVersion {kind:string;key:string;version:number;enabled:boolean;effective_at:string;definition:Record<string,unknown>}
export type Catalog=Record<string,Record<string,CatalogVersion>>;
export const operationalLabel=(v:string)=>({OPERATIONAL:'آماده',MAINTENANCE:'در حال نگهداری',UNAVAILABLE:'خارج از دسترس'}[v]??v);
export const trialLabel=(v:string)=>({ELIGIBLE:'قابل شروع',ACTIVE:'فعال',GRACE:'مهلت تمدید',EXPIRED:'منقضی',CONSUMED:'استفاده شده',INELIGIBLE:'غیرقابل استفاده',CANCELLED:'لغو شده',SUSPENDED:'تعلیق شده'}[v]??v);
