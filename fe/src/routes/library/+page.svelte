<script lang="ts">
  import Icon from "$lib/Icon.svelte";
  import Library from "$lib/Library.svelte";
  import Topbar from "$lib/Topbar.svelte";
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
    <Topbar>
      <span class="library-nav"><Icon name="library" size={18} />Thư viện</span>
    </Topbar>
  </div>
  <main class="page-content"><Library /></main>
</div>
