import { writable } from 'svelte/store';
export type User={id:string;email:string;name:string;admin:boolean;disabled?:boolean};
export const auth=writable<User|null>(null);
let pending:Promise<void>|null=null;
export function refreshAuth():Promise<void>{
 if(pending)return pending;
 pending=(async()=>{try{const r=await fetch('/api/auth/me',{cache:'no-store'});if(!r.ok)throw new Error();auth.set((await r.json()).user??null)}catch{auth.set(null)}})().finally(()=>{pending=null});
 return pending;
}
