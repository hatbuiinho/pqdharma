import { redirect } from '@sveltejs/kit';
import type { PageLoad } from './$types';

export const load: PageLoad = ({ url }) => {
 const recording = url.searchParams.get('recording');
 redirect(303, recording ? `/recordings/${encodeURIComponent(recording)}` : '/library');
};
