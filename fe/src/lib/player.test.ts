import {test} from 'node:test';
import assert from 'node:assert/strict';
import {activeIndex, selectedRange, loopBoundary, adjustRange, formatPrecise, practicePreferences} from './player';
const words=[{id:'a',text:'Thus',startMs:7040,endMs:7360,endEstimated:true},{id:'b',text:'have',startMs:7360,endMs:7600,endEstimated:true},{id:'c',text:'I',startMs:8000,endMs:8200,endEstimated:true}];
test('highlight respects boundaries, silence and backwards seeks',()=>{assert.equal(activeIndex(words,7039),-1);assert.equal(activeIndex(words,7040),0);assert.equal(activeIndex(words,7360),1);assert.equal(activeIndex(words,7800),-1);assert.equal(activeIndex(words,8100),2);assert.equal(activeIndex(words,7100),0);assert.equal(activeIndex(words,8200),-1)});
test('reverse selection produces an inclusive token range',()=>{assert.deepEqual(selectedRange(words,2,0),{startMs:7040,endMs:8200})});

test('finite loops finish on the requested count; unlimited loops repeat',()=>{assert.equal(loopBoundary(99,100,0,3),'continue');assert.equal(loopBoundary(100,100,1,3),'repeat');assert.equal(loopBoundary(100,100,2,3),'finish');assert.equal(loopBoundary(110,100,200,0),'repeat')});

test('range editing clamps to audio bounds and keeps markers apart',()=>{
 const range={startMs:1000,endMs:2000};
 assert.deepEqual(adjustRange(range,'startMs',-5000,3000),{startMs:0,endMs:2000});
 assert.deepEqual(adjustRange(range,'startMs',5000,3000),{startMs:1800,endMs:2000});
 assert.deepEqual(adjustRange(range,'endMs',-5000,3000),{startMs:1000,endMs:1200});
 assert.deepEqual(adjustRange(range,'endMs',5000,3000),{startMs:1000,endMs:3000});
 assert.deepEqual(range,{startMs:1000,endMs:2000});
 assert.equal(formatPrecise(7040),'0:07.0');
});

test('practice settings migrate to continuous loops with half-second rests',()=>{
 assert.deepEqual(practicePreferences(null),{gap:.5,repeats:0});
 assert.deepEqual(practicePreferences({gap:2,repeats:3}),{gap:.5,repeats:0});
 assert.deepEqual(practicePreferences({settingsVersion:2,gap:1,repeats:5}),{gap:1,repeats:5});
 assert.deepEqual(practicePreferences({settingsVersion:2,gap:-1,repeats:99}),{gap:.5,repeats:0});
});

test('held highlight changes at the next start and follows seeks and loops',()=>{
 assert.equal(activeIndex([],1000,true),-1);
 for(const [ms,index] of [[7039,-1],[7040,0],[7360,1],[7800,1],[7999,1],[8000,2],[9000,2],[7800,1],[7040,0],[0,-1]]){
  assert.equal(activeIndex(words,ms,true),index,`time ${ms}`);
 }
});
