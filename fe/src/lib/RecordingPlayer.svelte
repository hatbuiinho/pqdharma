<script lang="ts">
 import { shareRecordingLink } from '$lib/share';
 let shareMessage=$state('');
 async function shareRecording(){shareMessage='';shareMessage=await shareRecordingLink(recordingId,recordingTitle)}
 import Topbar from '$lib/Topbar.svelte';
 import Icon from '$lib/Icon.svelte';
 import { online } from '$lib/network';
 import { readingScrollDelta } from '$lib/reading-scroll';
 import SegmentEditor from '$lib/SegmentEditor.svelte';
 import { onMount, tick } from 'svelte';
 import { beforeNavigate } from '$app/navigation';
 import { activeIndex, selectedRange, loopBoundary, practicePreferences, formatTime, formatPlayerTime, formatPrecise, type IpaVoice, type TimeRange, type Transcript } from '$lib/player';
 type Job={status:string;progress?:number;progressMessage?:string;id?:string;kind?:string;voice?:IpaVoice;error?:string};
 const stageNames:Record<string,string>={queued:'Chờ xử lý',transcribing:'Đang xử lý audio',aligning:'Đang căn thời gian từng từ',aligned:'Chuẩn bị tạo phiên âm',phonetics:'Đang tạo IPA',ready:'Sẵn sàng',failed:'Xử lý chưa hoàn tất'};
 let {recordingId}: {recordingId:string} = $props();
 let recordingTitle=$state('Bài kinh'),job=$state<Job>({status:'ready'}),jobError=$state(''),showEditors=$state(false);
 const jobProgress=$derived(Math.max(0,Math.min(100,job.progress??0)));
 const jobBusy=$derived(!['ready','failed'].includes(job.status));
 let offlineReading=$state(false);
 let transcript = $state<Transcript|null>(null), loading=$state(true), error=$state(''),syncError=$state('');
 let audio = $state<HTMLAudioElement>(undefined!); let current=$state(0),duration=$state(0),playing=$state(false),rate=$state(1),fontSize=$state(22),autoScroll=$state(true),showPhonetics=$state(true);
 let selecting=$state(false),anchor=$state<number|null>(null),selectionEnd=$state<number|null>(null),looping=$state(false),repeats=$state(0),completed=$state(0),gap=$state(.5),waiting=$state(false);
 let manualRange=$state<TimeRange|null>(null), previewing=$state(false), controls=$state(false);
 let ipaStyle=$state<'readable'|'original'>('readable');
 let ipaChoice=$state<IpaVoice>('en-gb'),ipaDisplay=$state<IpaVoice|''>(''),ipaPending=$state<IpaVoice|null>(null),ipaSaving=$state(false),ipaError=$state(''),ipaMessage=$state('');
 const ipaReady=$derived(transcript?.ipaVoices?.includes(ipaChoice)??false);
 const ipaLabels:Record<IpaVoice,string>={'en-gb':'Anh–Anh','en-us':'Anh–Mỹ'};
 function phoneticFor(token:(typeof tokens)[number]){return ipaDisplay?token.phonetics?.[ipaDisplay]:token.phonetic}
 function chooseIPAVoice(){ipaError='';ipaMessage='';if(transcript?.ipaVoices?.includes(ipaChoice))ipaDisplay=ipaChoice}
 async function generateIPA(){
  ipaSaving=true;ipaError='';ipaMessage='';
  try{
   const voice=ipaChoice;
   const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/ipa`,{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({voice})});
   if(!r.ok)throw new Error(await r.text());
   const data=await r.json();ipaPending=voice;job={id:data.jobId,kind:'ipa',voice,status:'aligned',progress:0,progressMessage:`Chuẩn bị tạo IPA ${ipaLabels[voice]}`};
  }catch(e){ipaError=e instanceof Error?e.message:'Không tạo được IPA.'}finally{ipaSaving=false}
 }
 async function setDefaultIPA(){
  ipaSaving=true;ipaError='';ipaMessage='';
  try{
   const voice=ipaChoice;
   const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/ipa/default`,{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({voice})});
   if(!r.ok)throw new Error(await r.text());
   await r.json();if(transcript)transcript.defaultIpaVoice=voice;ipaDisplay=voice;ipaMessage=`Đã đặt ${ipaLabels[voice]} làm mặc định cho bài này.`;
  }catch(e){ipaError=e instanceof Error?e.message:'Không lưu được giọng mặc định.'}finally{ipaSaving=false}
 }
 let pendingPosition:number|null=null;
 let followPaused=$state(false);
 let followForce=true,lastFollowCheck=0,scrollCooldownUntil=0;
 let frame=0,savedAt=0,waitTimer:ReturnType<typeof setTimeout>|undefined,waitRemaining=0,waitDeadline=0;
 const tokens=$derived(transcript?.segments.flatMap(s=>s.tokens)??[]);
 const tokenPositions=$derived(new Map(tokens.map((token,index)=>[token.id,index])));
 const progress=$derived(duration>0 ? Math.max(0,Math.min(100,current/duration*100)) : 0);
 const tokenIndex=$derived(activeIndex(tokens,current*1000,true));
 const segmentIndex=$derived(activeIndex(transcript?.segments??[],current*1000,true));
 const range=$derived(manualRange ?? (anchor!==null&&selectionEnd!==null ? selectedRange(tokens,anchor,selectionEnd):null));
 let settingsDialog = $state<HTMLDialogElement>();
 const selectedText=$derived(range ? tokens.filter(t=>t.startMs<range.endMs&&t.endMs>range.startMs).map(t=>t.text).join(' ') : '');
 $effect(()=>{if(!settingsDialog)return;if(controls&&!settingsDialog.open)settingsDialog.showModal();else if(!controls&&settingsDialog.open)settingsDialog.close()});
 beforeNavigate(()=>{save();if(audio)audio.pause();disableLoop()});
 function stopWait(){clearTimeout(waitTimer);waitTimer=undefined;waiting=false;waitRemaining=0}
 function disableLoop(){previewing=false;if(waiting)playing=false;looping=false;completed=0;stopWait()}
 async function play(){followForce=true;error='';try{await audio.play();playing=true}catch{playing=false;error='Không phát được audio. Hãy thử lại.'}}
 function pause(){audio.pause();playing=false;if(waiting){waitRemaining=Math.max(0,waitDeadline-Date.now());clearTimeout(waitTimer)}}
 function scheduleWait(ms:number){waiting=true;playing=true;waitRemaining=ms;waitDeadline=Date.now()+ms;waitTimer=setTimeout(()=>{waiting=false;waitRemaining=0;if(range&&looping){audio.currentTime=range.startMs/1000;current=audio.currentTime;void play()}},ms)}
 function togglePlay(){if(playing){pause();return}if(waiting){scheduleWait(waitRemaining);return}if(looping&&range&&(current*1000<range.startMs||current*1000>=range.endMs)){audio.currentTime=range.startMs/1000}void play()}
 function seek(seconds:number){followPaused=false;followForce=true;scrollCooldownUntil=0;lastFollowCheck=0;disableLoop();audio.currentTime=Math.max(0,Math.min(seconds,duration));current=audio.currentTime}
 function choose(index:number){if(!$online&&!selecting)return;if(selecting){disableLoop();manualRange=null;if(anchor===null||selectionEnd!==null){anchor=index;selectionEnd=null}else{selectionEnd=index;selecting=false;save()}}else{seek(tokens[index].startMs/1000);void play()}}
 function startLoop(){if(!range)return;previewing=false;stopWait();looping=true;completed=0;audio.currentTime=range.startMs/1000;current=audio.currentTime;void play()}
 function changeRate(value:number){rate=value;if(audio)audio.playbackRate=rate;save()}
 function chooseWords(){
  const nextSelecting=!selecting;
  disableLoop();
  if(nextSelecting){pause();followPaused=true;window.scrollTo({top:window.scrollY,behavior:'instant'})}
  selecting=nextSelecting;anchor=null;selectionEnd=null;manualRange=null;controls=false;save();
 }
 function handleLoopShortcut() {
  if (looping) { disableLoop(); return; }
  if (range) { startLoop(); return; }
  chooseWords();
 }
 function listenPreview(){if(!range)return;disableLoop();previewing=true;audio.currentTime=range.startMs/1000;current=audio.currentTime;void play()}
 function clearRange(){disableLoop();anchor=null;selectionEnd=null;manualRange=null;selecting=false;save()}
 function save(){if(!transcript)return;try{localStorage.setItem(`dharma-player:${recordingId}`,JSON.stringify({settingsVersion:2,transcriptRevision:transcript.revision??0,current,rate,fontSize,gap,repeats,autoScroll,showPhonetics,ipaStyle,anchor,selectionEnd,manualRange}))}catch{}}
 function run(){if(!audio||pendingPosition!==null||audio.seeking){frame=requestAnimationFrame(run);return;}current=audio.currentTime;
 if(previewing&&range&&!audio.paused&&current*1000>=range.endMs){audio.pause();playing=false;previewing=false}
 if(looping&&range&&!waiting&&!audio.paused&&current*1000>=range.endMs){const action=loopBoundary(current*1000,range.endMs,completed,repeats);audio.pause();completed++;if(action==='finish'){looping=false;playing=false}else{scheduleWait(gap*1000)}}
 if(autoScroll&&!followPaused&&playing&&!waiting&&!controls&&!selecting&&!showEditors)followReading();
 if(Date.now()-savedAt>1500){save();savedAt=Date.now()}frame=requestAnimationFrame(run)}
 function wordBounds(index:number){
  const element=document.getElementById(tokens[index]?.id??'');
  return element?.getClientRects()[0];
 }
 function followReading(force=false){
  const now=performance.now();
  if(!force&&(now-lastFollowCheck<100||now<scrollCooldownUntil))return;
  lastFollowCheck=now;
  const index=activeIndex(tokens,current*1000);
  if(index<0)return;
  const active=wordBounds(index);if(!active)return;
  const header=document.querySelector('.site-header')?.getBoundingClientRect();
  const dock=document.querySelector('.player-dock')?.getBoundingClientRect();
  const visible=window.visualViewport;
  const viewport={top:Math.max(visible?.offsetTop??0,header?.bottom??0)+12,bottom:Math.min((visible?.offsetTop??0)+(visible?.height??window.innerHeight),dock?.top??window.innerHeight)-12};
  let line=active;
  const next=tokens[index+1];
  if(!force&&!followForce&&next&&next.startMs-current*1000>=0&&next.startMs-current*1000<=400*rate&&(!looping||!range||next.startMs<range.endMs)){
   const upcoming=wordBounds(index+1);
   if(upcoming&&upcoming.top>active.top+10)line=upcoming;
  }
  const delta=readingScrollDelta(line,viewport,force||followForce);
  followForce=false;
  if(delta===0)return;
  const target=Math.max(0,Math.min(window.scrollY+delta,document.documentElement.scrollHeight-window.innerHeight));
  if(Math.abs(target-window.scrollY)<8)return;
  window.scrollTo({top:target,behavior:window.matchMedia('(prefers-reduced-motion: reduce)').matches?'instant':'smooth'});
  scrollCooldownUntil=now+450;
 }
 function resumeFollowing(){autoScroll=true;followPaused=false;followForce=true;scrollCooldownUntil=0;followReading(true);save()}
 function manualScroll(event:Event){
  const target=event.target;
  if(target instanceof Element&&target.closest('dialog,.player-dock,input,textarea,select'))return;
  if(autoScroll&&transcript)followPaused=true;
 }
 function scrollKey(event:KeyboardEvent){
  if(!['ArrowUp','ArrowDown','PageUp','PageDown','Home','End',' '].includes(event.key))return;
  if(event.target instanceof Element&&event.target.closest('button,a,input,textarea,select,dialog'))return;
  manualScroll(event);
 }
 function scrollbarPointer(event:PointerEvent){if(event.clientX>=document.documentElement.clientWidth)manualScroll(event)}
 function seeking(){
  followForce=true;scrollCooldownUntil=0;lastFollowCheck=0;
  // Stop a smooth scroll aimed at the position before this seek.
  window.scrollTo({top:window.scrollY,behavior:'instant'});
 }
 async function seeked(){
  current=audio.currentTime;
  followForce=true;scrollCooldownUntil=0;lastFollowCheck=0;
  await tick();
  if(!audio.seeking&&autoScroll&&!followPaused&&!controls&&!selecting&&!showEditors)followReading(true);
  save();
 }
 function metadata(){duration=audio.duration;current=Math.min(pendingPosition??current,Math.max(0,duration-0.1));audio.currentTime=current;pendingPosition=null;audio.playbackRate=rate;if(manualRange&&(manualRange.endMs>duration*1000||manualRange.startMs<0))manualRange=null}
 async function refreshJob(){
  const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/job`);
  if(r.ok){job=await r.json();jobError=''}
 }
 async function retryJob(){try{if(job.kind==='ipa'&&job.voice){ipaPending=job.voice;ipaError=''}const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/retry`,{method:'POST'});if(!r.ok)throw new Error(await r.text());await refreshJob()}catch(e){jobError=e instanceof Error?e.message:'Không thử lại được.'}}
 function editQueued(){pause();disableLoop();void refreshJob()}
 function previewSegment(startMs:number,endMs:number){disableLoop();anchor=null;selectionEnd=null;manualRange={startMs,endMs};listenPreview()}
 onMount(()=>{
  let disposed=false,polling=false,refreshForCache=false,loadedRevision='';const abort=new AbortController();
  const loadTranscript=async(marker:string)=>{
     const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/transcript`,{signal:abort.signal});
     if(!r.ok)throw new Error('Không tải được lời. Kiểm tra backend đang chạy.');
     const data:Transcript=await r.json();if(disposed)return;
     if(transcript&&(transcript.revision??0)!==(data.revision??0)){disableLoop();anchor=null;selectionEnd=null;manualRange=null}
     const firstLoad=transcript===null;transcript=data;recordingTitle=data.title;loadedRevision=r.headers.get('X-PQ-Dharma-Offline')==='1'?'offline':marker;offlineReading=loadedRevision==='offline';
     if(firstLoad){
      ipaChoice=data.defaultIpaVoice??'en-gb';ipaDisplay=data.defaultIpaVoice??'';
      try{const v=JSON.parse(localStorage.getItem(`dharma-player:${recordingId}`)||'null');if(v){current=Math.max(0,Number(v.current)||0);rate=[.5,.75,1,1.25,1.5].includes(v.rate)?v.rate:1;fontSize=[20,22,26,30].includes(v.fontSize)?v.fontSize:22;const practice=practicePreferences(v);gap=practice.gap;repeats=practice.repeats;autoScroll=v.autoScroll!==false;showPhonetics=v.showPhonetics!==false;ipaStyle=v.ipaStyle==='original'?'original':'readable';if((v.transcriptRevision??0)===(data.revision??0)&&Number.isInteger(v.anchor)&&Number.isInteger(v.selectionEnd)&&v.anchor>=0&&v.selectionEnd>=0&&v.anchor<tokens.length&&v.selectionEnd<tokens.length){anchor=v.anchor;selectionEnd=v.selectionEnd}if((v.transcriptRevision??0)===(data.revision??0)&&v.manualRange&&Number.isFinite(v.manualRange.startMs)&&Number.isFinite(v.manualRange.endMs)&&v.manualRange.startMs>=0&&v.manualRange.endMs>=v.manualRange.startMs+200)manualRange=v.manualRange}}catch{}
      pendingPosition=current;
     }
     if(ipaPending&&data.ipaVoices?.includes(ipaPending)){ipaDisplay=ipaPending;ipaChoice=ipaPending;ipaMessage=`Đã tạo IPA ${ipaLabels[ipaPending]}.`;ipaPending=null}
     if(ipaDisplay&&!data.ipaVoices?.includes(ipaDisplay))ipaDisplay=data.defaultIpaVoice??'';
     await tick();error='';
  };
  const poll=async()=>{
   if(disposed||polling)return;polling=true;if(refreshForCache){loadedRevision='';refreshForCache=false}
   try{
    if(!navigator.onLine){offlineReading=true;loadedRevision='offline';if(!transcript)await loadTranscript('offline');syncError='';loading=false;return}
    const jr=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/job`,{signal:abort.signal});
    if(jr.ok){job=await jr.json();syncError='';if(job.kind==='ipa'&&job.status==='failed'){ipaPending=null;ipaError=job.error||'Không tạo được IPA.'}}else if(jr.status!==404)throw new Error('Không tải được trạng thái xử lý.');
    const marker=job.status==='ready' ? (job.id||'initial') : 'previous';
    if((job.status==='ready'||transcript===null&&(job.kind==='edit'||job.kind==='ipa'))&&loadedRevision!==marker){
     await loadTranscript(marker);
    }
    loading=false;
   }catch(e){
    if(!disposed){
     if(!transcript){try{await loadTranscript('offline');offlineReading=true}catch{}}
     if(transcript){offlineReading=true;loadedRevision='offline';syncError=''}else{syncError=navigator.onLine?(e instanceof Error?e.message:'Không tải được bài.'):'Bài này chưa có lời được lưu trên thiết bị. Kết nối mạng và mở bài một lần để lưu lời.'}
     loading=false;
    }
   }
   finally{polling=false;if(refreshForCache&&!disposed)queueMicrotask(()=>{void poll()})}
  };
  void fetch('/api/sutras',{signal:abort.signal}).then(r=>r.json()).then(items=>{if(!disposed){const item=items.find((v:{recordingId:string})=>v.recordingId===recordingId);if(item)recordingTitle=item.title}}).catch(()=>{});
  void poll();frame=requestAnimationFrame(run);const timer=setInterval(poll,2000);const reconnect=()=>{void poll()};window.addEventListener('online',reconnect);const cacheReady=()=>{refreshForCache=true;void poll()};window.addEventListener('pq-dharma:offline-ready',cacheReady);
  window.addEventListener('wheel',manualScroll,{passive:true});window.addEventListener('touchmove',manualScroll,{passive:true});window.addEventListener('keydown',scrollKey);window.addEventListener('pointerdown',scrollbarPointer);
 const hide=()=>save();window.addEventListener('pagehide',hide);return()=>{disposed=true;abort.abort();window.removeEventListener('online',reconnect);window.removeEventListener('pq-dharma:offline-ready',cacheReady);clearInterval(timer);cancelAnimationFrame(frame);clearTimeout(waitTimer);save();if(audio)audio.pause();window.removeEventListener('pagehide',hide);window.removeEventListener('wheel',manualScroll);window.removeEventListener('touchmove',manualScroll);window.removeEventListener('keydown',scrollKey);window.removeEventListener('pointerdown',scrollbarPointer)}
 });
