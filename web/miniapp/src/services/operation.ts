// Retain only a digest and key in tab storage. Unknown responses retain the key.
export async function operationKey(scope:string,payload:unknown):Promise<string>{
 const digest=Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256',new TextEncoder().encode(JSON.stringify(payload))))).map(b=>b.toString(16).padStart(2,'0')).join('');
 const slot=`telmivo:operation:${scope}`; const raw=sessionStorage.getItem(slot);
 if(raw){const saved=JSON.parse(raw) as {digest:string;key:string};if(saved.digest===digest)return saved.key;}
 const key=crypto.randomUUID();sessionStorage.setItem(slot,JSON.stringify({digest,key}));return key;
}
export function completeOperation(scope:string){sessionStorage.removeItem(`telmivo:operation:${scope}`);}
