<script lang="ts">
 import { onMount, untrack } from 'svelte';
 import { online } from '$lib/network';
 import Icon from './Icon.svelte';
 type Item = {recordingId:string;title:string;language:string;status:string;progress?:number;thumbnailUrl?:string;durationMs?:number};
 function formatDuration(ms:number){const seconds=Math.round(ms/1000);return seconds>=3600?`${Math.floor(seconds/3600)}:${String(Math.floor(seconds/60)%60).padStart(2,'0')}:${String(seconds%60).padStart(2,'0')}`:`${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')}`}
 type Filter = 'all'|'ready'|'processing'|'failed';
 const filters:{value:Filter;label:string}[]=[{value:'all',label:'Tất cả'},{value:'ready',label:'Sẵn sàng'},{value:'processing',label:'Đang xử lý'},{value:'failed',label:'Cần thử lại'}];
 const statusNames:Record<string,string>={ready:'Sẵn sàng',failed:'Cần thử lại',queued:'Chờ xử lý',transcribing:'Đang xử lý audio',aligning:'Đang căn thời gian',aligned:'Chuẩn bị tạo IPA',phonetics:'Đang tạo IPA',processing:'Đang xử lý'};
 let items=$state<Item[]>([]),loading=$state(true),query=$state(''),filter=$state<Filter>('all');
 let title=$state(''),file=$state<File|null>(null),uploading=$state(false),message=$state(''),listError=$state(''),importOpen=$state(false);
 let importDialog=$state<HTMLDialogElement>(),fileInput=$state<HTMLInputElement>(),pdfInput=$state<HTMLInputElement>();
 let savedCopy=$state(false);
 let source=$state<'youtube'|'mp3'|'pdf'>('youtube'),youtubeLink=$state(''),previewLoading=$state(false),previewMessage=$state('');
 type PdfTurn={speaker:'woman'|'man'|'narrator';text:string};
 let pdfTurns=$state<PdfTurn[]>([]),pdfFilename=$state(''),pdfThumbnail=$state(''),pdfLoading=$state(false),pdfSequence=0;
 let pdfVoices=$state({woman:'bf_emma',man:'bm_george',narrator:'bf_emma'}),pdfSpeed=$state(1),pdfGap=$state(.5);
 const voiceOptions=[{value:'bf_emma',label:'Emma · Nữ · Anh–Anh'},{value:'bf_isabella',label:'Isabella · Nữ · Anh–Anh'},{value:'bm_george',label:'George · Nam · Anh–Anh'},{value:'bm_lewis',label:'Lewis · Nam · Anh–Anh'},{value:'af_heart',label:'Heart · Nữ · Anh–Mỹ'},{value:'am_michael',label:'Michael · Nam · Anh–Mỹ'}];
 const pdfSpeakers=$derived([...new Set(pdfTurns.map(turn=>turn.speaker))]);
 const pdfValid=$derived(pdfTurns.length>0&&pdfTurns.length<=300&&pdfTurns.every(turn=>turn.text.trim().length>0&&Array.from(turn.text).length<=4000)&&pdfTurns.reduce((n,t)=>n+Array.from(t.text).length,0)<=60000);
 const importDisabled=$derived(!title.trim()||uploading||!$online||pdfLoading||(source==='pdf'?!pdfValid:source==='mp3'?!file:!youtubeLink.trim()));
 async function previewPDF(selected:File|undefined){
  if(!selected||uploading||pdfLoading)return;
  const sequence=++pdfSequence;pdfTurns=[];pdfFilename='';pdfThumbnail='';message='';
  if(!/\.pdf$/i.test(selected.name)||!selected.size||selected.size>20_000_000){message='Chọn file PDF có văn bản, tối đa 20 MB.';return}
  pdfLoading=true;
  try{
   const body=new FormData();body.set('pdf',selected);
   const response=await fetch('/api/pdf/preview',{method:'POST',body});
   if(!response.ok)throw new Error(await response.text());
   const data=await response.json();
   if(sequence===pdfSequence){pdfTurns=data.turns;pdfFilename=data.filename;pdfThumbnail=data.thumbnail||'';if(!title.trim()||title===autoTitle){autoTitle=Array.from(data.title as string).slice(0,200).join('');title=autoTitle}}
  }catch(error){if(sequence===pdfSequence)message=error instanceof Error?error.message:'Không đọc được PDF.'}
  finally{if(sequence===pdfSequence)pdfLoading=false}
 }
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
 function selectPDFs(files:File[]){if(uploading||pdfLoading||source!=='pdf')return;if(files.length!==1){message='Vui lòng chọn một file PDF mỗi lần.';return}void previewPDF(files[0])}
 function dragOver(event:DragEvent){event.preventDefault();if(event.dataTransfer)event.dataTransfer.dropEffect=uploading||pdfLoading||source==='youtube'?'none':'copy'}
 function dragEnter(event:DragEvent){if(!uploading&&!pdfLoading&&source!=='youtube'&&event.dataTransfer?.types.includes('Files')){event.preventDefault();dragDepth++;dragging=true}}
 function dragLeave(){dragDepth=Math.max(0,dragDepth-1);if(dragDepth===0)dragging=false}
 function dropFile(event:DragEvent){event.preventDefault();dragDepth=0;dragging=false;if(event.dataTransfer?.types.includes('Files')){const files=Array.from(event.dataTransfer.files);if(source==='pdf')selectPDFs(files);else selectFiles(files)}}

 const normalizedQuery=$derived(query.trim().normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/đ/gi,'d').toLocaleLowerCase('vi'));
 const visible=$derived(items.filter(item=>{
  const name=item.title.normalize('NFD').replace(/[\u0300-\u036f]/g,'').replace(/đ/gi,'d').toLocaleLowerCase('vi');
  const matchesStatus=filter==='all'||(filter==='processing'?!['ready','failed'].includes(item.status):item.status===filter);
  return matchesStatus&&name.includes(normalizedQuery);
 }));
 $effect(()=>{if(!importDialog)return;if(importOpen&&!importDialog.open)importDialog.showModal();else if(!importOpen&&importDialog.open)importDialog.close()});
 async function upload(event:SubmitEvent){
  event.preventDefault();if(importDisabled)return;uploading=true;message='';dragging=false;dragDepth=0;
  const body=new FormData();body.set('title',title.trim());if(file)body.set('audio',file);
  const endpoint=source==='pdf'?'/api/recordings/pdf':source==='youtube'?'/api/recordings/youtube':'/api/recordings';
  const payload=source==='pdf'?{title:title.trim(),filename:pdfFilename,thumbnail:pdfThumbnail,turns:pdfTurns,voices:pdfVoices,speed:pdfSpeed,gap:pdfGap}:{url:youtubeLink.trim(),title:title.trim()};
  try{const r=await fetch(endpoint,source==='mp3'?{method:'POST',body}:{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify(payload)});if(!r.ok)throw new Error(await r.text());const data=await r.json();location.assign(`/recordings/${encodeURIComponent(data.recordingId)}`)}
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
   <div class="library-filters" aria-label="Nguồn bài">{#each [{value:'youtube',label:'Link YouTube'},{value:'mp3',label:'File MP3'},{value:'pdf',label:'PDF → Audio'}] as option}<button type="button" class="chip" class:chip-active={source===option.value} aria-pressed={source===option.value} disabled={uploading||pdfLoading} onclick={()=>{source=option.value as 'youtube'|'mp3'|'pdf';message='';dragging=false;dragDepth=0;if(title===autoTitle){title='';autoTitle=''}if(source==='mp3'&&file)selectFiles([file])}}>{option.label}</button>{/each}</div>
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
   {#if source==='pdf'}
    <input bind:this={pdfInput} type="file" hidden accept=".pdf,application/pdf" disabled={uploading||pdfLoading} aria-label="Chọn file PDF tiếng Anh" onchange={event=>{const files=Array.from(event.currentTarget.files??[]);event.currentTarget.value='';if(files.length)selectPDFs(files)}}/>
    <button type="button" class="mp3-dropzone" class:dragging disabled={uploading||pdfLoading} aria-busy={pdfLoading} onclick={()=>pdfInput?.click()} ondragenter={dragEnter} ondragleave={dragLeave} ondragover={dragOver}>
     <span class="mp3-file-mark" aria-hidden="true">PDF</span>
     <span class="dropzone-title">{pdfLoading?'Đang đọc nội dung PDF…':dragging?'Thả file PDF tại đây':pdfFilename?'Kéo thả để đổi file PDF':'Kéo thả file PDF vào đây'}</span>
     <span class="dropzone-hint">hoặc bấm để chọn file từ thiết bị · Tối đa 20 MB</span>
    </button>
    {#if pdfFilename}<p class="selected-mp3" aria-live="polite"><span class="selected-mp3-name">{pdfFilename}</span></p>{/if}
    <p class="text-xs text-stone-500">PDF có văn bản · Tối đa 20 MB, 50 trang, 60.000 ký tự. Chưa hỗ trợ bản scan. Nội dung được đọc bằng giọng AI trên server.</p>
    {#if pdfLoading}<p role="status">Đang đọc nội dung PDF…</p>{/if}
    {#if pdfTurns.length}
     <div class="pdf-voice-settings">
      {#each pdfSpeakers as speaker}<label>Giọng {speaker==='woman'?'Woman':speaker==='man'?'Man':'người dẫn chuyện'}<select bind:value={pdfVoices[speaker]} disabled={uploading}>{#each voiceOptions as voice}<option value={voice.value}>{voice.label}</option>{/each}</select></label>{/each}
      <label>Tốc độ đọc<select bind:value={pdfSpeed} disabled={uploading}><option value={.85}>Chậm · 0.85×</option><option value={1}>Bình thường · 1×</option><option value={1.15}>Nhanh · 1.15×</option></select></label>
      <label>Nghỉ giữa lượt<select bind:value={pdfGap} disabled={uploading}><option value={0}>Không nghỉ thêm</option><option value={.5}>0.5 giây</option><option value={1}>1 giây</option><option value={2}>2 giây</option></select></label>
     </div>
     <div class="pdf-review-heading"><h3>Xem và sửa lời · {pdfTurns.length} lượt</h3><p>Nhãn người nói chỉ dùng để chọn giọng, không được đọc thành tiếng. Kiểm tra tiêu đề, số trang và nội dung trước khi tạo audio.</p></div>
     <div class="pdf-turns">{#each pdfTurns as turn,index}
      <fieldset class="pdf-turn" disabled={uploading}><legend>Lượt {index+1}</legend>
       <div class="pdf-turn-actions"><label>Người nói<select bind:value={turn.speaker}><option value="woman">Woman</option><option value="man">Man</option><option value="narrator">Người dẫn chuyện</option></select></label><button type="button" class="outline-btn" onclick={()=>{pdfTurns=pdfTurns.filter((_,i)=>i!==index)}} aria-label={`Xóa lượt ${index+1}`}>Xóa</button></div>
       <label>Lời đọc<textarea bind:value={turn.text} rows={3} maxlength={4000} required></textarea></label>
      </fieldset>
     {/each}</div>
     <button type="button" class="outline-btn" disabled={uploading||pdfTurns.length>=300} onclick={()=>{pdfTurns=[...pdfTurns,{speaker:'narrator',text:''}]}}>Thêm lượt đọc</button>
     {#if !pdfValid}<p role="status" class="text-sm text-red-800">Mỗi lượt cần 1–4.000 ký tự, tổng tối đa 60.000 ký tự.</p>{/if}
    {/if}
   {:else}
   <p class="text-xs text-stone-500">Tiếng Anh · Tối đa 200 MB, 4 giờ. Lời và IPA được tạo tự động; bạn có thể nghe và sửa sau khi xử lý.</p>
   {/if}
   {#if message}<p role="alert" class="text-sm text-red-800">{message}</p>{/if}
   <button class="primary-btn" type="submit" disabled={importDisabled}>{uploading?'Đang nhập bài…':source==='pdf'?'Tạo audio từ PDF':'Import & tạo lời'}</button>
  </form>
 </div>
</dialog>
