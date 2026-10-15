<script lang="ts">
 import RecordingPlayer from '$lib/RecordingPlayer.svelte';
 import type { PageData } from './$types';
 let { data }: {data:PageData} = $props();
</script>
{#key data.recordingId}
 <RecordingPlayer recordingId={data.recordingId}/>
{/key}
