<script lang="ts">
 import { onMount } from 'svelte';
 import { dev } from '$app/environment';
 import { online } from '$lib/network';
 type InstallPrompt=Event&{prompt:()=>Promise<void>;userChoice:Promise<{outcome:'accepted'|'dismissed'}>};
 let installPrompt=$state<InstallPrompt|null>(null),installed=$state(false),dismissed=$state(false),ios=$state(false),help=$state(false),applying=$state(false),installing=$state(false),message=$state('');
 let waiting=$state<ServiceWorker|null>(null);
 async function install(){
  if(!installPrompt){help=!help;return}
  installing=true;message='';
  try{await installPrompt.prompt();const choice=await installPrompt.userChoice;if(choice.outcome==='accepted')installed=true;installPrompt=null}catch{message='Mở menu trình duyệt để cài PQ Dharma.'}finally{installing=false}
 }
 function applyUpdate(){if(!waiting)return;applying=true;message='';waiting.postMessage({type:'PQ_DHARMA_APPLY_UPDATE'})}
 onMount(()=>{
  let disposed=false,registration:ServiceWorkerRegistration|undefined;
  const connections=()=>online.set(navigator.onLine);
  const standalone=()=>{installed=window.matchMedia('(display-mode: standalone)').matches||('standalone' in navigator&&navigator.standalone===true)};
  const beforeInstall=(event:Event)=>{event.preventDefault();installPrompt=event as InstallPrompt};
  const appInstalled=()=>{installed=true;installPrompt=null};
  const changed=()=>{if(applying)window.location.reload();else window.dispatchEvent(new Event('pq-dharma:offline-ready'))};
  const inspectWaiting=()=>{if(registration?.waiting)waiting=registration.waiting};
  const updateFound=()=>{const candidate=registration?.installing;if(candidate)candidate.addEventListener('statechange',()=>{if(candidate.state==='installed'&&!disposed)inspectWaiting()})};
  connections();standalone();
  ios=/iPad|iPhone|iPod/.test(navigator.userAgent)||(navigator.platform==='MacIntel'&&navigator.maxTouchPoints>1);
  window.addEventListener('online',connections);window.addEventListener('offline',connections);
  window.addEventListener('beforeinstallprompt',beforeInstall);window.addEventListener('appinstalled',appInstalled);
  const display=window.matchMedia('(display-mode: standalone)');display.addEventListener('change',standalone);
  if(!dev&&window.isSecureContext&&'serviceWorker' in navigator){
   navigator.serviceWorker.addEventListener('controllerchange',changed);
   void navigator.serviceWorker.register('/service-worker.js',{scope:'/',type:'module',updateViaCache:'none'}).then(value=>{
    if(disposed)return;registration=value;inspectWaiting();value.addEventListener('updatefound',updateFound);updateFound();
    void value.update().catch(()=>{});
   }).catch(()=>{if(!disposed)message='Chưa lưu được ứng dụng để mở ngoại tuyến. Hãy tải lại khi có mạng.'});
  }
  return()=>{disposed=true;window.removeEventListener('online',connections);window.removeEventListener('offline',connections);window.removeEventListener('beforeinstallprompt',beforeInstall);window.removeEventListener('appinstalled',appInstalled);display.removeEventListener('change',standalone);registration?.removeEventListener('updatefound',updateFound);if('serviceWorker' in navigator)navigator.serviceWorker.removeEventListener('controllerchange',changed)};
 });
</script>
{#if !$online}<div class="pwa-notice" role="status">Ngoại tuyến · Bạn có thể đọc lời đã lưu. Phát audio và xử lý bài cần kết nối mạng.</div>{/if}
{#if waiting}
 <div class="pwa-toolbar" role="status"><span>Đã có phiên bản PQ Dharma mới.</span><button class="pwa-action" disabled={applying} onclick={applyUpdate}>{applying?'Đang cập nhật…':'Cập nhật & tải lại'}</button></div>
{:else if !installed&&!dismissed&&(installPrompt||ios)}
 <div class="pwa-toolbar"><span>Cài PQ Dharma để mở nhanh từ màn hình chính.</span><button class="pwa-action" disabled={installing} onclick={install}>{installing?'Đang mở…':'Cài ứng dụng'}</button><button class="pwa-dismiss" onclick={()=>dismissed=true} aria-label="Ẩn gợi ý cài ứng dụng">×</button></div>
{/if}
{#if help&&!installed}<div class="pwa-notice" role="status">Trên iPhone/iPad, mở bằng Safari → Chia sẻ → Thêm vào Màn hình chính.</div>{/if}
{#if message}<div class="pwa-notice" role="status">{message}</div>{/if}
