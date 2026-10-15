<script lang="ts">
 import type { Snippet } from 'svelte';
 let { children }: { children?: Snippet } = $props();
</script>

<header class="topbar">
 <a href="/library" class="brand">
  <img class="brand-logo" src="/icons/icon-192.png" width="37" height="37" alt=""/>
  <span>PQ Dharma</span><span class="brand-sub">NGHE & TỤNG</span>
 </a>
 {@render children?.()}
</header>
