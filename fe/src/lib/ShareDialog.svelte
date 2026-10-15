<script lang="ts">
 import Popup from '$lib/Popup.svelte';
 import {onMount} from 'svelte';
 import Combobox from './Combobox.svelte';
 import UserPicker from './UserPicker.svelte';
 import {shareRecordingLink} from './share';
 type Person={id:string;email:string;name?:string;disabled?:boolean};
 let {recordingId,title,onclose,onsaved}:{recordingId:string;title:string;onclose:()=>void;onsaved:(visibility:string)=>void}=$props();
 let nativeShare=$state(false),sharing=$state(false);
 let loading=$state(true),busy=$state(false),owned=$state(false),error=$state(''),notice=$state(''),copying=$state(false),discard=$state(false);
 let owner=$state<Person|null>(null),people=$state<Person[]>([]),emails=$state<string[]>([]),general=$state('restricted'),original=$state(''),originalVisibility=$state('private');
 const snapshot=$derived(JSON.stringify({general,emails:[...emails].sort()}));
 const dirty=$derived(!loading&&owned&&snapshot!==original);
 const link=$derived(typeof location==='undefined'?'':new URL(`/recordings/${encodeURIComponent(recordingId)}`,location.origin).href);
 const visibility=$derived(general==='public'?'public':emails.length?'shared':'private');
 onMount(()=>{
  const payload={title,url:link};
  nativeShare=typeof navigator.share==='function';
  if(nativeShare&&typeof navigator.canShare==='function'){try{nativeShare=navigator.canShare(payload)}catch{nativeShare=false}}
  const controller=new AbortController();
  void (async()=>{try{const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/access`,{signal:controller.signal,cache:'no-store'});if(!r.ok)throw new Error(await r.text());const data=await r.json();owned=data.owned;owner=data.owner;people=data.users;emails=data.emails;general=data.visibility==='public'?'public':'restricted';originalVisibility=data.visibility;original=JSON.stringify({general,emails:[...data.emails].sort()})}catch(e){if(!controller.signal.aborted)error=e instanceof Error?e.message:'Không tải được quyền truy cập'}finally{if(!controller.signal.aborted)loading=false}})();
  return()=>controller.abort();
 });
 function close(){if(busy||sharing)return;if(dirty){discard=true;return}onclose()}
 async function save(){
  if(busy||loading||!owned||!dirty)return;busy=true;error='';notice='';
  try{const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/access`,{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({visibility,emails})});if(!r.ok)throw new Error(await r.text());original=snapshot;originalVisibility=visibility;onsaved(visibility);notice='Đã lưu quyền truy cập.'}catch(e){error=e instanceof Error?e.message:'Không lưu được quyền'}finally{busy=false}
 }
 async function share(){
  if(sharing||busy||loading||!original||!nativeShare)return;
  if(dirty){notice='Hãy lưu thay đổi quyền trước khi chia sẻ.';return}
  sharing=true;notice='';
  try{notice=await shareRecordingLink(recordingId,title)}finally{sharing=false}
 }

 async function copy(){copying=true;notice='';try{await navigator.clipboard.writeText(link);notice=dirty?'Đã sao chép liên kết. Hãy lưu thay đổi quyền trước khi gửi.':'Đã sao chép liên kết.'}catch{notice='Chọn và sao chép liên kết trong ô bên dưới.'}finally{copying=false}}
</script>
<Popup open title="Chia sẻ bài nghe" subtitle={title} busy={busy||sharing} wide class="user-dialog share-dialog" onrequestclose={close}>
 {#if loading}<p role="status">Đang tải quyền truy cập…</p>{:else if original}
 {#if owned}<UserPicker bind:emails disabled={busy||sharing} showSelected={false} onadd={person=>{people=[...people.filter(p=>p.email!==person.email),person]}}/>{/if}
 <section class="share-section" aria-labelledby="share-people"><h3 id="share-people">Người có quyền truy cập</h3>
 {#if owner?.id}<div class="share-person"><div class="share-avatar" aria-hidden="true">{(owner.name||owner.email||'C').charAt(0).toLocaleUpperCase('vi')}</div><div class="share-person-info"><strong>{owner.name||owner.email||'Chủ sở hữu'}</strong>{#if owner.email}<small>{owner.email}</small>{/if}</div><span class="share-role">Chủ sở hữu</span></div>{/if}
 {#if owned}{#each emails as email (email)}{@const person=people.find(p=>p.email===email)}<div class="share-person"><div class="share-avatar" aria-hidden="true">{(person?.name||email).charAt(0).toLocaleUpperCase('vi')}</div><div class="share-person-info"><strong>{person?.name||email}</strong>{#if person?.name}<small>{email}</small>{/if}{#if person?.disabled}<small>Đã vô hiệu hóa</small>{/if}</div><span class="share-role">Người xem</span><button type="button" class="share-remove" disabled={busy||sharing} aria-label={`Gỡ quyền của ${email}`} onclick={()=>{emails=emails.filter(e=>e!==email);notice=''}}>Gỡ</button></div>{/each}
 {:else}<p class="share-help">Chỉ chủ sở hữu có thể thay đổi quyền truy cập.</p>{/if}
 </section>
 <section class="share-section" aria-labelledby="share-general"><h3 id="share-general">Quyền truy cập chung</h3>
 {#if owned}<Combobox label="Quyền truy cập chung" bind:value={general} disabled={busy||sharing} options={[{value:'restricted',label:'Bị giới hạn'},{value:'public',label:'Bất kỳ ai có liên kết'}]}/>{:else}<p>{originalVisibility==='public'?'Bất kỳ ai có liên kết':'Bị giới hạn'}</p>{/if}
 <p class="share-help">{general==='public'?'Bất kỳ ai có liên kết đều có thể nghe mà không cần đăng nhập. Bài cũng xuất hiện trong mục Khám phá.':'Chỉ chủ sở hữu và những người được thêm mới có thể truy cập. Người xem cần đăng nhập đúng tài khoản.'}</p>
 </section>
 <label class="share-link-label">Liên kết bài nghe<input class="share-link" readonly value={link} onclick={e=>e.currentTarget.select()}/></label>
 {/if}
 {#if error}<p role="alert" class="danger-text">{error}</p>{/if}
 {#if notice}<p role="status" class="share-help">{notice}</p>{/if}
 {#snippet footer()}
 {#if discard}<div class="share-discard" role="alert"><p>Bạn có thay đổi quyền chưa lưu.</p><div class="user-dialog-actions"><button type="button" class="outline-btn" onclick={()=>discard=false}>Tiếp tục chỉnh sửa</button><button type="button" class="outline-btn danger-text" onclick={onclose}>Bỏ thay đổi</button></div></div>{/if}
 {#if nativeShare&&dirty}<p class="share-help share-footer-note">Lưu thay đổi quyền trước khi chia sẻ qua ứng dụng.</p>{/if}
 <div class="share-footer"><div class="share-link-actions"><button type="button" class="outline-btn" disabled={loading||copying||busy||sharing||!original} onclick={()=>void copy()}>{copying?'Đang sao chép…':'Sao chép liên kết'}</button>{#if nativeShare}<button type="button" class="outline-btn" disabled={loading||busy||copying||sharing||dirty||!original} onclick={()=>void share()}>{sharing?'Đang chia sẻ…':'Chia sẻ…'}</button>{/if}</div><button type="button" class="primary-btn" disabled={busy||loading||sharing} onclick={()=>dirty?void save():close()}>{busy?'Đang lưu…':dirty?'Lưu thay đổi':'Xong'}</button></div>
 {/snippet}
</Popup>
