<script lang="ts">
 import Popup from '$lib/Popup.svelte';
 import Combobox from '$lib/Combobox.svelte';
 import {onMount} from 'svelte';
 import {auth,refreshAuth,type User} from '$lib/auth';
 import Topbar from '$lib/Topbar.svelte';
 let users=$state<User[]>([]),query=$state(''),name=$state(''),email=$state(''),password=$state(''),admin=$state(false),target=$state<User|null>(null),mode=$state<'create'|'password'|'status'|'name'|null>(null),busy=$state(false),loading=$state(true),message=$state(''),error=$state(''),formError=$state(''),status=$state('all');
 const formId=$props.id();
 let visible=$derived(users.filter(u=>status==='all'||(status==='disabled'?u.disabled:!u.disabled)));
 onMount(()=>{void refreshAuth()});
 $effect(()=>{
  const q=query; if(!$auth?.admin){loading=false;return}
  const controller=new AbortController();loading=true;error='';
  const timer=setTimeout(async()=>{try{const r=await fetch(`/api/users?include_disabled=true&q=${encodeURIComponent(q)}`,{signal:controller.signal});if(!r.ok)throw new Error(await r.text());users=await r.json()}catch(e){if(!controller.signal.aborted)error=e instanceof Error?e.message:'Không tải được tài khoản'}finally{if(!controller.signal.aborted)loading=false}},250);
  return()=>{clearTimeout(timer);controller.abort()};
 });
 function edit(next:'create'|'password'|'status'|'name',user:User|null=null){target=user;mode=next;email=user?.email||'';name=user?.name||'';password='';admin=false;formError='';}
 function close(){if(busy)return;mode=null;password='';formError=''}
 async function submit(e:SubmitEvent){
  e.preventDefault();if(busy)return;busy=true;formError='';message='';
  const action=mode,person=target;
  try{
   const path=action==='create'?'/api/users':`/api/users/${encodeURIComponent(person!.id)}${action==='status'?'/status':action==='name'?'/profile':''}`;
   const body=action==='status'?{disabled:!person!.disabled}:action==='create'?{email,password,admin,name}:action==='name'?{name}:{password};
   const r=await fetch(path,{method:action==='create'?'POST':'PATCH',headers:{'Content-Type':'application/json'},body:JSON.stringify(body)});
   if(!r.ok)throw new Error(await r.text());
   message=action==='name'?`Đã cập nhật tên cho ${person!.email}.`:action==='create'?`Đã tạo ${admin?'quản trị viên':'tài khoản'} ${email}.`:action==='status'?`${person!.disabled?'Đã kích hoạt lại':'Đã vô hiệu hóa'} ${person!.email}.`:`Đã đặt lại mật khẩu cho ${person!.email}. Các phiên đăng nhập đã kết thúc.`;
   mode=null;password='';
   if(action==='password'&&person?.id===$auth?.id){await refreshAuth();location.assign('/library');return}
   if(action==='name'){users=users.map(u=>u.id===person!.id?{...u,name:name.trim()}:u);if(person?.id===$auth?.id)await refreshAuth()}
   else if(action==='status')users=users.map(u=>u.id===person!.id?{...u,disabled:!person!.disabled}:u);
   else if(action==='create'){const list=await fetch(`/api/users?include_disabled=true&q=${encodeURIComponent(query)}`);if(!list.ok)throw new Error('Đã tạo tài khoản, nhưng chưa tải lại được danh sách.');users=await list.json()}
  }catch(e){const text=e instanceof Error?e.message:'Không lưu được tài khoản';if(mode!==null)formError=text;else error=text}finally{busy=false}
 }
