export type ReadingBounds = {top:number;bottom:number};

// Keep the current line above the lower reading edge with space for upcoming lines.
export function readingScrollDelta(line:ReadingBounds,viewport:ReadingBounds,force=false):number {
 const height=viewport.bottom-viewport.top;
 if(height<80)return 0;
 if(!force&&line.top>=viewport.top+12&&line.bottom<=viewport.top+height*.6)return 0;
 const delta=line.top-(viewport.top+height*.4);
 return Math.abs(delta)<8?0:delta;
}
