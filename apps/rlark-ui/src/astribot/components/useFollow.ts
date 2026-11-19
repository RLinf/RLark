import { useEffect } from "react";

// Follows an operation while key names one: reads it at once and every 3 s until ended says it is over or tries
// run out, handing each read to onRead and a failed read's message to onError. A new key starts over.
export function useFollow<T>(
  key: string | undefined,
  read: (signal: AbortSignal) => Promise<T>,
  ended: (value: T) => boolean,
  onRead: (value: T) => void,
  onError: (message: string) => void,
  tries: number,
) {
  useEffect(() => {
    if (!key) return;
    const controller = new AbortController();
    let timer = 0;
    let left = tries;
    const poll = async () => {
      try {
        const value = await read(controller.signal);
        onRead(value);
        if (ended(value)) return;
      } catch (reason) {
        if (!controller.signal.aborted) onError((reason as Error).message);
      }
      if (--left > 0 && !controller.signal.aborted) timer = window.setTimeout(poll, 3000);
    };
    void poll();
    return () => {
      controller.abort();
      window.clearTimeout(timer);
    };
    // The key names what is read; the callbacks are the caller's of that moment.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key]);
}
