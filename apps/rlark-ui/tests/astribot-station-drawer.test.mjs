import assert from "node:assert/strict";
import test from "node:test";
import { admissionLabels, conditionLabel, conditionStatusClass, conditionView, stationDeleteReason, stationFacts } from "../dist/test/astribot/lib/stationDrawer.js";

test("known conditions are localized while unknown types retain their API names", () => {
  assert.equal(conditionLabel("Observed", true), "已观测");
  assert.equal(conditionLabel("IdentityMatched", false), "Identity matched");
  assert.equal(conditionLabel("VendorSpecific", true), "VendorSpecific");
});

test("condition status classes cover True False and Unknown and sort unhealthy first", () => {
  assert.equal(conditionStatusClass("True"), "status-online");
  assert.equal(conditionStatusClass("False"), "status-failed");
  assert.equal(conditionStatusClass("Unknown"), "status-pending");
  const result = conditionView({ status: { node: { conditions: [
    { type: "A", status: "True" }, { type: "Z", status: "Unknown" }, { type: "B", status: "False" },
  ] } } }, Date.parse("2026-11-11T12:00:00Z"), true);
  assert.deepEqual(result.map((item) => item.type), ["B", "Z", "A"]);
});

test("facts expose runtime, reboot count, heartbeat and cluster", () => {
  assert.deepEqual(stationFacts({ status: { node: { bootTime: "2026-11-08T09:00:00Z", reboots: 2, lastSeenTime: "2026-11-11T11:59:48Z" } } }, { name: "cluster-a" }, Date.parse("2026-11-11T12:00:00Z"), true).map((fact) => fact.value), ["3 天 3 小时", "2", "12 秒前", "cluster-a"]);
});

test("delete reason follows stationDeletable state and admission labels localize all states", () => {
  assert.match(stationDeleteReason({ spec: {}, status: { node: { state: "Bound" } } }, true, true), /同名 Node/);
  assert.equal(stationDeleteReason({ spec: {}, status: { node: { state: "Offline" } } }, false, true), "");
  assert.deepEqual(admissionLabels({ state: "Joined", issued_by: "admin", expires_at: "2026-11-20T16:53:00Z" }, true), { state: "已入网", issuer: "admin", expires: "2026-11-20T16:53:00Z" });
  assert.equal(admissionLabels(undefined, false).state, "No admission code");
});
