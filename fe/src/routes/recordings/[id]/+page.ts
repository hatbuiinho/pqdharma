import type { PageLoad } from './$types';
export const load: PageLoad = ({ params }) => ({ recordingId: params.id });
