<script lang="ts">
 import Profile from './Profile.svelte';
 import type { Snippet } from 'svelte';
 let { children, back = false }: { children?: Snippet; back?: boolean } = $props();
</script>

<header class="topbar">
 <div class="topbar-identity">
 {#if back}<a href="/library" class="topbar-back" aria-label="Quay lại thư viện"><svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m12 19-7-7 7-7M5 12h14"/></svg></a>{/if}
 <a href="/library" class="brand" aria-label="PQ Dharma · Thư viện">
  <img class="brand-logo" src="/icons/icon-192.png" width="37" height="37" alt=""/>
  <span>PQ Dharma</span><span class="brand-sub">NGHE & TỤNG</span>
 </a>
 </div>
 <div class="topbar-actions">{#if children}<div class="topbar-context">{@render children()}</div>{/if}<Profile/></div>
</header>
