import assert from "node:assert/strict";
import test from "node:test";
import {
  clusterForStation,
  formatRelativeTime,
  shortUID,
  stationDisplayName,
  stationNetworkState,
  stationStatusClass,
  stationUptime,
} from "../dist/test/astribot/lib/stationDisplay.js";

const station = { metadata: { namespace: "ns" }, spec: { sequence: 7, displayName: "Station-0007" } };
const cluster = { agentNamespace: "ns", name: "集群" };

test("Station display follows the current cluster name and preserves the sequence", () => {
  assert.equal(stationDisplayName(station, [cluster]), "集群-0007");
  assert.equal(stationDisplayName(station, [{ ...cluster, name: "新名称" }]), "新名称-0007");
  assert.equal(station.spec.displayName, "Station-0007");
});

test("ambiguous or conflicted cluster namespace cannot select a cluster", () => {
  assert.equal(clusterForStation([cluster, { ...cluster, name: "other" }], "ns"), undefined);
  assert.equal(clusterForStation([{ ...cluster, agentNamespaceConflict: true }], "ns"), undefined);
  assert.equal(stationDisplayName(station, []), "Station-0007");
});

test("network state is derived only from the NetworkVerified condition", () => {
  const withCondition = (status) => ({ status: { conditions: [{ type: "NetworkVerified", status }] } });
  assert.equal(stationNetworkState(withCondition("True")), "verified");
  assert.equal(stationNetworkState(withCondition("False")), "failed");
  assert.equal(stationNetworkState(withCondition("Unknown")), "unknown");
  assert.equal(stationNetworkState({ status: { conditions: [{ type: "RLarkLinked", status: "True" }] } }), "unknown");
  assert.equal(stationNetworkState({}), "unknown");
});

test("phase maps onto shared status pill variants", () => {
  assert.equal(stationStatusClass("Bound"), "status-online");
  assert.equal(stationStatusClass("Offline"), "status-pending");
  assert.equal(stationStatusClass("Conflict"), "status-failed");
  assert.equal(stationStatusClass("Retired"), "status-stopped");
  assert.equal(stationStatusClass("Unknown"), "status-stopped");
});

test("uptime and relative time tolerate missing, invalid and future values", () => {
  const now = Date.parse("2026-11-11T12:00:00Z");
  assert.equal(stationUptime("2026-11-08T09:00:00Z", now, true), "3 天 3 小时");
  assert.equal(stationUptime("2026-11-11T10:30:00Z", now, false), "1h 30m");
  assert.equal(stationUptime("2026-11-11T12:01:00Z", now, false), "—");
  assert.equal(stationUptime(undefined, now, true), "—");
  assert.equal(stationUptime("not-a-date", now, true), "—");
  assert.equal(stationUptime("2026-11-11T11:15:00Z", now, true), "45 分钟");
  assert.equal(formatRelativeTime("2026-11-11T11:59:48Z", now, true), "12 秒前");
  assert.equal(formatRelativeTime("2026-11-11T09:00:00Z", now, false), "3h ago");
  assert.equal(formatRelativeTime("2026-11-11T12:00:30Z", now, false), "0s ago");
  assert.equal(formatRelativeTime(undefined, now, false), "—");
});

test("shortUID keeps short values and abbreviates long UIDs", () => {
  assert.equal(shortUID("4fd62c79-629f-4fec-b0a3-7fe739757021"), "4fd62c79…7021");
  assert.equal(shortUID("abc"), "abc");
  assert.equal(shortUID(undefined), "—");
});
