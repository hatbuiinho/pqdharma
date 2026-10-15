<script lang="ts">
  import Icon from "$lib/Icon.svelte";
  import Library from "$lib/Library.svelte";
</script>

<svelte:head>
  <title>Thư viện kinh · Dharma</title>
  <meta
    name="description"
    content="Thư viện bài kinh tiếng Anh. Chọn bài để nghe, luyện tụng hoặc nhập MP3 mới."
  />
</svelte:head>
<div class="library-shell">
  <div class="site-header">
    <header class="topbar">
      <a href="/library" class="brand"
        ><img class="brand-logo" src="/icons/icon-192.png" width="37" height="37" alt=""/><span
          >PQ Dharma</span
        ><span class="brand-sub">NGHE & TỤNG</span></a
      >
      <span class="library-nav"><Icon name="library" size={18} />Thư viện</span>
    </header>
  </div>
  <main class="page-content"><Library /></main>
</div>
