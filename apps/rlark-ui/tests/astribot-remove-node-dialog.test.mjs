import assert from "node:assert/strict";
import test from "node:test";
import { removalWorkloadPreview } from "../dist/test/astribot/lib/nodeRemoval.js";

test("workload preview reports an empty list without hidden items", () => {
  assert.deepEqual(removalWorkloadPreview([], 6), { visible: [], hiddenCount: 0 });
});

test("workload preview keeps all items when the list fits", () => {
  assert.deepEqual(removalWorkloadPreview(["task-a", "task-b"], 6), {
    visible: ["task-a", "task-b"],
    hiddenCount: 0,
  });
});

test("workload preview limits long lists and reports the hidden count", () => {
  assert.deepEqual(removalWorkloadPreview(["a", "b", "c", "d", "e", "f", "g", "h"], 6), {
    visible: ["a", "b", "c", "d", "e", "f"],
    hiddenCount: 2,
  });
});
