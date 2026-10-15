<script lang="ts">
 import { onMount, untrack } from 'svelte';
 import { online } from '$lib/network';
 import Icon from './Icon.svelte';
 type Item = {recordingId:string;title:string;language:string;status:string;progress?:number;thumbnailUrl?:string;durationMs?:number};
 function formatDuration(ms:number){const seconds=Math.round(ms/1000);return seconds>=3600?`${Math.floor(seconds/3600)}:${String(Math.floor(seconds/60)%60).padStart(2,'0')}:${String(seconds%60).padStart(2,'0')}`:`${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}`}
 type Filter = 'all'|'ready'|'processing'|'failed';
 const filters:{value:Filter;label:string}[]=[{value:'all',label:'Tất cả'},{value:'ready',label:'Sẵn sàng'},{value:'processing',label:'Đang xử lý'},{value:'failed',label:'Cần thử lại'}];
 const statusNames:Record<string,string>={ready:'Sẵn sàng',failed:'Cần thử lại',queued:'Chờ xử lý',transcribing:'Đang nhận dạng',aligning:'Đang căn thời gian',aligned:'Chuẩn bị tạo IPA',phonetics:'Đang tạo IPA',processing:'Đang xử lý'};
 let items=$state<Item[]>([]),loading=$state(true),query=$state(''),filter=$state<Filter>('all');
 let title=$state(''),file=$state<File|null>(null),uploading=$state(false),message=$state(''),listError=$state(''),importOpen=$state(false);
 let importDialog=$state<HTMLDialogElement>(),fileInput=$state<HTMLInputElement>();
 let savedCopy=$state(false);
 let source=$state<'youtube'|'mp3'>('youtube'),youtubeLink=$state(''),previewLoading=$state(false),previewMessage=$state('');
 let previewSequence=0;
 $effect(()=>{
  const link=youtubeLink.trim();const mode=source;const sequence=++previewSequence;
  previewMessage='';previewLoading=false;
  if(mode!=='youtube'||!link)return;
  const controller=new AbortController();previewLoading=true;
  const titleBefore=untrack(()=>title);
  const timer=setTimeout(async()=>{
   try{const r=await fetch('/api/youtube/preview',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:link}),signal:controller.signal});if(!r.ok)throw new Error(await r.text());const data=await r.json();if(sequence===previewSequence&&title===titleBefore){title=data.title;autoTitle=data.title}}
   catch(e){if(sequence===previewSequence&&!controller.signal.aborted)previewMessage=e instanceof Error?e.message:'Không lấy được tên video.'}
   finally{if(sequence===previewSequence)previewLoading=false}
  },500);
  return()=>{clearTimeout(timer);controller.abort()};
 });
 let dragging=$state(false),autoTitle='';let dragDepth=0;
 function selectFiles(files:File[]){
  if(uploading||source!=='mp3')return;
  message='';
  if(files.length!==1){clearFile();message='Vui lòng chọn một file MP3 mỗi lần.';return}
  const selected=files[0];
  if(!/\.mp3$/i.test(selected.name)){clearFile();message='Chỉ nhận file có định dạng .mp3.';return}
  if(selected.size===0){clearFile();message='File MP3 đang trống. Hãy chọn file khác.';return}
  if(selected.size>200_000_000){clearFile();message='File MP3 vượt quá 200 MB. Hãy chọn file nhỏ hơn.';return}
  file=selected;
  if(!title.trim()||title===autoTitle){autoTitle=Array.from(selected.name.replace(/\.mp3$/i,'').trim()).slice(0,200).join('')||'Bài kinh mới';title=autoTitle}
 }
 function clearFile(){file=null;if(fileInput)fileInput.value='';if(title===autoTitle)title='';autoTitle='';message=''}
 function dragOver(event:DragEvent){event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect=uploading?'none':'copy'}
 function dragEnter(event:DragEvent){if(!uploading&&event.dataTransfer?.types.includes('Files')){event.preventDefault();dragDepth++;dragging=true}}
 function dragLeave(){dragDepth=Math.max(0,dragDepth-1);if(dragDepth===0)dragging=false}
 function dropFile(event:DragEvent){event.preventDefault();dragDepth=0;dragging=false;if(event.dataTransfer?.types.includes('Files'))selectFiles(Array.from(event.dataTransfer.files))}

 const normalizedQuery=$derived(query.trim().normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/đ/gi,'d').toLocaleLowerCase('vi'));
 const visible=$derived(items.filter(item=>{
  const name=item.title.normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/đ/gi,'d').toLocaleLowerCase('vi');
  const matchesStatus=filter==='all'||(filter==='processing'?!['ready','failed'].includes(item.status):item.status===filter);
  return matchesStatus&&name.includes(normalizedQuery);
 }));
 $effect(()=>{if(!importDialog)return;if(importOpen&&!importDialog.open)importDialog.showModal();else if(!importOpen&&importDialog.open)importDialog.close()});
 async function upload(event:SubmitEvent){
  event.preventDefault();if(uploading||!title.trim()||(source==='mp3'?!file:!youtubeLink.trim()))return;uploading=true;message='';dragging=false;dragDepth=0;
  const body=new FormData();body.set('title',title.trim());if(file)body.set('audio',file);
  try{const r=await fetch(source==='youtube'?'/api/recordings/youtube':'/api/recordings',source==='youtube'?{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({url:youtubeLink.trim(),title:title.trim()})}:{method:'POST',body});if(!r.ok)throw new Error(await r.text());const data=await r.json();location.assign(`/recordings/${encodeURIComponent(data.recordingId)}`)}
  catch(e){message=e instanceof Error?e.message:'Không nhập được bài.';uploading=false}
 }
 onMount(()=>{
  let disposed=false,fetching=false;const abort=new AbortController();
  const refresh=async()=>{if(fetching||!navigator.onLine&&items.length>0)return;fetching=true;try{const r=await fetch('/api/sutras',{signal:abort.signal});if(!r.ok)throw new Error();const data:Item[]=await r.json();if(!disposed){items=data;listError='';savedCopy=r.headers.get('X-PQ-Dharma-Offline')==='1'}}catch{if(!disposed)listError=navigator.onLine?'Không tải được thư viện. Hãy kiểm tra kết nối và thử lại.':'Thiết bị chưa lưu thư viện. Kết nối mạng và mở thư viện một lần để sử dụng ngoại tuyến.'}finally{if(!disposed)loading=false;fetching=false}};
  void refresh();const timer=setInterval(refresh,5000);const reconnect=()=>{void refresh()};window.addEventListener('online',reconnect);window.addEventListener('pq-dharma:offline-ready',reconnect);return()=>{window.removeEventListener('online',reconnect);window.removeEventListener('pq-dharma:offline-ready',reconnect);disposed=true;abort.abort();clearInterval(timer)};
 });
