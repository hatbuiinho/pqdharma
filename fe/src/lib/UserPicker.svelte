<script lang="ts">
 import {onMount,tick} from 'svelte';
 import {auth} from './auth';
 type Person={id:string;email:string;name?:string};
 let {emails=$bindable([]),disabled=false,showSelected=true,onadd}:{emails?:string[];disabled?:boolean;showSelected?:boolean;onadd?:(person:Person)=>void}=$props();
 const uid=$props.id();
 let input:HTMLInputElement,popup:HTMLDivElement,root:HTMLDivElement;
 let query=$state(''),people=$state<Person[]>([]),loading=$state(false),error=$state(''),open=$state(false),active=$state(-1);
 let left=$state(0),top=$state(0),width=$state(0),height=$state(240);
 const matches=$derived(people.filter(p=>p.id!==$auth?.id&&!emails.includes(p.email)));
 const blocked=$derived(disabled||emails.length>=100);
 function close(){if(popup?.isConnected)popup.hidePopover();open=false;active=-1}
 async function position(){
  await tick();if(!open||!input||!popup)return;
  const rect=input.getBoundingClientRect(),below=window.innerHeight-rect.bottom-16,above=rect.top-16;
  width=Math.min(rect.width,window.innerWidth-24);left=Math.max(12,Math.min(rect.left,window.innerWidth-width-12));
  const upwards=below<160&&above>below;height=Math.max(0,Math.min(240,upwards?above:below));
  top=rect.bottom+6;
  await tick();if(open&&upwards)top=rect.top-6-popup.getBoundingClientRect().height;
 }
 async function reveal(){if(blocked)return;open=true;await position();if(open){popup.showPopover();await position()}}
 $effect(()=>{
  const q=query,shown=open;if(!shown)return;
  const controller=new AbortController();loading=true;error='';active=-1;people=[];
  const timer=setTimeout(async()=>{
   try{const r=await fetch(`/api/users?q=${encodeURIComponent(q)}`,{signal:controller.signal});if(!r.ok)throw new Error(await r.text());people=await r.json()}
   catch(e){if(!controller.signal.aborted)error=e instanceof Error?e.message:'Không tải được tài khoản.'}
   finally{if(!controller.signal.aborted){loading=false;void position()}}
  },250);
  return()=>{clearTimeout(timer);controller.abort()};
 });
 $effect(()=>{if(blocked)close()});
 function choose(index:number){const person=matches[index];if(!person||blocked)return;emails=[...emails,person.email];onadd?.(person);query='';close()}
 async function scrollActive(){await tick();document.getElementById(`${uid}-option-${active}`)?.scrollIntoView({block:'nearest'})}
 function keydown(e:KeyboardEvent){
  if(e.key==='Escape'&&open){e.preventDefault();e.stopPropagation();close();return}
  if(e.key==='Tab'){close();return}
  if(e.key==='ArrowDown'||e.key==='ArrowUp'){e.preventDefault();if(!open){void reveal();return}if(!matches.length)return;active=active<0?(e.key==='ArrowDown'?0:matches.length-1):(active+(e.key==='ArrowDown'?1:-1)+matches.length)%matches.length;void scrollActive()}
  if(e.key==='Enter'&&open){e.preventDefault();if(!loading)choose(active)}
 }
 onMount(()=>{
  const outside=(e:PointerEvent)=>{if(!root.contains(e.target as Node)&&!popup.contains(e.target as Node))close()};
  const scroll=(e:Event)=>{if(!popup.contains(e.target as Node))close()};
  document.addEventListener('pointerdown',outside);window.addEventListener('scroll',scroll,true);window.addEventListener('resize',close);
  return()=>{document.removeEventListener('pointerdown',outside);window.removeEventListener('scroll',scroll,true);window.removeEventListener('resize',close);close()};
 });
</script>
<div class="user-picker" bind:this={root}>
 <label for={`${uid}-input`}>Tìm người được chia sẻ</label>
 <input bind:this={input} id={`${uid}-input`} type="text" role="combobox" autocomplete="off" aria-autocomplete="list" aria-haspopup="listbox" aria-controls={`${uid}-suggestions`} aria-expanded={open} aria-activedescendant={open&&active>=0?`${uid}-option-${active}`:undefined} bind:value={query} disabled={blocked} placeholder="Tìm tên hoặc email…" onfocus={()=>void reveal()} onclick={()=>{if(!open)void reveal()}} oninput={()=>{if(!open)void reveal()}} onkeydown={keydown} onblur={()=>close()}/>
 <div bind:this={popup} id={`${uid}-suggestions`} class="combobox-menu user-suggestions" popover="manual" role="listbox" aria-label="Tài khoản gợi ý" style:left={`${left}px`} style:top={`${top}px`} style:width={`${width}px`} style:max-height={`${height}px`}>
 {#if loading}<p role="status">Đang tìm tài khoản…</p>{:else if error}<p role="status" class="danger-text">{error}</p>{:else}
 {#each matches as person,index (person.id)}<button id={`${uid}-option-${index}`} type="button" role="option" aria-selected={active===index} tabindex="-1" class="combobox-option" class:combobox-highlighted={active===index} onpointerdown={e=>e.preventDefault()} onpointermove={()=>active=index} onclick={()=>choose(index)}><span class="suggestion-person">{#if person.name}<strong>{person.name}</strong>{/if}<span>{person.email}</span></span><span class="suggestion-add">Thêm</span></button>{:else}<p role="status">Không có tài khoản phù hợp.</p>{/each}
 {/if}
 </div>
 {#if showSelected}<div class="selected-users">{#each emails as email}<button type="button" class="chip chip-active" disabled={disabled} onclick={()=>emails=emails.filter(value=>value!==email)} aria-label={`Bỏ chia sẻ với ${email}`}>{email} ×</button>{/each}</div>{/if}
 {#if emails.length>=100}<p role="status">Đã chọn tối đa 100 tài khoản.</p>{/if}
</div>
