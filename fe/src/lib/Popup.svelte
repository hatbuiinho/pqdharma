<script lang="ts">
 import {onMount,type Snippet} from 'svelte';
 let {open,title,subtitle='',busy=false,wide=false,class:className='',children,footer,onrequestclose,onclose,ondragover,ondrop}:{
  open:boolean;title:string;subtitle?:string;busy?:boolean;wide?:boolean;class?:string;
  children:Snippet;footer?:Snippet;onrequestclose:()=>void;onclose?:()=>void;
  ondragover?:(event:DragEvent)=>void;ondrop?:(event:DragEvent)=>void;
 }=$props();
 let element=$state<HTMLDialogElement>();
 const uid=$props.id();
 let viewport=$state<number>(),center=$state<number>();
 $effect(()=>{if(!element)return;if(open&&!element.open)element.showModal();else if(!open&&element.open)element.close()});
 function requestClose(){if(!busy)onrequestclose()}
 onMount(()=>{
  const update=()=>{const v=window.visualViewport;viewport=v?.height??window.innerHeight;center=(v?.offsetTop??0)+viewport/2};update();
  window.visualViewport?.addEventListener('resize',update);window.visualViewport?.addEventListener('scroll',update);window.addEventListener('resize',update);
  return()=>{window.visualViewport?.removeEventListener('resize',update);window.visualViewport?.removeEventListener('scroll',update);window.removeEventListener('resize',update)};
 });
</script>
<dialog bind:this={element} class={`app-popup ${className}`} class:popup-wide={wide} aria-labelledby={`${uid}-title`} aria-describedby={subtitle?`${uid}-subtitle`:undefined} aria-busy={busy} style:--popup-viewport={viewport?`${viewport}px`:undefined} style:--popup-center={center?`${center}px`:undefined} {ondragover} {ondrop} onclose={()=>onclose?.()} oncancel={e=>{e.preventDefault();requestClose()}}>
 <header class="popup-header"><div class="popup-heading"><h2 id={`${uid}-title`}>{title}</h2>{#if subtitle}<p id={`${uid}-subtitle`}>{subtitle}</p>{/if}</div><button type="button" class="icon-btn popup-close" aria-label={`Đóng ${title}`} disabled={busy} onclick={requestClose}><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" aria-hidden="true"><path d="m6 6 12 12M18 6 6 18"/></svg></button></header>
 <div class="popup-body">{@render children()}</div>
 {#if footer}<footer class="popup-footer">{@render footer()}</footer>{/if}
</dialog>
