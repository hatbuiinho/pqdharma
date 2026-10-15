import {test} from 'node:test';
import assert from 'node:assert/strict';
import {createLibraryLoader} from './library-loader';

function deferred<T>() { let resolve!:(value:T)=>void;let reject!:(error:Error)=>void;const promise=new Promise<T>((a,b)=>{resolve=a;reject=b});return {promise,resolve,reject}; }

test('account switch loads immediately and ignores late response from previous scope',async()=>{
 const first=deferred<string>(),second=deferred<string>();const requests:{scope:string;signal:AbortSignal}[]=[];const results:string[]=[];let settled=0;
 const loader=createLibraryLoader({fetch:(scope,signal)=>{requests.push({scope,signal});return requests.length===1?first.promise:second.promise},data:(data:string)=>results.push(data),error:()=>assert.fail('unexpected error'),settled:()=>settled++});
 const old=loader.reset('public');const next=loader.reset('mine');
 assert.deepEqual(requests.map(r=>r.scope),['public','mine']);assert.equal(requests[0].signal.aborted,true);
 first.resolve('stale');await old;assert.deepEqual(results,[]);assert.equal(settled,0);
 await loader.refresh();assert.equal(requests.length,2); // Old completion must not unlock the current request.
 second.resolve('my recordings');await next;assert.deepEqual(results,['my recordings']);assert.equal(settled,1);
 loader.dispose();
});

test('disposed loader ignores errors and never updates an unmounted library',async()=>{
 const request=deferred<string>();let updates=0;
 const loader=createLibraryLoader({fetch:()=>request.promise,data:()=>updates++,error:()=>updates++,settled:()=>updates++});
 const pending=loader.reset('mine');loader.dispose();request.reject(new Error('aborted'));await pending;await loader.refresh();assert.equal(updates,0);
});
