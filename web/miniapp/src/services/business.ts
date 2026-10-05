export interface Money{asset:string;amount_atomic:string}
const scales:Record<string,number>={IRR:0,USD:2,USDT_TON:6,TON:9,XTR:0};
export function moneyLabel(m:Money){if(!/^(0|[1-9][0-9]*)$/.test(m.amount_atomic)||!(m.asset in scales))throw new Error('مبلغ نامعتبر');const s=scales[m.asset],v=m.amount_atomic.padStart(s+1,'0');return `${s?v.slice(0,-s)+'.'+v.slice(-s):v} ${m.asset}`}
export interface Definition<T>{id:string;name:string;revision:number;definition:T}
export interface Page<T>{items:T[];next_cursor:string}
export interface FormField{key:string;label:string;type:string;required:boolean;max_length:number;min:string;max:string;options:string[];crm_field:string;tags:Record<string,string>}
export interface FormDefinition{name:string;enabled:boolean;access:string;policy:string;fields:FormField[]}
export const emptyField=():FormField=>({key:'field',label:'پاسخ',type:'TEXT',required:true,max_length:200,min:'0',max:'100',options:[],crm_field:'',tags:{}});
