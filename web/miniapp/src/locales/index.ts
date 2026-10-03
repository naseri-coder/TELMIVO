import common from './fa/common.json';
import policies from './fa/policies.json';
const fa={...common,...policies};
export type MessageKey=keyof typeof fa;
export function t(key:string):string{return fa[key as MessageKey]??fa.noData;}
export function formatAtomic(value:string,asset:string):string{const n=BigInt(value);if(asset==='USD'){const sign=n<0n?'-':'';const abs=n<0n?-n:n;return `${sign}${(abs/100n).toLocaleString('fa-IR')}.${(abs%100n).toString().padStart(2,'0')}`;}return n.toLocaleString('fa-IR');}
