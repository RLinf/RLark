import assert from "node:assert/strict";
import test from "node:test";
import { createCameraMediaScheduler } from "../dist/test/astribot/lib/cameraMediaScheduler.js";

test("higher priorities preempt, and release wakes the highest priority FIFO waiter", () => {
  const scheduler = createCameraMediaScheduler({ maxLiveStreams: 1 });
  const changes = new Map();
  const add = (id, priority) => scheduler.acquire(id, priority, (active) => changes.set(id, active));
  const releaseCard = add("card", "card");
  add("pinned", "pinned");
  add("dialog", "dialog");
  assert.deepEqual(scheduler.getStats().activeIds, ["dialog"]);
  assert.equal(changes.get("card"), false);
  const releaseDialog = add("dialog", "dialog");
  releaseDialog();
  assert.deepEqual(scheduler.getStats().activeIds, ["pinned"]);
  releaseCard();
});

test("budget changes notify demotions and promote queued entries", () => {
  const scheduler = createCameraMediaScheduler({ maxLiveStreams: 2 });
  const changes = {};
  const releases = ["a", "b", "c"].map((id) => scheduler.acquire(id, "card", (active) => { changes[id] = active; }));
  assert.equal(scheduler.getStats().activeCount, 2);
  scheduler.setMaxLiveStreams(1);
  assert.equal(scheduler.getStats().activeCount, 1);
  scheduler.setMaxLiveStreams(3);
  assert.equal(scheduler.getStats().activeCount, 3);
  assert.equal(changes.c, true);
  releases.forEach((release) => release());
  assert.equal(scheduler.nextSnapshotDelayMs(5000, 1500, () => 0), 3500);
  assert.equal(scheduler.nextSnapshotDelayMs(1000, 0, () => 0.5), 1500);
});

test("reacquiring an ID grants the new lease and fences the old release", () => {
  const scheduler = createCameraMediaScheduler({ maxLiveStreams: 1 });
  const old = [];
  const replacement = [];
  const waiter = [];
  const releaseOld = scheduler.acquire("same", "card", (active) => old.push(active));
  scheduler.acquire("waiter", "card", (active) => waiter.push(active));
  const releaseReplacement = scheduler.acquire("same", "card", (active) => replacement.push(active));

  assert.deepEqual(old, [true, false]);
  assert.deepEqual(replacement, [true]);
  releaseOld();
  assert.deepEqual(scheduler.getStats().activeIds, ["same"]);
  assert.deepEqual(waiter, []);
  releaseReplacement();
  assert.deepEqual(replacement, [true, false]);
  assert.deepEqual(scheduler.getStats().activeIds, ["waiter"]);
  assert.deepEqual(waiter, [true]);
});
