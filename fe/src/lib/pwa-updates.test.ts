import test from 'node:test';
import assert from 'node:assert/strict';
import {inspectWorkerUpdate,workerVersion} from './pwa-updates';

function worker(version:string):ServiceWorker{
 return {postMessage(message:{type:string},ports:MessagePort[]){
  assert.equal(message.type,'PQ_DHARMA_GET_VERSION');
  ports[0].postMessage({type:'PQ_DHARMA_VERSION',version});
 }} as ServiceWorker;
}

test('fresh page already on the waiting build does not need an app update',async()=>{
 assert.equal(await inspectWorkerUpdate(worker('build-2'),'build-2',async()=>{throw new Error('network unnecessary')}),'current');
});
test('page on the preceding build is offered the waiting update',async()=>{
 assert.equal(await inspectWorkerUpdate(worker('build-2'),'build-1',async()=>null),'update');
});
test('version request transfers a reply port and times out safely for older workers',async()=>{
 assert.equal(await workerVersion(worker('build-2')),'build-2');
 assert.equal(await workerVersion({postMessage(){}} as unknown as ServiceWorker,10),null);
});
test('legacy worker cannot cause an update banner when network build matches the page or cannot be checked',async()=>{
 const legacy={postMessage(){throw new Error('worker unavailable')}} as unknown as ServiceWorker;
 assert.equal(await inspectWorkerUpdate(legacy,'build-2',async()=>'build-2'),'unknown');
 assert.equal(await inspectWorkerUpdate(legacy,'build-2',async()=>null),'unknown');
 assert.equal(await inspectWorkerUpdate(legacy,'build-1',async()=>'build-2'),'update');
});

test('superseded waiting worker does not advertise an older or intermediate build',async()=>{
 assert.equal(await inspectWorkerUpdate(worker('build-1'),'build-2',async()=>'build-2'),'unknown');
 assert.equal(await inspectWorkerUpdate(worker('build-2'),'build-1',async()=>'build-3'),'unknown');
});
