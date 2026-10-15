import {test} from 'node:test';
import assert from 'node:assert/strict';
import {get} from 'svelte/store';
import {auth,refreshAuth} from './auth';

test('simultaneous profile and library initialization share one auth request',async(t)=>{
 let finish!:(response:Response)=>void;let calls=0;
 t.mock.method(globalThis,'fetch',()=>{calls++;return new Promise<Response>(resolve=>finish=resolve)});
 const profile=refreshAuth(),library=refreshAuth();
 assert.equal(calls,1);assert.equal(profile,library);
 finish(new Response(JSON.stringify({user:{id:'reader',email:'reader@test.local',name:'Reader',admin:false}})));
 await Promise.all([profile,library]);assert.equal(get(auth)?.id,'reader');
 const again=refreshAuth();assert.equal(calls,2);finish(new Response(JSON.stringify({user:null})));await again;assert.equal(get(auth),null);
});
