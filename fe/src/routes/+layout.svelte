<script lang="ts">
 import '../app.css';
 import { onMount } from 'svelte';
 import PwaControls from '$lib/PwaControls.svelte';
 let { children } = $props();
 onMount(()=>{document.getElementById('app-loading')?.remove()});
</script>
<PwaControls/>
{@render children()}
