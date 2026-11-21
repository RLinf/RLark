import assert from "node:assert/strict";
import test from "node:test";
import {
  CAMERA_VIEWS,
  cameraAge,
  cameraCells,
  cameraFramePath,
  camerasPath,
  publishedCameras,
} from "../dist/test/astribot/lib/cameras.js";

const camera = (fields) => ({
  state: "ok", fps: 30, age_seconds: 0.03, step: 0, attempts: 0, next_attempt_at: null, last_action: null, last_result: null, ...fields,
});

test("the wall is head, torso / wrist_left, wrist_right, and a body's missing views say so", () => {
  assert.deepEqual(CAMERA_VIEWS, ["head", "torso", "wrist_left", "wrist_right"]);
  // s1u_observer: head and torso only.
  const cells = cameraCells({ state: "ok", updated_at: 1795000000, cameras: { head: camera(), torso: camera({ age_seconds: 12.4 }) } }, true);
  assert.deepEqual(cells.map((cell) => [cell.view, cell.name, cell.present, cell.badge, cell.age, cell.frames]), [
    ["head", "头部", true, "正常", "0.0 秒前", true],
    ["torso", "躯干", true, "正常", "12 秒前", true],
    ["wrist_left", "左腕", false, "该本体没有这路相机", "—", false],
    ["wrist_right", "右腕", false, "该本体没有这路相机", "—", false],
  ]);
  // Before the robot's first answer nothing is said to be missing.
  assert.ok(cameraCells(undefined, true).every((cell) => cell.present === undefined && cell.badge === "等待相机状态" && !cell.frames));
});

test("each camera's badge follows its state and the step its recovery is at", () => {
  const badge = (fields) => {
    const [cell] = cameraCells({ state: "recovering", updated_at: 0, cameras: { head: camera(fields) } }, true);
    return [cell.badge, cell.tone, cell.frames];
  };
  assert.deepEqual(badge({}), ["正常", "online", true]);
  assert.deepEqual(badge({ state: "slow", fps: 5 }), ["帧率低", "pending", true]);
  assert.deepEqual(badge({ state: "stale", age_seconds: 7 }), ["无画面", "failed", true]);
  assert.deepEqual(badge({ state: "absent", age_seconds: null }), ["无画面", "failed", false]);
  assert.deepEqual(badge({ state: "stale", step: 1, attempts: 2 }), ["恢复中 第 1 步", "pending", true]);
  assert.deepEqual(badge({ state: "absent", step: 2 }), ["恢复中 第 2 步", "pending", false]);
  assert.deepEqual(badge({ state: "stale", step: 3 }), ["需要断电或 USB 复位", "failed", true]);
  // A slow camera's driver restart (step 1) is not a recovery from no frames.
  assert.deepEqual(badge({ state: "slow", step: 1 }), ["帧率低", "pending", true]);
  assert.equal(cameraCells({ state: "ok", updated_at: 0, cameras: { head: camera({ state: "stale", step: 3 }) } }, false)[0].badge,
    "Needs a power cycle or USB reset");
});

test("ages and the Gateway's paths", () => {
  assert.equal(cameraAge(null, true), "—");
  assert.equal(cameraAge(0.123, false), "0.1 s ago");
  assert.equal(cameraAge(95.6, true), "96 秒前");
  assert.equal(camerasPath("s1u-03-x86", "rlark-robot"), "/api/v1/rlinf.io/v1alpha1/nodes/s1u-03-x86/cameras?namespace=rlark-robot");
  assert.equal(cameraFramePath("s1u-03-x86", "rlark-robot", "wrist_left"),
    "/api/v1/rlinf.io/v1alpha1/nodes/s1u-03-x86/cameras/wrist_left.jpg?namespace=rlark-robot");
});

test("while the live report cannot be read no view shows a state, the published report gives the body's views only", () => {
  const published = publishedCameras(JSON.stringify({ state: "ok", updated_at: 0, cameras: { head: camera(), torso: camera({ state: "stale" }) } }));
  assert.equal(publishedCameras(undefined), undefined);
  assert.deepEqual(cameraCells(published, true, false).map((cell) => [cell.view, cell.present, cell.badge, cell.tone, cell.age, cell.frames]), [
    ["head", true, "读不到实时状态", "stopped", "—", false],
    ["torso", true, "读不到实时状态", "stopped", "—", false],
    ["wrist_left", false, "该本体没有这路相机", "stopped", "—", false],
    ["wrist_right", false, "该本体没有这路相机", "stopped", "—", false],
  ]);
  assert.ok(cameraCells(undefined, false, false).every((cell) => cell.present === undefined && cell.badge === "Live state unavailable" && !cell.frames));
});
