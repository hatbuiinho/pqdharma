<script lang="ts">
  import Iconify from '@iconify/svelte';
  import icons from './icons.json';
  let { name, size = 20, class: className = '' }: {
    name: keyof typeof icons;
    size?: number;
    class?: string;
  } = $props();
</script>
<Iconify icon={icons[name]} width={size} height={size} class={`shrink-0 ${className}`} aria-hidden="true" />
