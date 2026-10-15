/// <reference lib="webworker" />
import { build, files, version } from '$service-worker';
import { cachePolicy } from './lib/pwa-policy';

const worker=globalThis as unknown as ServiceWorkerGlobalScope;
const PREFIX='pq-dharma-shell-';
const SHELL=PREFIX+version;
const DATA='pq-dharma-data-v1';
const SHELL_KEY='/__pq_dharma_app_shell__';
const ASSETS=[...build,...files.filter(path=>!path.startsWith('/branding/'))];

worker.addEventListener('install',event=>{
 event.waitUntil((async()=>{
  const cache=await caches.open(SHELL);
  await cache.addAll(ASSETS);
  const shell=await fetch(new Request('/library',{cache:'reload'}));
  if(!shell.ok||!shell.headers.get('content-type')?.includes('text/html'))throw new Error('App shell unavailable');
  await cache.put(SHELL_KEY,shell);
  // Updates wait until the user chooses to apply them. Playback is never reloaded automatically.
 })());
});
worker.addEventListener('activate',event=>{
 event.waitUntil((async()=>{
  const keys=(await caches.keys()).filter(key=>key.startsWith(PREFIX)&&key!==SHELL);
  // Keep the preceding bundle so other open tabs can still load their chunks.
  await Promise.all(keys.slice(0,-1).map(key=>caches.delete(key)));
  await worker.clients.claim();
 })());
});
worker.addEventListener('message',event=>{
 if(event.data?.type==='PQ_DHARMA_APPLY_UPDATE')event.waitUntil(worker.skipWaiting());
});

async function cachedAsset(request:Request):Promise<Response>{
 const cache=await caches.open(SHELL);
 let saved=await cache.match(request);
 if(!saved&&new URL(request.url).pathname.startsWith('/_app/immutable/')){
  for(const key of (await caches.keys()).filter(key=>key.startsWith(PREFIX)&&key!==SHELL)){
   saved=await(await caches.open(key)).match(request);if(saved)break;
  }
 }
 if(saved)return saved;
 const response=await fetch(request);
 if(response.ok&&response.status===200)await cache.put(request,response.clone()).catch(()=>{});
 return response;
}
async function navigation(request:Request):Promise<Response>{
 const cache=await caches.open(SHELL);
 try{
  const controller=new AbortController();const timer=setTimeout(()=>controller.abort(),5000);
  let response:Response;
  try{response=await fetch(request,{signal:controller.signal})}finally{clearTimeout(timer)}
  if(response.status>=500)throw new Error('App server unavailable');
  // All app routes use the same CSR shell with absolute asset paths.
  if(response.ok&&response.headers.get('content-type')?.includes('text/html'))await cache.put(SHELL_KEY,response.clone()).catch(()=>{});
  return response;
 }catch{
  return await cache.match(SHELL_KEY)??new Response('Chưa có bản lưu. Kết nối mạng để mở PQ Dharma lần đầu.',{status:503,headers:{'Content-Type':'text/plain; charset=utf-8'}});
 }
}
async function savedData(request:Request):Promise<Response>{
 const cache=await caches.open(DATA);
 const saved=await cache.match(request);
 try{
  const controller=new AbortController();const timer=saved?setTimeout(()=>controller.abort(),5000):undefined;
  let response:Response;
  try{response=await fetch(request,{signal:controller.signal})}finally{clearTimeout(timer)}
  if(response.status>=500)throw new Error('Data server unavailable');
  if(response.status===404)await cache.delete(request);
  if(response.ok&&response.status===200&&response.headers.get('content-type')?.includes('application/json')){
   await cache.put(request,response.clone()).catch(()=>{});
   const keys=await cache.keys();
   // Bound storage and keep the library entry when trimming old transcripts.
   const transcripts=keys.filter(key=>new URL(key.url).pathname!=='/api/sutras');
   await Promise.all(transcripts.slice(0,Math.max(0,transcripts.length-60)).map(key=>cache.delete(key)));
  }
  return response;
 }catch(error){
  if(!saved)throw error;
  const headers=new Headers(saved.headers);headers.set('X-PQ-Dharma-Offline','1');
  return new Response(saved.body,{status:saved.status,statusText:saved.statusText,headers});
 }
}
worker.addEventListener('fetch',event=>{
 const request=event.request;
 const policy=cachePolicy({url:request.url,method:request.method,mode:request.mode,hasRange:request.headers.has('range')},worker.location.origin);
 if(policy==='navigation')event.respondWith(navigation(request));
 else if(policy==='asset')event.respondWith(cachedAsset(request));
 else if(policy==='data')event.respondWith(savedData(request));
});