</script>
<svelte:head><title>Quản lý tài khoản · Dharma</title></svelte:head>
<div class="library-shell"><div class="site-header"><Topbar back><a href="/library">← Thư viện</a></Topbar></div><main class="page-content">
 <div class="library-heading"><div><h1>Quản lý tài khoản</h1><p>Tạo tài khoản, quản lý quyền và trạng thái truy cập.</p></div>{#if $auth?.admin}<button class="primary-btn" onclick={()=>edit('create')}>Tạo tài khoản</button>{/if}</div>
 {#if $auth?.admin}
 <div class="users-toolbar"><label class="user-search">Tìm tài khoản<input type="search" bind:value={query} placeholder="Tìm tên hoặc email…"/></label><label>Trạng thái<Combobox label="Trạng thái" bind:value={status} options={[{value:"all",label:"Tất cả"},{value:"active",label:"Đang hoạt động"},{value:"disabled",label:"Đã vô hiệu hóa"}]}/></label></div>
 {#if message}<p class="users-feedback" role="status">{message}</p>{/if}
 {#if error}<p role="alert">{error}</p>{/if}
 {#if loading}<p role="status" class="library-count">Đang tìm tài khoản…</p>{:else}
 <p class="library-count">{visible.length} tài khoản{users.length===100?' · Hiển thị tối đa 100 kết quả. Nhập email để tìm chính xác hơn.':''}</p>
 <div class="users-list">{#each visible as user (user.id)}<article class="user-row" class:user-disabled={user.disabled}>
 <div class="user-identity"><strong>{user.name||user.email.split('@')[0]}</strong><p>{user.email}</p><div class="user-badges"><span class="user-badge" class:admin-badge={user.admin}>{user.admin?'Quản trị viên':'Người dùng'}</span><span class="user-badge" class:disabled-badge={user.disabled}>{user.disabled?'Đã vô hiệu hóa':'Đang hoạt động'}</span>{#if user.id===$auth?.id}<span class="user-badge">Bạn</span>{/if}</div></div>
 <div class="user-actions"><button class="outline-btn" disabled={busy} onclick={()=>edit('name',user)}>Sửa tên</button><button class="outline-btn" disabled={busy} onclick={()=>edit('password',user)}>Đặt lại mật khẩu</button>{#if user.id!==$auth?.id}<button class="outline-btn" class:danger-text={!user.disabled} disabled={busy} onclick={()=>edit('status',user)}>{user.disabled?'Kích hoạt lại':'Vô hiệu hóa'}</button>{:else}<span class="user-self-note">Không thể tự vô hiệu hóa</span>{/if}</div>
 </article>{:else}<div class="library-empty"><h2>Không có tài khoản phù hợp</h2><p>Thử email khác hoặc thay đổi bộ lọc trạng thái.</p></div>{/each}</div>
 {/if}
 {:else}<p role="status">{$auth?'Chỉ quản trị viên được quản lý tài khoản.':'Đăng nhập bằng tài khoản quản trị để tiếp tục.'}</p>{/if}
</main></div>
<Popup open={mode!==null} title={mode==='name'?'Sửa tên hiển thị':mode==='create'?'Tạo tài khoản':mode==='password'?'Đặt lại mật khẩu':target?.disabled?'Kích hoạt lại tài khoản?':'Vô hiệu hóa tài khoản?'} {busy} class="user-dialog" onrequestclose={close}>
 <form id={formId} class="upload-form" onsubmit={submit}>
 {#if mode==='create'||mode==='name'}<label>Tên hiển thị<input bind:value={name} required maxlength={100} autocomplete="name" disabled={busy}/></label>{/if}
 {#if mode==='create'}
 <label>Email<input type="email" bind:value={email} required maxlength={254} disabled={busy} autocomplete="off"/></label>
 <label>Vai trò<Combobox label="Vai trò" bind:value={admin} disabled={busy} options={[{value:false,label:"Người dùng"},{value:true,label:"Quản trị viên"}]}/></label>
 <p class="user-role-help">{admin?'Quản trị viên có thể tạo tài khoản, tạo admin khác, đặt lại mật khẩu và vô hiệu hóa người dùng.':'Người dùng có thể quản lý bài nghe của mình và truy cập nội dung được chia sẻ.'}</p>
 {/if}
 {#if mode!=='create'}<p class="user-target">{target?.email}</p>{/if}
 {#if mode==='create'||mode==='password'}<label>Mật khẩu mới<input type="password" bind:value={password} minlength={12} maxlength={72} required autocomplete="new-password" disabled={busy}/><small>Từ 12 đến 72 ký tự.</small></label>{/if}
 {#if mode==='password'}<p>Các phiên đăng nhập hiện tại sẽ kết thúc. {target?.id===$auth?.id?'Bạn cần đăng nhập lại sau khi đổi mật khẩu.':''}</p>{/if}
 {#if mode==='status'}<p>{target?.disabled?'Tài khoản sẽ có thể đăng nhập lại với mật khẩu hiện tại. Quyền và dữ liệu được giữ nguyên.':'Tài khoản sẽ bị đăng xuất và không thể đăng nhập cho đến khi được kích hoạt lại. Bài nghe và dữ liệu vẫn được giữ nguyên.'}</p>{#if target?.admin&&!target.disabled}<p class="user-role-help">Đây là tài khoản quản trị viên. Quyền quản trị sẽ tạm ngừng cho đến khi kích hoạt lại.</p>{/if}{/if}
 {#if formError}<p role="alert" class="danger-text">{formError}</p>{/if}

 </form>
 {#snippet footer()} <div class="user-dialog-actions"><button type="button" class="outline-btn" disabled={busy} onclick={close}>Hủy</button><button type="submit" form={formId} class="primary-btn" class:danger-btn={mode==='status'&&!target?.disabled} disabled={busy}>{busy?'Đang xử lý…':mode==='name'?'Lưu tên':mode==='create'?(admin?'Tạo quản trị viên':'Tạo tài khoản'):mode==='password'?'Đặt lại mật khẩu':target?.disabled?'Kích hoạt lại':'Vô hiệu hóa'}</button></div>{/snippet}
</Popup>
