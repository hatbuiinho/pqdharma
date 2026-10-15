<script lang="ts" generics="T extends string | number | boolean">
 import {onMount,tick} from 'svelte';
 type Option={value:T;label:string;disabled?:boolean};
 let {options,value=$bindable(),id,label,disabled=false,placeholder='Chọn…',onchange}: {options:Option[];value?:T;id?:string;label:string;disabled?:boolean;placeholder?:string;onchange?:(value:T)=>void}=$props();
 const uid=$props.id();
 let trigger:HTMLButtonElement,popup:HTMLSpanElement;
 let open=$state(false),active=$state(-1),left=$state(0),top=$state(0),width=$state(0),height=$state(280);
 let search='',searchedAt=0;
 const selected=$derived(options.find(o=>o.value===value));
 const listId=`${uid}-list`;
 function close(){popup?.hidePopover()}
 async function reveal(index=options.findIndex(o=>o.value===value)){
  if(disabled||!options.some(o=>!o.disabled))return;
  const rect=trigger.getBoundingClientRect(),margin=8;
  width=Math.min(Math.max(rect.width,180),window.innerWidth-24);
  left=Math.max(12,Math.min(rect.left,window.innerWidth-width-12));
  const below=window.innerHeight-rect.bottom-margin-12,above=rect.top-margin-12;
  const upwards=below<180&&above>below;
  height=Math.max(0,Math.min(280,upwards?above:below));
  top=upwards?rect.top-margin-height:rect.bottom+margin;
  active=index>=0&&!options[index]?.disabled?index:options.findIndex(o=>!o.disabled);
  await tick();popup.showPopover();open=true;
  if(upwards)top=rect.top-margin-popup.getBoundingClientRect().height;
  void scrollActive();
 }
 async function scrollActive(){await tick();document.getElementById(`${uid}-option-${active}`)?.scrollIntoView({block:'nearest'})}
 function move(direction:number){let next=active;for(let i=0;i<options.length;i++){next=(next+direction+options.length)%options.length;if(!options[next].disabled){active=next;void scrollActive();return}}}
 function choose(index:number){const option=options[index];if(!option||option.disabled)return;value=option.value;close();onchange?.(option.value)}
 function keydown(e:KeyboardEvent){
  if(e.key==='Tab'){close();return}
  if(e.key==='Escape'){if(open){e.preventDefault();e.stopPropagation();close()}return}
  if(['ArrowDown','ArrowUp','Home','End','Enter',' '].includes(e.key)){
   e.preventDefault();
   if(e.key==='Enter'||e.key===' '){if(open)choose(active);else void reveal();return}
   if(e.key==='Home'||e.key==='End'){const indices=options.map((o,i)=>o.disabled?-1:i).filter(i=>i>=0);const next=e.key==='Home'?indices[0]:indices.at(-1);if(!open)void reveal(next);else{active=next??-1;void scrollActive()}return}
   if(!open)void reveal();else move(e.key==='ArrowDown'?1:-1);return;
  }
  if(e.key.length===1&&!e.ctrlKey&&!e.metaKey&&!e.altKey){
   e.preventDefault();const now=Date.now();search=now-searchedAt>700?e.key:search+e.key;searchedAt=now;
   const normalized=search.toLocaleLowerCase('vi');const index=options.findIndex(o=>!o.disabled&&o.label.toLocaleLowerCase('vi').startsWith(normalized));
   if(index>=0){if(!open)void reveal(index);else{active=index;void scrollActive()}}
  }
 }
 $effect(()=>{if(disabled)close()});
 onMount(()=>{const dismiss=(e:Event)=>{if(!popup.contains(e.target as Node))close()};window.addEventListener('scroll',dismiss,true);window.addEventListener('resize',close);return()=>{window.removeEventListener('scroll',dismiss,true);window.removeEventListener('resize',close)}});
</script>
<span class="combobox">
 <button bind:this={trigger} id={id??uid} type="button" class="combobox-trigger" role="combobox" aria-label={label} aria-haspopup="listbox" aria-expanded={open} aria-controls={listId} aria-activedescendant={open&&active>=0?`${uid}-option-${active}`:undefined} {disabled} onclick={()=>open?close():void reveal()} onkeydown={keydown}>
  <span class="combobox-value">{selected?.label??placeholder}</span><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m6 9 6 6 6-6"/></svg>
 </button>
 <span bind:this={popup} id={listId} class="combobox-menu" popover="auto" role="listbox" aria-label={label} style:left={`${left}px`} style:top={`${top}px`} style:width={`${width}px`} style:max-height={`${height}px`} ontoggle={e=>{open=e.newState==='open';if(!open){search='';active=-1}}}>
  {#each options as option,index}<button id={`${uid}-option-${index}`} type="button" role="option" aria-selected={option.value===value} aria-disabled={option.disabled||undefined} disabled={option.disabled} tabindex="-1" class="combobox-option" class:combobox-highlighted={index===active} onpointerdown={e=>e.preventDefault()} onpointermove={()=>{if(!option.disabled)active=index}} onclick={e=>{e.preventDefault();e.stopPropagation();choose(index)}}><span>{option.label}</span>{#if option.value===value}<svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m5 12 4 4L19 6"/></svg>{/if}</button>{/each}
 </span>
</span>
