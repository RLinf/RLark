import assert from "node:assert/strict";
import test from "node:test";
import { readCameraFrames, sameCameraFrame } from "../dist/test/astribot/lib/cameraStream.js";
import { cameraStreamPath, formatCameraMilliseconds, readCameraAge } from "../dist/test/astribot/lib/cameras.js";

import { cameraClockSample, cameraDisplayLatency } from "../dist/test/astribot/lib/cameraLatency.js";

const mime = "multipart/x-mixed-replace; boundary=s1ucamera";
const part = (body, captureHeader = "") => Buffer.concat([
  Buffer.from(`--s1ucamera\r\nContent-Type: image/jpeg\r\nContent-Length: ${body.length}\r\nX-Age-Seconds: 0.18\r\n${captureHeader}\r\n`),
  body,
  Buffer.from("\r\n"),
]);

function response(bytes) {
  let offset = 0;
  return new Response(new ReadableStream({
    pull(controller) {
      if (offset >= bytes.length) controller.close();
      else {
        controller.enqueue(bytes.subarray(offset, offset + 7));
        offset += 7;
      }
    },
  }), { headers: { "Content-Type": mime } });
}

test("split multipart parts preserve JPEG bytes and repeated frames compare equal", async () => {
  const first = Buffer.from("jpeg\r\n--s1ucamera\r\n");
  const second = Buffer.from("new jpeg");
  const frames = [];
  for await (const frame of readCameraFrames(response(Buffer.concat([part(first, "X-Camera-Capture-Ms: 1795000000000.123\r\n"), part(first), part(second)])))) frames.push(frame);
  assert.deepEqual(frames.map((frame) => Buffer.from(frame.jpeg)), [first, first, second]);
  assert.deepEqual(frames.map((frame) => frame.capturedAtMs), [1795000000000.123, undefined, undefined]);
  // The service's reported age travels with each part so the card can show it without a second request.
  assert.deepEqual(frames.map((frame) => frame.ageSeconds), [0.18, 0.18, 0.18]);
  assert.equal(sameCameraFrame(undefined, frames[0].jpeg), false);
  assert.equal(sameCameraFrame(frames[0].jpeg, frames[1].jpeg), true);
  assert.equal(sameCameraFrame(frames[1].jpeg, frames[2].jpeg), false);
});

test("truncated streams and invalid part sizes fail so the preview can retry", async () => {
  const corrupt = Buffer.from("--s1ucamera\r\nContent-Type: image/jpeg\r\nContent-Length: 999999999\r\n\r\n");
  await assert.rejects(readCameraFrames(response(corrupt)).next(), /Invalid camera frame headers/);
  await assert.rejects(readCameraFrames(response(part(Buffer.from("jpeg")).subarray(0, -4))).next(), /Incomplete camera frame/);
  await assert.rejects(readCameraFrames(new Response("not a camera stream")).next(), /Invalid camera stream/);
});

test("camera telemetry keeps missing age distinct from zero, and routes the chosen rate", () => {
  assert.equal(readCameraAge(null), undefined);
  assert.equal(readCameraAge(" "), undefined);
  assert.equal(readCameraAge("-1"), undefined);
  assert.equal(readCameraAge("0"), 0);
  assert.equal(readCameraAge("0.18"), 0.18);
  assert.equal(formatCameraMilliseconds(undefined), "—");
  assert.equal(formatCameraMilliseconds(180), "180 ms");
  assert.equal(cameraStreamPath("s1u-01", "rlark-robot", "head", 15), "/api/v1/rlinf.io/v1alpha1/nodes/s1u-01/cameras/head/stream?namespace=rlark-robot&fps=15");
});


test("latency maps browser paint to the sampled camera clock and rejects unusable timing", () => {
  const clock = cameraClockSample(100, 140, { received_ms: 100010, sent_ms: 100020 });
  assert.deepEqual(clock, { offsetMs: 99895, uncertaintyMs: 15, sampledAt: 140 });
  assert.deepEqual(cameraDisplayLatency(99995, 175, clock), { elapsedMs: 75, uncertaintyMs: 15, displayAt: 175 });
  assert.equal(cameraDisplayLatency(undefined, 175, clock), undefined);
  assert.equal(cameraDisplayLatency(100200, 175, clock), undefined);
  assert.equal(cameraDisplayLatency(99995, 31000, clock), undefined);
  assert.equal(cameraClockSample(100, 500, { received_ms: 100010, sent_ms: 100020 }), undefined);
});
