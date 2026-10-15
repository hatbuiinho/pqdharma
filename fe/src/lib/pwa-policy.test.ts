import test from 'node:test';
import assert from 'node:assert/strict';
import {cachePolicy} from './pwa-policy';
const origin='https://dharma.test';
const policy=(path:string,options:Partial<Parameters<typeof cachePolicy>[0]>={})=>cachePolicy({url:new URL(path,origin).href,method:'GET',...options},origin);
test('app-shell fallback covers deep links and the legacy recording query only for navigations',()=>{
 for(const path of ['/','/?recording=sample','/library','/recordings/new-id','/recordings/new-id/'])assert.equal(policy(path,{mode:'navigate'}),'navigation');
 assert.equal(policy('/recordings/new-id'),'network');
 assert.equal(policy('/not-an-app-route',{mode:'navigate'}),'network');
 assert.equal(policy('/api/sutras',{mode:'navigate'}),'data');
});
test('only stable JSON snapshots are cached; processing, mutations and media stay live',()=>{
 assert.equal(policy('/api/sutras'),'data');
 assert.equal(policy('/api/recordings/sample/transcript'),'data');
 for(const path of ['/api/recordings/sample/job','/api/recordings/sample/audio','/api/recordings/sample/audio?v=1','/api/health'])assert.equal(policy(path),'network');
 for(const method of ['POST','PATCH','DELETE'])assert.equal(policy('/api/recordings/sample/transcript',{method}),'network');
 assert.equal(policy('/api/recordings/sample/transcript',{hasRange:true}),'network');
 assert.equal(policy('/icons/icon-512.png'),'asset');
 assert.equal(policy('https://another.test/api/sutras'),'network');
});
