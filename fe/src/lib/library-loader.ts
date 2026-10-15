// A new account/scope cancels the previous request and can load immediately.
// Ignore stale responses even when an intermediary doesn't honor cancellation.
export function createLibraryLoader<T>(callbacks: {
 fetch: (scope: string, signal: AbortSignal) => Promise<T>;
 data: (data: T) => void;
 error: () => void;
 settled: () => void;
}) {
 let scope: string | undefined;
 let current: AbortController | undefined;
 let disposed = false;
 async function refresh() {
  if (disposed || scope === undefined || current) return;
  const request = new AbortController();
  current = request;
  try {
   const data = await callbacks.fetch(scope, request.signal);
   if (!disposed && current === request) callbacks.data(data);
  } catch {
   if (!disposed && current === request && !request.signal.aborted) callbacks.error();
  } finally {
   if (!disposed && current === request) { current = undefined; callbacks.settled(); }
  }
 }
 return {
  refresh,
  reset(nextScope: string) { current?.abort(); current = undefined; scope = nextScope; return refresh(); },
  dispose() { disposed = true; current?.abort(); current = undefined; }
 };
}
