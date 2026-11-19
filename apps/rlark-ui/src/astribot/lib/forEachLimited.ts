// Calls work on each item, at most limit at a time, and starts no more once signal aborts.
export async function forEachLimited<T>(items: T[], limit: number, signal: AbortSignal, work: (item: T) => Promise<void>) {
  let next = 0;
  const worker = async () => {
    while (!signal.aborted && next < items.length) await work(items[next++]);
  };
  await Promise.all(Array.from({ length: Math.min(limit, items.length) }, worker));
}
