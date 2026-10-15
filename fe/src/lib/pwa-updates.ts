export type WaitingUpdate = 'current' | 'update' | 'unknown';

// A freshly navigated page can already use the waiting worker's build even
// though an older worker still controls other open tabs.
export function workerVersion(worker:ServiceWorker, timeoutMs=1500):Promise<string|null>{
 return new Promise(resolve=>{
  const channel=new MessageChannel();
  const finish=(version:string|null)=>{clearTimeout(timer);channel.port1.close();channel.port2.close();resolve(version)};
  const timer=setTimeout(()=>finish(null),timeoutMs);
  channel.port1.onmessage=event=>{
   if(event.data?.type==='PQ_DHARMA_VERSION'&&typeof event.data.version==='string')finish(event.data.version);
  };
  try{worker.postMessage({type:'PQ_DHARMA_GET_VERSION'},[channel.port2])}catch{finish(null)}
 });
}

export async function inspectWorkerUpdate(worker:ServiceWorker,pageVersion:string,latestVersion:()=>Promise<string|null>):Promise<WaitingUpdate>{
 const candidate=await workerVersion(worker);
 if(candidate===pageVersion)return 'current';
 if(candidate!==null){
  const latest=await latestVersion();
  // A superseded waiting build (including after a rollback) is not an update
  // for a page that already loaded the server's current build.
  return latest===null||latest===candidate?'update':'unknown';
 }
 // Workers installed before version messaging existed need a network fallback.
 // Absence of a reply by itself is not evidence of a newer app.
 const latest=await latestVersion();
 return latest!==null&&latest!==pageVersion?'update':'unknown';
}
