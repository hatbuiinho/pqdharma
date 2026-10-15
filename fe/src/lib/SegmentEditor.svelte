<script lang="ts">
 import { formatPrecise, type Segment } from './player';
 let {recordingId,segment,busy,onqueued,onlisten}=$props<{recordingId:string;segment:Segment;busy:boolean;onqueued:()=>void;onlisten:()=>void}>();
 const estimated=$derived(segment.tokens.filter((token:Segment['tokens'][number])=>token.endEstimated));
 let editing=$state(false),text=$state(''),saving=$state(false),error=$state('');
 async function submit(){
  saving=true;error='';
  try{const r=await fetch(`/api/recordings/${encodeURIComponent(recordingId)}/segments/${encodeURIComponent(segment.id)}`,{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({text})});if(!r.ok)throw new Error(await r.text());editing=false;onqueued()}
  catch(e){error=e instanceof Error?e.message:'Không lưu được đoạn lời.'}finally{saving=false}
 }
</script>
{#if editing}
 <div class="segment-editor">
  <label for={`edit-${segment.id}`}>Sửa lời đoạn này</label>
  <textarea id={`edit-${segment.id}`} bind:value={text} maxlength={4000} rows={3} disabled={saving}></textarea>
  <p class="text-xs text-stone-500">Lưu sẽ căn lại thời gian và tạo IPA cho đoạn này. Bản đang nghe được giữ đến khi xử lý xong.</p>
  {#if error}<p role="alert" class="text-sm text-red-800">{error}</p>{/if}
  <div class="practice-actions"><button class="outline-btn" onclick={onlisten} disabled={saving}>Nghe đoạn</button><button class="primary-btn" onclick={submit} disabled={saving||busy||!text.trim()||text.trim()===segment.text}>{saving?'Đang lưu…':'Lưu & căn lại'}</button><button class="outline-btn" onclick={()=>editing=false} disabled={saving}>Hủy</button></div>
 </div>
{:else}
 <button class="edit-link" disabled={busy} onclick={()=>{text=segment.text;error='';editing=true}}>Sửa lời</button>
{/if}

{#if estimated.length}
 <details class="timing-details">
  <summary>Mốc thời gian ước lượng · {estimated.length} từ/cụm</summary>
  <p>Những từ/cụm này cần nghe lại để kiểm tra thời gian.</p>
  <ul>{#each estimated as token (token.id)}<li><span>{token.text}</span><span>{formatPrecise(token.startMs)} — {formatPrecise(token.endMs)}</span></li>{/each}</ul>
 </details>
{/if}
