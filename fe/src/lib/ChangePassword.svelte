<script lang="ts">
 import Popup from '$lib/Popup.svelte';
 import {auth} from './auth';
 let {open=$bindable(false),onlogin}:{open?:boolean;onlogin:()=>void}=$props();
 const formId=$props.id();
 let current=$state(''),password=$state(''),confirmation=$state(''),busy=$state(false),error=$state(''),done=$state(false),show=$state(false);
 function clear(){current='';password='';confirmation='';error='';done=false;show=false;open=false}
 async function submit(e:SubmitEvent){
  e.preventDefault();if(busy)return;error='';
  if(password!==confirmation){error='Mật khẩu xác nhận chưa khớp.';return}
  const length=new TextEncoder().encode(password).length;
  if(password.length<12||length>72){error='Mật khẩu cần ít nhất 12 ký tự và tối đa 72 byte (ký tự có dấu có thể chiếm nhiều byte).';return}
  if(current===password){error='Mật khẩu mới phải khác mật khẩu hiện tại.';return}
  busy=true;
  try{const r=await fetch('/api/auth/password',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({currentPassword:current,newPassword:password})});if(!r.ok)throw new Error(await r.text());current='';password='';confirmation='';done=true;auth.set(null)}catch(e){error=e instanceof Error?e.message:'Không đổi được mật khẩu. Vui lòng thử lại.'}finally{busy=false}
 }
</script>
<Popup {open} title={done?'Đã đổi mật khẩu':'Đổi mật khẩu'} {busy} class="user-dialog" onclose={clear} onrequestclose={()=>open=false}>
 {#if done}<div class="upload-form"><p role="status">Mật khẩu mới đã được lưu. Các phiên đăng nhập đã kết thúc.</p></div>
 {:else}<form id={formId} class="upload-form" onsubmit={submit}>
 <p>Sau khi đổi mật khẩu, bạn cần đăng nhập lại trên các thiết bị.</p>
 <label>Mật khẩu hiện tại<input type="password" bind:value={current} autocomplete="current-password" required disabled={busy}/></label>
 <label>Mật khẩu mới<input type={show?'text':'password'} bind:value={password} autocomplete="new-password" required minlength={12} maxlength={72} disabled={busy}/><small>Từ 12 ký tự, tối đa 72 byte.</small></label>
 <label>Xác nhận mật khẩu mới<input type={show?'text':'password'} bind:value={confirmation} autocomplete="new-password" required minlength={12} maxlength={72} disabled={busy}/></label>
 <label class="password-visibility"><input type="checkbox" bind:checked={show} disabled={busy}/>Hiện mật khẩu mới</label>
 {#if error}<p role="alert" class="danger-text">{error}</p>{/if}
 </form>{/if}
 {#snippet footer()}{#if done}<button type="button" class="primary-btn" onclick={()=>{open=false;onlogin()}}>Đăng nhập lại</button>{:else} <div class="user-dialog-actions"><button type="button" class="outline-btn" disabled={busy} onclick={()=>open=false}>Hủy</button><button type="submit" form={formId} class="primary-btn" disabled={busy}>{busy?'Đang lưu…':'Đổi mật khẩu'}</button></div>
{/if}{/snippet}
</Popup>
