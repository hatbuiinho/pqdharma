<script lang="ts">
 import Popup from '$lib/Popup.svelte';
 import {onMount} from 'svelte';
 import ChangePassword from './ChangePassword.svelte';
 import AccountName from './AccountName.svelte';
 let nameOpen=$state(false);
 let passwordOpen=$state(false);
 import {auth,refreshAuth} from './auth';
 const formId=$props.id();
 let control:HTMLDivElement;
 let open=$state(false),loginOpen=$state(false),email=$state(''),password=$state(''),message=$state(''),busy=$state(false),create=$state(false);
 onMount(()=>{void refreshAuth()});
 async function submit(e:SubmitEvent){e.preventDefault();busy=true;message='';try{const r=await fetch(create?'/api/users':'/api/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({email,password})});if(!r.ok)throw new Error(await r.text());password='';if(create){message='Đã tạo tài khoản.'}else{await refreshAuth();location.assign('/library')}}catch(e){message=e instanceof Error?e.message:'Không thực hiện được'}finally{busy=false}}
 async function logout(){await fetch('/api/auth/logout',{method:'POST'});auth.set(null);location.assign('/library')}
</script>
<svelte:window onpointerdown={e=>{if(control&&!control.contains(e.target as Node))open=false}} onkeydown={e=>{if(e.key==='Escape')open=false}}/>
<div class="profile-control" bind:this={control}>
 <button class="outline-btn profile-trigger" aria-label={$auth?'Mở menu tài khoản':'Mở menu đăng nhập'} aria-expanded={open} onclick={()=>open=!open}><svg class="profile-trigger-icon" width="21" height="21" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="8" r="4"/><path d="M5 21v-2a7 7 0 0 1 14 0v2"/></svg><span class="profile-trigger-label">{$auth?($auth.name||$auth.email.split('@')[0]):'Đăng nhập'}</span></button>
 {#if open}<div class="profile-dropdown">
  {#if $auth}<div class="profile-account-summary"><strong>{$auth.name||$auth.email.split('@')[0]}</strong><p>{$auth.email}</p></div><button onclick={()=>{open=false;nameOpen=true}}>Sửa tên hiển thị</button><a href="/library">Thư viện của tôi</a>{#if $auth.admin}<a href="/users" onclick={()=>open=false}>Quản lý user</a>{/if}<button onclick={()=>{open=false;passwordOpen=true}}>Đổi mật khẩu</button><button onclick={()=>void logout()}>Đăng xuất</button>
  {:else}<button onclick={()=>{create=false;loginOpen=true;open=false;message=''}}>Đăng nhập</button>{/if}
 </div>{/if}
</div>
<Popup open={loginOpen} title="Đăng nhập" {busy} class="user-dialog" onrequestclose={()=>{loginOpen=false;password=''}} onclose={()=>{loginOpen=false;password=''}}>
 <form id={formId} onsubmit={submit} class="upload-form"><label>Email<input type="email" bind:value={email} required autocomplete="username" disabled={busy}/></label><label>Mật khẩu<input type="password" bind:value={password} required maxlength={72} autocomplete="current-password" disabled={busy}/></label>{#if message}<p role="status">{message}</p>{/if}</form>
 {#snippet footer()}<div class="user-dialog-actions"><button type="button" class="outline-btn" disabled={busy} onclick={()=>{loginOpen=false;password=''}}>Đóng</button><button type="submit" form={formId} class="primary-btn" disabled={busy}>{busy?'Đang xử lý…':'Đăng nhập'}</button></div>{/snippet}
</Popup>

<ChangePassword bind:open={passwordOpen} onlogin={()=>{create=false;message='';password='';loginOpen=true}}/>

<AccountName bind:open={nameOpen}/>
