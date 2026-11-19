import assert from "node:assert/strict";
import test from "node:test";
import { forEachLimited } from "../dist/test/astribot/lib/forEachLimited.js";

test("work runs on every item, at most limit at a time", async () => {
  let active = 0;
  let maximum = 0;
  const done = [];
  await forEachLimited(["one", "two", "three", "four", "five", "six"], 4, new AbortController().signal, async (item) => {
    active += 1;
    maximum = Math.max(maximum, active);
    await new Promise((resolve) => setTimeout(resolve, 5));
    active -= 1;
    done.push(item);
  });
  assert.equal(maximum, 4);
  assert.deepEqual(done.sort(), ["five", "four", "one", "six", "three", "two"]);
});

test("no more work starts once the signal aborts", async () => {
  const controller = new AbortController();
  let started = 0;
  const pending = [];
  const work = forEachLimited(["one", "two", "three", "four", "five"], 4, controller.signal, () => {
    started += 1;
    return new Promise((resolve) => pending.push(resolve));
  });
  assert.equal(started, 4);
  controller.abort();
  for (const resolve of pending) resolve();
  await work;
  assert.equal(started, 4);
});