</script>
<svelte:head>
 <title>{recordingTitle} · Dharma</title>
 <meta name="description" content="Nghe kinh, theo từng lời và luyện tụng bằng đoạn bạn chọn."/>
</svelte:head>
<div class="app-shell" class:has-selection={selecting||range!==null}>
 <div class="site-header"><Topbar>
  <div class="topbar-actions"><button class="outline-btn" onclick={shareRecording}>Chia sẻ</button>
  <button class="icon-btn" onclick={()=>controls=true} aria-label="Cài đặt" aria-haspopup="dialog"><Icon name="sliders-horizontal"/></button>
  </div>
 </Topbar>
 <nav class="page-navigation" aria-label="Điều hướng bài kinh"><a class="back-link" href="/library">← Thư viện</a></nav></div>
 <main class="page-content">
  {#if shareMessage}<p role="status" class="saved-copy-note">{shareMessage}</p>{/if}
  <div class="eyebrow mt-6">MỘT CHÚT TĨNH LẶNG MỖI NGÀY</div>
  <div class="title-row"><div class="min-w-0"><h1 class="text-[28px] sm:text-4xl font-semibold tracking-tight leading-tight">{recordingTitle}</h1><p class="mt-2 text-sm text-stone-500">Bản đọc tiếng Anh · Theo từng lời</p></div></div>
  {#if transcript}<div class="import-actions"><button class="outline-btn" onclick={()=>showEditors=!showEditors}>{showEditors?'Ẩn sửa lời':'Chỉnh lời'}</button></div>{/if}
  {#if job.status!=='ready'&&$online}<section class="panel mt-4" aria-live="polite"><div class="processing-heading"><p class="font-semibold">{stageNames[job.status]||job.status}</p><span class="processing-percent">{jobProgress}%</span></div><progress class="processing-progress" max="100" value={jobProgress} aria-label="Tiến độ xử lý bài" aria-valuetext={`${jobProgress}% · ${stageNames[job.status]||job.status}`}>{jobProgress}%</progress>{#if job.progressMessage}<p class="text-sm mt-2 text-stone-600">{job.progressMessage}</p>{/if}<p class="text-xs mt-2 text-stone-500">Tiến độ ước tính theo từng bước xử lý.</p><p class="text-sm mt-2 text-stone-600">{job.kind==='edit'?'Đang cập nhật đoạn lời; bạn vẫn có thể xem bản hiện tại.':'Bạn có thể đóng trang và quay lại. Bài được xử lý trong nền.'}</p>{#if job.status==='failed'}<p class="text-sm text-red-800 mt-2">{job.error||'Không xử lý được audio.'}</p><button class="outline-btn mt-3" disabled={!$online} onclick={retryJob}>Thử lại</button>{/if}{#if jobError}<p role="alert">{jobError}</p>{/if}</section>{/if}
  {#if transcript}<section id="practice" class="practice-card">
   <div class="practice-heading"><div class="min-w-0"><div class="eyebrow">LUYỆN TỪNG ĐOẠN</div><p class="text-sm mt-2 text-stone-600 leading-relaxed" aria-live="polite">{selecting?(anchor===null?'Chạm từ bắt đầu trong lời kinh.':'Chạm từ cuối để hoàn tất.'):range?`${formatPrecise(range.startMs)} — ${formatPrecise(range.endMs)}`:'Chọn một đoạn lời để nghe lại.'}</p></div>
    <button class="outline-btn" disabled={!transcript} onclick={chooseWords}><Icon name={selecting?'x':'text-cursor-input'} size={18}/>{selecting?'Hủy':range?'Chọn lại':'Chọn đoạn'}</button>
   </div>
   {#if range}<p class="selection-preview">{selectedText}</p><div class="practice-actions"><button class="outline-btn" onclick={listenPreview}><Icon name="play" size={16}/>Nghe thử</button><button class="primary-btn" onclick={()=>looping?disableLoop():startLoop()}><Icon name={looping?'x':'repeat'} size={18}/>{looping?'Dừng lặp':'Lặp đoạn'}</button></div>{/if}
   <p class="practice-hint">{repeats===0?'Lặp liên tục':`Lặp ${repeats} lượt`} · {gap===0?'Không nghỉ':`Nghỉ ${String(gap).replace('.',',')} giây`}</p>
  </section>
  {/if}
  {#if offlineReading&&$online}<p class="saved-copy-note" role="status">Đang xem bản lời hiện có. Kết nối để cập nhật trạng thái bài.</p>{/if}
  {#if transcript}<div class="reading-heading"><span class="eyebrow">LỜI KINH</span><span class="text-xs text-stone-500">{selecting?'Chạm để chọn':'Chạm từ để nghe'}</span></div>{/if}
  {#if error||syncError}<div role="alert" class="panel text-red-800">{error||syncError}<button class="outline-btn mt-2" disabled={!$online} onclick={()=>location.reload()}>Thử lại</button></div>{/if}
  {#if loading}<div class="panel animate-pulse">Đang tải lời kinh…</div>{/if}
  {#if transcript}
   <div class="transcript" style={`--reading-size:${fontSize}px`}>
    {#each transcript.segments as segment, si (segment.id)}
     <section id={segment.id} class:active-segment={si===segmentIndex} class="verse"><div class="verse-meta"><span>{String(si+1).padStart(2,'0')}</span><span>{formatTime(segment.startMs/1000)}</span></div><p>
      {#each segment.tokens as token (token.id)}
       {@const phonetic=phoneticFor(token)}
       {@const index=tokenPositions.get(token.id)!}
       <button id={token.id} class="word" class:spoken={index===tokenIndex} class:chosen={(range!==null&&token.startMs<range.endMs&&token.endMs>range.startMs)||(selecting&&index===anchor)} onclick={()=>choose(index)} aria-label={`${selecting?'Chọn':'Nghe từ'} ${token.text}`} aria-pressed={index===tokenIndex}>{token.text}{#if showPhonetics && phonetic}<span class="phonetic" aria-hidden="true">{phonetic.type === 'ipa' ? `/${ipaStyle==='original'?phonetic.text:(phonetic.readable??phonetic.text.replaceAll('ɹ','r'))}/` : phonetic.text}</span>{/if}</button>{' '}
      {/each}
     </p>{#if showEditors}<SegmentEditor {recordingId} {segment} busy={jobBusy||!$online} onqueued={editQueued} onlisten={()=>previewSegment(segment.startMs,segment.endMs)}/>{/if}</section>
    {/each}
   </div><p class="text-xs text-stone-500 mt-6 leading-6">Lời được tạo tự động · Phiên âm IPA chuẩn, có thể khác cách đọc trong audio.{#if tokens.some(t=>t.endEstimated)}<br/>Một số mốc thời gian được ước lượng; thao tác nghe từng từ có thể lệch nhẹ.{/if}<br/>{#if transcript.sourceUrl}<a class="underline" href={transcript.sourceUrl} target="_blank" rel="noreferrer">Xem bản gốc</a>{/if}</p>
  {/if}
 </main>
 {#if transcript}<audio bind:this={audio} src={`/api/recordings/${encodeURIComponent(recordingId)}/audio${recordingId==='sample'?'?v=sample-cbr-1':''}`} preload="metadata" onloadedmetadata={metadata} onseeking={seeking} onseeked={seeked} onended={()=>{if(!looping){playing=false;previewing=false}}} onerror={()=>{error=$online?'Không tải được audio.':'';playing=false}} onpause={()=>{if(!waiting)playing=false}} onplay={()=>playing=true}></audio>{/if}
 {#if transcript}<footer class="player-dock"><div class="player-inner">
  {#if selecting||range}
   <div class="dock-practice">
    {#if selecting}
     <p aria-live="polite">{anchor===null?'Chạm từ bắt đầu trong lời kinh.':'Chạm từ cuối để hoàn tất.'}</p><button class="outline-btn" onclick={clearRange}>Hủy</button>
    {:else if range}
     <p class="dock-range">{formatPrecise(range.startMs)} — {formatPrecise(range.endMs)}</p>
     <button class="outline-btn dock-preview" onclick={listenPreview} aria-label="Nghe thử đoạn đã chọn"><Icon name="play" size={16}/><span>Nghe thử</span></button>
     <button class="primary-btn" onclick={()=>looping?disableLoop():startLoop()}><Icon name={looping?'x':'repeat'} size={16}/>{looping?'Dừng lặp':'Lặp'}</button>
     <details class="range-menu"><summary aria-label="Tùy chọn đoạn đã chọn">⋯</summary><div><button onclick={e=>{e.currentTarget.closest('details')?.removeAttribute('open');chooseWords()}}>Chọn lại</button><button onclick={clearRange}>Bỏ chọn</button></div></details>
    {/if}
   </div>
  {/if}
  {#if followPaused&&autoScroll&&!selecting}<button class="follow-reading-btn" onclick={resumeFollowing}>Theo lời đang đọc ↑</button>{/if}
  <p class="sr-only" aria-live="polite">{waiting?(playing?'Đến lượt bạn tụng…':'Đã tạm dừng lượt tụng'):looping?`Đang lặp · ${completed+1}${repeats?`/${repeats}`:''}`:''}</p>
  <div class="dock-progress"><span>{formatPlayerTime(current,duration)}</span><label class="sr-only" for="progress">Vị trí phát</label><input id="progress" class="progress" type="range" min="0" max="100" step="any" value={progress} aria-valuetext={`${formatPlayerTime(current,duration)} trên ${formatPlayerTime(duration,duration)}`} disabled={!transcript||!duration||!$online} oninput={e=>seek(Number(e.currentTarget.value)/100*duration)}/><span>{formatPlayerTime(duration,duration)}</span></div>
  <div class="transport">
   <button class="icon-btn loop-shortcut" class:loop-active={looping} disabled={!transcript||!$online} onclick={handleLoopShortcut} aria-label={looping?'Dừng lặp':selecting?'Hủy chọn đoạn':range?'Lặp đoạn đã chọn':'Chọn đoạn để lặp'} aria-pressed={looping||selecting}><Icon name="repeat" size={20}/></button>
   <button class="skip-btn" disabled={!transcript||!$online} onclick={()=>seek(current-10)} aria-label="Lùi 10 giây"><Icon name="rotate-ccw" size={20}/><span>10s</span></button>
   <button class="play-btn" disabled={!transcript||!$online&&!playing} onclick={togglePlay} aria-label={playing?'Tạm dừng':'Phát'}><Icon name={playing?'pause':'play'} size={22}/></button>
   <button class="skip-btn" disabled={!transcript||!$online} onclick={()=>seek(current+10)} aria-label="Tiến 10 giây"><Icon name="rotate-cw" size={20}/><span>10s</span></button>
   <button class="speed-btn" onclick={()=>controls=true} aria-label="Chỉnh tốc độ phát" aria-haspopup="dialog">{rate}×</button>
  </div>
 </div></footer>{/if}
</div>
<dialog bind:this={settingsDialog} class="settings-dialog player-settings-dialog" onclose={()=>controls=false} oncancel={()=>controls=false} aria-labelledby="settings-title">
 <div class="settings-header"><h2 id="settings-title" class="text-lg font-semibold">Cài đặt nghe</h2><button class="icon-btn" onclick={()=>controls=false} aria-label="Đóng cài đặt"><Icon name="x"/></button></div>
 <div class="settings-content">
  <fieldset><legend>Tốc độ phát</legend><div class="speed-options">{#each [.5,.75,1,1.25,1.5] as value}<button class="chip" class:chip-active={rate===value} aria-pressed={rate===value} onclick={()=>changeRate(value)}>{value}×</button>{/each}</div></fieldset>
  <div class="control-grid"><label>Số lượt lặp<select bind:value={repeats} onchange={()=>{disableLoop();save()}}><option value={0}>Liên tục</option><option value={3}>3 lượt</option><option value={5}>5 lượt</option><option value={10}>10 lượt</option></select></label><label>Nghỉ giữa lượt<select bind:value={gap} onchange={save}><option value={0}>Không nghỉ</option><option value={.5}>0,5 giây</option><option value={1}>1 giây</option><option value={2}>2 giây</option><option value={4}>4 giây</option><option value={6}>6 giây</option></select></label></div>
  <div class="control-grid"><label>Cỡ chữ<select bind:value={fontSize} onchange={save}><option value={20}>Nhỏ</option><option value={22}>Vừa</option><option value={26}>Lớn</option><option value={30}>Rất lớn</option></select></label><label class="scroll-setting"><input type="checkbox" bind:checked={autoScroll} onchange={()=>{followPaused=false;followForce=true;save()}}/>Tự cuộn theo lời</label></div>
  <label>Cách hiển thị IPA<select bind:value={ipaStyle} onchange={save}><option value="readable">IPA dễ đọc</option><option value="original">IPA gốc</option></select></label>
  {#if transcript?.ipaEditable}
   <fieldset class="ipa-settings"><legend>Phiên âm IPA</legend>
    <label>Giọng IPA<select bind:value={ipaChoice} onchange={chooseIPAVoice} disabled={ipaSaving}><option value="en-gb">Anh–Anh</option><option value="en-us">Anh–Mỹ</option></select></label>
    <p class="ipa-caption">{transcript.defaultIpaVoice?`Mặc định của bài: ${ipaLabels[transcript.defaultIpaVoice]}.`:'Bài này đang dùng IPA cũ, chưa xác định giọng.'}</p>
    {#if !ipaReady}<p class="ipa-caption">Chưa có bản {ipaLabels[ipaChoice]} hoàn chỉnh. Tạo IPA để sử dụng; bản hiện tại vẫn được giữ.</p>{/if}
    <div class="ipa-actions"><button class="outline-btn" disabled={jobBusy||ipaSaving||!$online} onclick={generateIPA}>{ipaReady?'Tạo lại IPA':`Tạo IPA ${ipaLabels[ipaChoice]}`}</button><button class="outline-btn" disabled={!ipaReady||jobBusy||ipaSaving||!$online||transcript.defaultIpaVoice===ipaChoice} onclick={setDefaultIPA}>Đặt mặc định cho bài</button></div>
    {#if job.kind==='ipa'&&jobBusy}<div class="ipa-job" aria-live="polite"><progress class="processing-progress" max="100" value={jobProgress} aria-label="Tiến độ tạo IPA"></progress><p class="ipa-caption">{jobProgress}% · {job.progressMessage||'Đang tạo IPA…'}</p></div>{/if}
    {#if ipaError}<p class="text-xs text-red-800" role="alert">{ipaError}</p>{/if}
    {#if ipaMessage}<p class="ipa-caption" role="status">{ipaMessage}</p>{/if}
   </fieldset>
  {/if}
  <label class="scroll-setting"><input type="checkbox" bind:checked={showPhonetics} onchange={save}/>Hiển thị phiên âm IPA</label>
 </div>
 <div class="settings-footer"><button class="primary-btn w-full" onclick={()=>{save();controls=false}}><Icon name="check" size={18}/>Xong</button></div>
</dialog>
