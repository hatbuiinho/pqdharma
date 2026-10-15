<script lang="ts">
 import Popup from '$lib/Popup.svelte';
 import {auth} from './auth';
 import {untrack} from 'svelte';
 let {open=$bindable(false)}:{open?:boolean}=$props();
 const formId=$props.id();
 let name=$state(''),busy=$state(false),error=$state('');
 $effect(()=>{if(open){name=untrack(()=>$auth?.name||'');error=''}});
 async function submit(e:SubmitEvent){e.preventDefault();if(busy)return;error='';busy=true;try{const r=await fetch('/api/auth/profile',{method:'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify({name:name.trim()})});if(!r.ok)throw new Error(await r.text());const saved=await r.json();auth.update(user=>user?{...user,name:saved.name}:user);open=false}catch(e){error=e instanceof Error?e.message:'Không lưu được tên'}finally{busy=false}}
</script>
<Popup {open} title="Sửa tên hiển thị" {busy} class="user-dialog" onrequestclose={()=>open=false} onclose={()=>open=false}>
 <form id={formId} class="upload-form" onsubmit={submit}><label>Tên hiển thị<input bind:value={name} required maxlength={100} autocomplete="name" disabled={busy}/></label><p>Tên này hiển thị trên tài khoản của bạn. Email đăng nhập giữ nguyên.</p>{#if error}<p role="alert" class="danger-text">{error}</p>{/if}</form>
 {#snippet footer()}<div class="user-dialog-actions"><button type="button" class="outline-btn" disabled={busy} onclick={()=>open=false}>Hủy</button><button type="submit" form={formId} class="primary-btn" disabled={busy||!name.trim()}>{busy?'Đang lưu…':'Lưu tên'}</button></div>{/snippet}
</Popup>
