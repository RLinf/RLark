import assert from "node:assert/strict";
import test from "node:test";
import { podsToStop, removalStagesDone, verifiedNodeUID } from "../dist/test/astribot/lib/nodeRemoval.js";

test("the node removed is the one RLark verified: its unretired Station's, else the name's", () => {
  const station = (namespace, nodeName, nodeUID, retired, phase) => ({ metadata: { name: nodeUID, namespace }, spec: { retired, binding: { nodeName, nodeUID } }, status: { phase } });
  const stations = [station("office", "nb260706069", "old", true), station("office", "nb260706069", "a36766cc", false, "Bound"), station("robots", "nb260706069", "other", false, "Bound"),
    station("office", "reset-by-hand", "gone", false, "Conflict")];
  assert.equal(verifiedNodeUID(stations, "office", "nb260706069"), "a36766cc");
  // No Station, or one bound to a Node of the name that is gone: removed by name.
  assert.equal(verifiedNodeUID(stations, "office", "cailiang-desktop"), "");
  assert.equal(verifiedNodeUID(stations, "office", "reset-by-hand"), "");
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

test("retired Stations are left out of the list unless asked for, and can be deleted", async () => {
  const { stationRetired, stationsOfPhase, stationDeleteAsked, stationDeleteRefusal } = await import("../dist/test/astribot/lib/nodeRemoval.js");
  const station = (name, phase, retired, annotations) => ({ metadata: { name, namespace: "office", annotations }, spec: { retired, binding: { nodeName: name, nodeUID: name } }, status: { phase } });
  // Station-0002: retired, its status still the last one from before.
  const items = [station("bound", "Bound", false), station("offline", "Offline", false), station("stale", "Offline", true), station("retired", "Retired", true, { "station.rlark.io/delete-refused": "its node is in the cluster" })];
  assert.deepEqual(stationsOfPhase(items, "").map((item) => item.metadata.name), ["bound", "offline"]);
  assert.deepEqual(stationsOfPhase(items, "Offline").map((item) => item.metadata.name), ["offline"]);
  assert.deepEqual(stationsOfPhase(items, "Retired").map((item) => item.metadata.name), ["stale", "retired"]);
  assert.equal(stationRetired(items[2]), true);
  assert.equal(stationDeleteRefusal(items[3]), "its node is in the cluster");
  assert.equal(stationDeleteAsked(station("asked", "Retired", true, { "station.rlark.io/delete-requested-by": "alice" })), true);
});