</script>
<div class="eyebrow mt-6">MỘT CHÚT TĨNH LẶNG MỖI NGÀY</div>
<div class="library-heading"><div><h1>Thư viện kinh</h1><p>Chọn một bài để nghe và luyện từng lời.</p></div><button class="primary-btn" disabled={!$online} onclick={()=>{message='';importOpen=true}}>Nhập bài</button></div>
{#if savedCopy&&$online}<p class="saved-copy-note" role="status">Đang xem thư viện đã lưu. Trạng thái bài sẽ cập nhật khi kết nối lại.</p>{/if}
<section aria-label="Tìm và lọc bài kinh" class="library-tools">
 <label for="library-search">Tìm bài kinh</label>
 <input id="library-search" type="search" bind:value={query} placeholder="Tìm theo tên bài…"/>
 <div class="library-filters" aria-label="Trạng thái bài">{#each filters as option}<button class="chip" class:chip-active={filter===option.value} aria-pressed={filter===option.value} onclick={()=>filter=option.value}>{option.label}</button>{/each}</div>
</section>
{#if listError}<p role="alert" class="panel text-red-800 mt-4">{listError}<button class="outline-btn mt-3" onclick={()=>location.reload()}>Thử lại</button></p>{/if}
{#if loading}<p class="panel mt-4" role="status">Đang tải thư viện…</p>
{:else if items.length===0&&!listError}<div class="library-empty"><Icon name="library" size={36}/><h2>Thư viện chưa có bài kinh</h2><p>Nhập link YouTube hoặc file MP3 tiếng Anh để bắt đầu.</p><button class="primary-btn" onclick={()=>importOpen=true}>Nhập bài đầu tiên</button></div>
{:else}
 <p class="library-count" aria-live="polite">{visible.length} bài kinh{query||filter!=='all'?` · trong ${items.length} bài`:''}</p>
 <div class="library-grid">{#each visible as item (item.recordingId)}
  <a class="library-item" href={`/recordings/${encodeURIComponent(item.recordingId)}`}>
   <div class="recording-thumbnail">{#if item.thumbnailUrl}<img src={item.thumbnailUrl} alt="" width="480" height="270" loading="lazy" decoding="async" onerror={event=>{item.thumbnailUrl=undefined}}/>{:else}<Icon name="leaf" size={40}/>{/if}</div>
   <div class="recording-info"><h2>{item.title}</h2><p>{item.language==='en'?'Tiếng Anh':item.language}{#if item.durationMs&&item.durationMs>0} · <span aria-label={`Thời lượng ${formatDuration(item.durationMs)}`}>{formatDuration(item.durationMs)}</span>{/if} · {item.status==='ready'?'Nghe & luyện tụng':'Mở bài để xem tiến trình'}</p><span class="recording-status" class:status-ready={item.status==='ready'} class:status-failed={item.status==='failed'}>{statusNames[item.status]||'Đang xử lý'}{#if !['ready','failed'].includes(item.status)} · {item.progress??0}%{/if}</span></div>
  </a>
 {/each}</div>
 {#if visible.length===0&&items.length>0}<div class="library-empty"><h2>Không tìm thấy bài phù hợp</h2><p>Thử tên khác hoặc đổi bộ lọc.</p><button class="outline-btn" onclick={()=>{query='';filter='all'}}>Xem tất cả</button></div>{/if}
{/if}
<dialog bind:this={importDialog} class="settings-dialog" ondragover={dragOver} ondrop={dropFile} onclose={()=>{importOpen=false;dragging=false;dragDepth=0}} oncancel={e=>{if(uploading)e.preventDefault();else importOpen=false}} aria-labelledby="import-title">
 <div class="settings-header"><h2 id="import-title" class="text-lg font-semibold">Nhập bài mới</h2><button class="icon-btn" disabled={uploading} onclick={()=>importOpen=false} aria-label="Đóng nhập bài"><Icon name="x"/></button></div>
 <div class="settings-content">
  <form onsubmit={upload} class="upload-form">
   <div class="library-filters" aria-label="Nguồn bài">{#each [{value:'youtube',label:'Link YouTube'},{value:'mp3',label:'File MP3'}] as option}<button type="button" class="chip" class:chip-active={source===option.value} aria-pressed={source===option.value} disabled={uploading} onclick={()=>{source=option.value as 'youtube'|'mp3';message='';if(title===autoTitle){title='';autoTitle=''}if(source==='mp3'&&file)selectFiles([file])}}>{option.label}</button>{/each}</div>
   {#if source==='youtube'}
    <label>Link YouTube<input type="url" bind:value={youtubeLink} required placeholder="https://www.youtube.com/watch?v=…" disabled={uploading}/></label>
    {#if previewLoading}<p role="status" class="text-xs text-stone-500">Đang lấy tên video…</p>{/if}
    {#if previewMessage}<p role="status" class="text-sm text-red-800">{previewMessage}</p>{/if}
   {/if}
   <label>Tên bài<input type="text" bind:value={title} maxlength={200} required placeholder="Tên bài kinh" disabled={uploading}/></label>
   {#if source==='mp3'}
   <input bind:this={fileInput} type="file" hidden accept=".mp3,audio/mpeg" disabled={uploading} aria-label="Chọn file MP3" onchange={e=>{const files=Array.from(e.currentTarget.files??[]);if(files.length)selectFiles(files)}}/>
   <button type="button" class="mp3-dropzone" class:dragging disabled={uploading} onclick={()=>fileInput?.click()} ondragenter={dragEnter} ondragleave={dragLeave} ondragover={dragOver}>
    <span class="mp3-file-mark" aria-hidden="true">MP3</span>
    <span class="dropzone-title">{dragging?'Thả file MP3 tại đây':file?'Kéo thả để đổi file MP3':'Kéo thả file MP3 vào đây'}</span>
    <span class="dropzone-hint">hoặc bấm để chọn file từ thiết bị</span>
   </button>
   {#if file}<div class="selected-mp3" aria-live="polite"><div><span class="selected-mp3-name">{file.name}</span><span class="selected-mp3-size">{(file.size/1_000_000).toLocaleString('vi-VN',{maximumFractionDigits:2})} MB</span></div><button type="button" class="icon-btn" disabled={uploading} onclick={clearFile} aria-label="Bỏ file MP3 đã chọn"><Icon name="x" size={18}/></button></div>{/if}
   {/if}
   <p class="text-xs text-stone-500">Tiếng Anh · Tối đa 200 MB, 4 giờ. Lời và IPA được tạo tự động; bạn có thể nghe và sửa sau khi xử lý.</p>
   {#if message}<p role="alert" class="text-sm text-red-800">{message}</p>{/if}
   <button class="primary-btn" type="submit" disabled={!title.trim()||(source==='mp3'?!file:!youtubeLink.trim())||uploading||!$online}>{uploading?'Đang nhập bài…':'Import & tạo lời'}</button>
  </form>
 </div>
</dialog>
