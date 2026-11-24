import assert from "node:assert/strict";
import test from "node:test";
import {
  latestRemovalOperation,
  removalAppliesToNode,
  removalFailureMessage,
  removalIsActive,
  removalNextPollDelayMs,
  removalNoProgressMs,
  removalOperationKey,
  removalOutcome,
  removalRequestID,
  removalLooksStuck,
  removalStageProgress,
  parseStoredRemovals,
} from "../dist/test/astribot/lib/nodeRemoval.js";

const makeOperation = (name, created, extra = {}) => ({
  metadata: { name, namespace: "cluster-ns", creationTimestamp: created },
  spec: { action: "Remove", nodeName: "worker-1", clusterID: "cluster-1", ...extra.spec },
  status: { phase: "Running", stage: "Drain", conditions: [], ...extra.status },
});

test("request IDs are deterministic, DNS-safe, bounded, and change with attempt", () => {
  const first = removalRequestID("a cluster/that is long", "worker-1", "uid:abc", 0);
  assert.equal(first, removalRequestID("a cluster/that is long", "worker-1", "uid:abc", 0));
  assert.notEqual(first, removalRequestID("a cluster/that is long", "worker-1", "uid:abc", 1));
  assert.match(first, /^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/);
  assert.ok(first.length <= 128);
});

test("the most recently created Remove operation wins", () => {
  const operations = [
    makeOperation("old", "2026-11-20T10:00:00Z"),
    makeOperation("join", "2026-11-24T10:00:00Z", { spec: { action: "Join" } }),
    makeOperation("new", "2026-11-21T10:00:00Z"),
  ];
  assert.equal(latestRemovalOperation(operations, "worker-1").metadata.name, "new");
  assert.equal(removalOperationKey("cluster-1", "worker-1"), "cluster-1/worker-1");
});

test("UID, recreation, and age determine whether an operation applies", () => {
  const now = Date.parse("2026-11-23T00:00:00Z");
  const operation = makeOperation("active", "2026-11-22T23:00:00Z", { spec: { nodeUID: "uid-old" } });
  assert.equal(removalAppliesToNode(operation, "uid-new", undefined, now), false);
  assert.equal(removalAppliesToNode(operation, "uid-old", undefined, now), true);
  // The node list has no member Node UID: the operation is matched by name.
  assert.equal(removalAppliesToNode(operation, undefined, undefined, now), true);
  const succeeded = makeOperation("done", "2026-11-22T23:00:00Z", { spec: { nodeUID: "uid-old" }, status: { phase: "Succeeded" } });
  assert.equal(removalAppliesToNode(succeeded, "uid-new", "2026-11-22T23:30:00Z", now), false);
  const oldFailure = makeOperation("old-failure", "2026-11-21T00:00:00Z", { status: { phase: "Failed" } });
  assert.equal(removalAppliesToNode(oldFailure, undefined, undefined, now), false);
  assert.equal(removalAppliesToNode({ ...oldFailure, status: { phase: "Running" } }, undefined, undefined, now), true);
});

test("activity, stage progress, and polling backoff are stable", () => {
  const active = makeOperation("active", "2026-11-22T23:00:00Z");
  assert.equal(removalIsActive(active), true);
  assert.equal(removalOutcome(active), "active");
  assert.equal(removalOutcome({ ...active, status: { phase: "Blocked" } }), "failed");
  assert.equal(removalStageProgress(undefined), 0);
  assert.equal(removalStageProgress("Drain"), Math.round(3 / 7 * 100));
  assert.equal(removalStageProgress("VerifyRemoval"), 100);
  assert.equal(removalNextPollDelayMs(1, () => 0.5), 3000);
  assert.equal(removalNextPollDelayMs(2, () => 0.5), 6000);
  assert.equal(removalNextPollDelayMs(3, () => 0.5), 12000);
  assert.equal(removalNextPollDelayMs(4, () => 0.5), 30000);
  assert.equal(removalNextPollDelayMs(8, () => 0), 24000);
});

test("failure copy preserves Progressing reason and message", () => {
  const failure = makeOperation("failed", "2026-11-22T23:00:00Z", { status: { phase: "Failed", conditions: [{ type: "Progressing", reason: "PowerUnavailable", message: "Robot did not power off" }] } });
  assert.equal(removalFailureMessage(failure), "PowerUnavailable · Robot did not power off");
});

test("long-running active operations are flagged from Progressing transition time only", () => {
  const now = Date.parse("2026-11-23T02:00:00Z");
  const operation = makeOperation("slow", "2026-11-23T00:00:00Z", {
    status: { phase: "Running", stage: "RemovePeer", conditions: [{ type: "Progressing", status: "True", lastTransitionTime: "2026-11-23T00:00:00Z" }] },
  });
  assert.equal(removalNoProgressMs(operation, now), 2 * 60 * 60 * 1000);
  assert.equal(removalLooksStuck(operation, now), true);
  assert.equal(removalLooksStuck({ ...operation, status: { ...operation.status, phase: "Failed" } }, now), false);
  assert.equal(removalLooksStuck({ ...operation, status: { ...operation.status, phase: "Succeeded" } }, now), false);
  assert.equal(removalLooksStuck({ ...operation, status: { ...operation.status, conditions: [] } }, now), false);
});

test("stored sessions parse valid recent records and ignore malformed or expired data", () => {
  const now = 2_000_000_000_000;
  const stored = [
    { clusterID: "cluster", nodeName: "node", operationName: "remove-1", attempt: 0, startedAt: now - 1000 },
    { clusterID: "cluster", nodeName: "old", operationName: "remove-2", attempt: 1, startedAt: now - 25 * 60 * 60 * 1000 },
    { clusterID: "cluster", nodeName: "bad", operationName: "remove-3", attempt: "1", startedAt: now },
  ];
  assert.deepEqual(parseStoredRemovals(JSON.stringify(stored), now), [stored[0]]);
  assert.deepEqual(parseStoredRemovals("{", now), []);
  assert.deepEqual(parseStoredRemovals(JSON.stringify(Array.from({ length: 12 }, (_, index) => ({ ...stored[0], nodeName: `node-${index}` }))), now).length, 10);
});
