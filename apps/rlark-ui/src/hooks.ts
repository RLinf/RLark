import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type SetStateAction,
} from "react";
import { isMockMode } from "./config";

export function usePersistentState<T>(
  key: string,
  fallback: T,
): [T, Dispatch<SetStateAction<T>>] {
  const [value, setValue] = useState<T>(() => {
    try {
      const stored = localStorage.getItem(key);
      return stored === null ? fallback : (JSON.parse(stored) as T);
    } catch {
      return fallback;
    }
  });

  useEffect(() => {
    try {
      localStorage.setItem(key, JSON.stringify(value));
    } catch {
      // Storage can be unavailable in privacy-restricted browser contexts.
    }
  }, [key, value]);

  return [value, setValue];
}

export function useBackendMode(): { isMockMode: boolean; checking: boolean } {
  return { isMockMode, checking: false };
}

export function useAutoRefresh(
  fetcher: (isInitial: boolean) => Promise<void>,
  interval = 10000,
  deps: unknown[] = [],
): { refresh: () => void } {
  const fetcherRef = useRef(fetcher);
  const timerRef = useRef<number | undefined>(undefined);
  const mountedRef = useRef(true);
  const firstRef = useRef(true);

  fetcherRef.current = fetcher;

  const refresh = useCallback(async () => {
    try {
      await fetcherRef.current(firstRef.current);
      if (mountedRef.current) firstRef.current = false;
    } catch (e) {
      console.warn("auto refresh failed:", e);
    }
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    firstRef.current = true;

    const start = () => {
      refresh();
      if (timerRef.current) window.clearInterval(timerRef.current);
      timerRef.current = window.setInterval(refresh, interval);
    };

    const stop = () => {
      if (timerRef.current) {
        window.clearInterval(timerRef.current);
        timerRef.current = undefined;
      }
    };

    const onVisibility = () => {
      if (document.hidden) {
        stop();
      } else {
        start();
      }
    };

    start();
    document.addEventListener("visibilitychange", onVisibility);

    return () => {
      mountedRef.current = false;
      stop();
      document.removeEventListener("visibilitychange", onVisibility);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [interval, ...deps]);

  return { refresh };
}
