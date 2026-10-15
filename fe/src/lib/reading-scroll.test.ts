import {test} from 'node:test';
import assert from 'node:assert/strict';
import {readingScrollDelta} from './reading-scroll';
const viewport={top:120,bottom:620};
test('reading lines scroll before the lower edge and stay stable inside the reading band',()=>{
 assert.equal(readingScrollDelta({top:340,bottom:390},viewport),0);
 assert.equal(readingScrollDelta({top:410,bottom:460},viewport),90);
 assert.equal(readingScrollDelta({top:320,bottom:370},viewport),0);
});
test('a backwards loop or seek brings words below the sticky header',()=>{
 assert.equal(readingScrollDelta({top:50,bottom:100},viewport),-270);
 assert.equal(readingScrollDelta({top:340,bottom:390},viewport,true),20);
});
test('a viewport obscured by controls does not trigger scrolling',()=>{
 assert.equal(readingScrollDelta({top:90,bottom:130},{top:120,bottom:160}),0);
});
