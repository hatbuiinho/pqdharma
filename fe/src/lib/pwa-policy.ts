export type CachePolicy = 'navigation' | 'data' | 'asset' | 'network';

// Keep processing state, uploads, mutations and large/ranged media on the network.
export function cachePolicy(request: {url:string;method:string;mode?:string;hasRange?:boolean}, origin:string):CachePolicy {
 const url=new URL(request.url);
 if(url.origin!==origin||request.method!=='GET'||request.hasRange)return 'network';
 const path=url.pathname;
 if(request.mode==='navigate'&&(path==='/'||path==='/library'||/^\/recordings\/[^/]+\/?$/.test(path)))return 'navigation';
 if(path==='/api/sutras'||/^\/api\/recordings\/[^/]+\/transcript$/.test(path))return 'data';
 if(path.startsWith('/_app/immutable/')||path.startsWith('/icons/')||path==='/manifest.webmanifest')return 'asset';
 return 'network';
}
