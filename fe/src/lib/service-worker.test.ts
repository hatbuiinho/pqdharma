import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import vm from 'node:vm';
import ts from 'typescript';
import { cachePolicy } from './pwa-policy';

// Run the real worker handlers with browser-like CacheStorage, not a second
// implementation of their routing. No browser UI or network service is needed.
function harness(){
 const origin='https://dharma.test';
 const handlers=new Map<string,(event:any)=>void>();
 const storage=new Map<string,Map<string,Response>>();
 const network=new Map<string,()=>Response>([
  ['/library',()=>new Response('<html><script src="/_app/immutable/start.js"></script></html>',{headers:{'Content-Type':'text/html'}})],
  ['/_app/immutable/start.js',()=>new Response('bootstrap')],
  ['/icons/icon-192.png',()=>new Response('icon')]
 ]);
 let offline=false,claims=0,skips=0;
 const key=(request:Request|string)=>new URL(typeof request==='string'?request:request.url,origin).href;
 class WorkerRequest extends Request{constructor(input:RequestInfo|URL,init?:RequestInit){super(typeof input==='string'?new URL(input,origin):input,init)}}
 const fetcher=async(input:Request|string)=>{if(offline)throw new Error('offline');const path=new URL(key(input)).pathname;return network.get(path)?.()??new Response('missing',{status:404})};
 const caches={
  keys:async()=>[...storage.keys()],
  delete:async(name:string)=>storage.delete(name),
  open:async(name:string)=>{
   if(!storage.has(name))storage.set(name,new Map());const saved=storage.get(name)!;
   return {
    addAll:async(paths:string[])=>{for(const path of paths){const response=await fetcher(path);if(!response.ok)throw new Error('precache failed');saved.set(key(path),response.clone())}},
    put:async(request:Request|string,response:Response)=>{saved.set(key(request),response.clone())},
    match:async(request:Request|string)=>saved.get(key(request))?.clone(),
    delete:async(request:Request|string)=>saved.delete(key(request)),
    keys:async()=>[...saved.keys()].map(url=>new WorkerRequest(url))
   };
  }
 };
 const source=readFileSync(new URL('../service-worker.ts',import.meta.url),'utf8').replace(/^import .* from .*;$/gm,'');
 const code=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2022,module:ts.ModuleKind.None}}).outputText;
 vm.runInNewContext(code,{
  build:['/_app/immutable/start.js'],files:['/icons/icon-192.png'],version:'new',cachePolicy,
  location:{origin},addEventListener:(name:string,handler:(event:any)=>void)=>handlers.set(name,handler),
  clients:{claim:async()=>{claims++}},skipWaiting:async()=>{skips++},caches,fetch:fetcher,
  Request:WorkerRequest,Response,Headers,URL,AbortController,setTimeout,clearTimeout,console
 });
 const event=async(name:string,extra:object={})=>{const pending:Promise<unknown>[]=[];handlers.get(name)!({waitUntil:(promise:Promise<unknown>)=>pending.push(promise),...extra});await Promise.all(pending)};
 const request=(path:string,options:{method?:string;mode?:string;range?:boolean}={})=>{
  let response:Promise<Response>|undefined;
  const value={url:key(path),method:options.method??'GET',mode:options.mode??'cors',headers:new Headers(options.range?{range:'bytes=0-99'}:{})};
  handlers.get('fetch')!({request:value,respondWith:(promise:Promise<Response>)=>{response=promise}});
  return response;
 };
 return {event,request,network,caches,storage,setOffline:(value:boolean)=>{offline=value},counts:()=>({claims,skips})};
}

test('installs a complete shell, opens offline deep links, and waits for explicit update consent',async()=>{
 const h=harness();await h.event('install');assert.deepEqual(h.counts(),{claims:0,skips:0});
 await h.caches.open('another-app-cache');await h.caches.open('pq-dharma-shell-oldest');await h.caches.open('pq-dharma-shell-previous');await h.caches.open('pq-dharma-data-v1');
 await h.event('activate');assert.deepEqual(h.counts(),{claims:1,skips:0});
 assert.ok(h.storage.has('another-app-cache'));assert.ok(!h.storage.has('pq-dharma-data-v1'));assert.ok(h.storage.has('pq-dharma-shell-previous'));assert.ok(!h.storage.has('pq-dharma-shell-oldest'));
 h.setOffline(true);
 const response=await h.request('/recordings/never-navigated-id?test=1',{mode:'navigate'})!;
 assert.equal(response.status,200);assert.match(await response.text(),/\/_app\/immutable\/start\.js/);
 assert.equal(await(await h.request('/_app/immutable/start.js')!).text(),'bootstrap');
 await h.event('message',{data:{type:'unrelated'}});assert.equal(h.counts().skips,0);
 await h.event('message',{data:{type:'PQ_DHARMA_APPLY_UPDATE'}});assert.equal(h.counts().skips,1);
});
test('private data bypasses service worker storage and activation removes legacy account snapshots',async()=>{
 const h=harness();await h.caches.open('pq-dharma-data-v1');await h.event('install');await h.event('activate');
 assert.ok(!h.storage.has('pq-dharma-data-v1'));
 for(const path of ['/api/sutras','/api/recordings/sample/transcript','/api/recordings/sample/audio','/api/auth/me'])assert.equal(h.request(path),undefined);
});
test('worker reports its actual cached build through the requesting page reply port',async()=>{
 const h=harness();let reply:unknown;
 await h.event('message',{data:{type:'PQ_DHARMA_GET_VERSION'},ports:[{postMessage:(message:unknown)=>{reply=message}}]});
 assert.deepEqual(JSON.parse(JSON.stringify(reply)),{type:'PQ_DHARMA_VERSION',version:'new'});
 assert.equal(h.counts().skips,0);
});
