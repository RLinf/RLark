import assert from "node:assert/strict";
import test from "node:test";
import { podsToStop, removalStagesDone, stationDeletable, stationsOfPhase, verifiedNodeUID } from "../dist/test/astribot/lib/nodeRemoval.js";

const station = (namespace, name, nodeUID, retired, state) => ({ metadata: { name, namespace }, spec: { nodeUID, retired }, status: { node: { state } } });

test("the node removed is the one RLark verified: its record's, else the name's", () => {
  const stations = [station("office", "removed", "old", true, "Bound"), station("office", "nb260706069", "a36766cc", false, "Bound"), station("robots", "nb260706069", "other", false, "Bound"),
    station("office", "reinstalled", "gone", false, "Conflict")];
  assert.equal(verifiedNodeUID(stations, "office", "nb260706069"), "a36766cc");
  // No record, a removed node's, or one another Node reports under: removed by name.
  assert.equal(verifiedNodeUID(stations, "office", "cailiang-desktop"), "");
  assert.equal(verifiedNodeUID(stations, "office", "removed"), "");
  assert.equal(verifiedNodeUID(stations, "office", "reinstalled"), "");
});

test("the pods that stop are the running ones on this node of this cluster", () => {
  const pod = (name, node, namespace, phase) => ({ metadata: { name, namespace }, spec: { taskName: `task-${name}` }, status: { node, phase } });
  const stop = podsToStop([pod("b", "n", "office", "Running"), pod("a", "n", "office", "Pending"), pod("c", "n", "office", "Succeeded"),
    pod("d", "other", "office", "Running"), pod("e", "n", "robots", "Running")], "office", "n");
  assert.deepEqual(stop, ["task-a", "task-b"]);
});

test("a removal counts the stages up to the one its operation completed", () => {
  assert.equal(removalStagesDone(undefined), 0);
  assert.equal(removalStagesDone("RemovePeer"), 4);
});

test("removed nodes are left out of the list unless asked for, and a record no node stands on can be deleted", () => {
  const items = [station("office", "bound", "a", false, "Bound"), station("office", "offline", "b", false, "Offline"), station("office", "removed", "c", true, "Offline"), station("office", "conflict", "d", false, "Conflict")];
  assert.deepEqual(stationsOfPhase(items, "").map((item) => item.metadata.name), ["bound", "offline", "conflict"]);
  assert.deepEqual(stationsOfPhase(items, "Offline").map((item) => item.metadata.name), ["offline"]);
  assert.deepEqual(stationsOfPhase(items, "Retired").map((item) => item.metadata.name), ["removed"]);
  assert.deepEqual(items.map((item) => stationDeletable(item, true)), [false, false, true, true]);
  assert.deepEqual(items.map((item) => stationDeletable(item, false)), [false, true, true, true]);
});
