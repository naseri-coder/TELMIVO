import faContent from './fa/content-plugins.json';
import factory from './fa/bot-factory.json';
import groups from './fa/group-product.json';
import customer from './fa/customer.json';
import common from './fa/common.json';
import policies from './fa/policies.json';
const fa={...faContent,...factory,...common,...policies,...customer,...groups};
export type MessageKey=keyof typeof fa;
export function t(key:string):string{return fa[key as MessageKey]??fa.noData;}
export function formatAtomic(value:string,asset:string):string{const scales:Record<string,number>={IRR:0,USD:2,USDT_TON:6,TON:9,XTR:0};const scale=scales[asset];if(scale===undefined)throw Error('Unsupported asset');const n=BigInt(value);const sign=n<0n?'-':'';const abs=n<0n?-n:n;const base=10n**BigInt(scale);return `${sign}${(abs/base).toLocaleString('fa-IR')}${scale?'.'+(abs%base).toString().padStart(scale,'0'):''}`;}
