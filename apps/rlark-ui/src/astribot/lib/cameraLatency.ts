export interface CameraClock {
  offsetMs: number;
  uncertaintyMs: number;
  sampledAt: number;
}

type CameraClockReply = { received_ms: number; sent_ms: number };

// Translate the robot's wall clock to the browser's monotonic clock. Network
// delay can be asymmetric, so preserve the feasible offset interval.
export function cameraClockSample(browserSent: number, browserReceived: number, clock?: CameraClockReply): CameraClock | undefined {
  if (!clock || !Number.isFinite(clock.received_ms) || !Number.isFinite(clock.sent_ms)
    || clock.received_ms <= 0 || clock.sent_ms < clock.received_ms) return undefined;
  const lower = clock.sent_ms - browserReceived;
  const upper = clock.received_ms - browserSent;
  const uncertaintyMs = (upper - lower) / 2;
  if (uncertaintyMs < 0 || uncertaintyMs > 100) return undefined;
  return { offsetMs: lower + uncertaintyMs, uncertaintyMs, sampledAt: browserReceived };
}

export function cameraDisplayLatency(capturedAtMs: number | undefined, displayAt: number, clock?: CameraClock) {
  if (capturedAtMs === undefined || !clock || displayAt - clock.sampledAt > 30000) return undefined;
  const elapsedMs = displayAt + clock.offsetMs - capturedAtMs;
  if (!Number.isFinite(elapsedMs) || elapsedMs < 0 || elapsedMs > 15000) return undefined;
  return { elapsedMs, uncertaintyMs: clock.uncertaintyMs, displayAt };
}
