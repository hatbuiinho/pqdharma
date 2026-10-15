export async function shareRecordingLink(recordingId:string,title:string):Promise<string>{
 const url=new URL(`/recordings/${encodeURIComponent(recordingId)}`,location.origin).href;
 try{
  if(navigator.share){await navigator.share({title,url});return ''}
  await navigator.clipboard.writeText(url);return 'Đã sao chép link bài nghe.';
 }catch(error){
  if(error instanceof DOMException&&error.name==='AbortError')return '';
  return 'Chưa chia sẻ được. Bạn có thể mở bài và sao chép link trên thanh địa chỉ.';
 }
}
