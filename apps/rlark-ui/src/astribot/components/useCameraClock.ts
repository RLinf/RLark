import { useEffect, useRef } from "react";
import { requestJson } from "../../api";
import { camerasPath, type CamerasReport } from "../lib/cameras";
import { cameraClockSample, type CameraClock } from "../lib/cameraLatency";

const clockCache = new Map<string, { sample: CameraClock; expiresAt: number }>();

export function useCameraClock(node: string, namespace: string, enabled = true, options: { cacheKey?: string } = {}) {
  const sample = useRef<CameraClock | undefined>(undefined);
  const cacheKey = options.cacheKey ?? `${namespace}/${node}`;
  useEffect(() => {
    if (!enabled) return;
    const cached = clockCache.get(cacheKey);
    if (cached && cached.expiresAt > Date.now()) {
      sample.current = cached.sample;
      return;
    }
    let active: AbortController | undefined;
    let retry = 0;
    const calibrate = async () => {
      if (document.hidden || active) return;
      const controller = new AbortController();
      active = controller;
      let best: CameraClock | undefined;
      try {
        for (let i = 0; i < 3 && !controller.signal.aborted; i++) {
          const sent = performance.now();
          const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(2500)]);
          const report = await requestJson<CamerasReport>(camerasPath(node, namespace), { signal });
          if (!report.clock) break;
          const candidate = cameraClockSample(sent, performance.now(), report.clock);
          if (candidate && (!best || candidate.uncertaintyMs < best.uncertaintyMs)) best = candidate;
        }
      } catch {
        // A completed sample from this batch can still be used.
      } finally {
        if (active === controller) {
          sample.current = best;
          if (best) clockCache.set(cacheKey, { sample: best, expiresAt: Date.now() + 60000 });
          active = undefined;
          if (!document.hidden) retry = window.setTimeout(() => void calibrate(), 15000);
        }
      }
    };
    const stop = () => {
      active?.abort();
      active = undefined;
      sample.current = undefined;
      window.clearTimeout(retry);
    };
    const visibility = () => { if (document.hidden) stop(); else void calibrate(); };
    document.addEventListener("visibilitychange", visibility);
    void calibrate();
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      stop();
    };
  }, [node, namespace, enabled, cacheKey]);
  return sample;
}
