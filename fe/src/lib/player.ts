export type IpaVoice = 'en-gb' | 'en-us';
export type Phonetic = {type:string;text:string;source?:string;readable?:string};
export type Token = { id: string; text: string; startMs: number; endMs: number; endEstimated: boolean; phonetic?: Phonetic;phonetics?: Partial<Record<IpaVoice,Phonetic>> };
export type Segment = {id:string; startMs:number;endMs:number;text:string;tokens:Token[]};
export type Transcript = {defaultIpaVoice?:IpaVoice;ipaVoices?:IpaVoice[];ipaEditable?:boolean;revision?:number;title:string;language:string;sourceUrl:string;status:string;segments:Segment[]};
export function activeIndex<T extends {startMs:number;endMs:number}>(items:T[],ms:number,holdUntilNext=false):number {
 let lo=0,hi=items.length-1,result=-1;
 while(lo<=hi){const mid=(lo+hi)>>>1;if(items[mid].startMs<=ms){result=mid;lo=mid+1}else hi=mid-1}
 // Derive from playback time so backwards seeks and loops reset immediately.
 return result>=0 && (holdUntilNext || ms<items[result].endMs) ? result : -1;
}
export function selectedRange(tokens:Token[],a:number,b:number){const lo=Math.min(a,b),hi=Math.max(a,b);return {startMs:tokens[lo].startMs,endMs:tokens[hi].endMs}}
export function formatTime(seconds:number){if(!Number.isFinite(seconds))return '0:00';return `${Math.floor(seconds/60)}:${String(Math.floor(seconds%60)).padStart(2,'0')}`}
// Keep both transport timestamps in the same format for the recording.
export function formatPlayerTime(seconds:number,duration:number){
 const total=Math.floor(Number.isFinite(seconds)?Math.max(0,seconds):0);
 const pad=(value:number)=>String(value).padStart(2,'0');
 return duration>=3600||total>=3600
  ? `${pad(Math.floor(total/3600))}:${pad(Math.floor(total/60)%60)}:${pad(total%60)}`
  : `${pad(Math.floor(total/60))}:${pad(total%60)}`;
}
export function loopBoundary(ms:number,endMs:number,completed:number,repeats:number):'continue'|'repeat'|'finish'{
 if(ms<endMs)return 'continue';
 return repeats>0&&completed+1>=repeats?'finish':'repeat';
}
export type TimeRange={startMs:number;endMs:number};
export function adjustRange(range:TimeRange,edge:'startMs'|'endMs',delta:number,durationMs:number):TimeRange {
 const updated={...range};
 updated[edge]=edge==='startMs'?Math.max(0,Math.min(range.endMs-200,range.startMs+delta)):Math.min(durationMs,Math.max(range.startMs+200,range.endMs+delta));
 return updated;
}
export function formatPrecise(ms:number){return `${formatTime(ms/1000)}.${Math.floor(ms%1000/100)}`}
// Migrate the early demo's 2-second rest to the agreed 0.5-second default.
export function practicePreferences(value: { settingsVersion?: number; gap?: number; repeats?: number } | null) {
 return {
  gap: value?.settingsVersion === 2 && [0,.5,1,2,4,6].includes(value.gap ?? -1) ? value.gap! : .5,
  repeats: value?.settingsVersion === 2 && [0,3,5,10].includes(value.repeats ?? -1) ? value.repeats! : 0
 };
}
