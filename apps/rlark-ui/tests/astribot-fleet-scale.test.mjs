import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { runInNewContext } from "node:vm";
import ts from "typescript";
import * as siteNodeDisplay from "../dist/test/astribot/lib/siteNodeDisplay.js";
import { computeFleetSummary, computeVirtualWindow, defaultViewModeForCount, filterAndSortFleetItems, parseStationSummary, selectFleetSummary } from "../dist/test/astribot/lib/stationFleet.js";

const station = (name, state) => ({
  metadata: { name, namespace: "robots" }, spec: { nodeUID: name },
  labels: { "rlark.io/robot-model": "s1u", ...(state ? { "rlark.io/power-state": state } : {}) },
  status: { node: { state: "Bound", conditions: [] } },
});

const source = readFileSync(new URL("../src/astribot/components/useRobotPower.ts", import.meta.url), "utf8");

test("robot power mount effect re-enables state updates after StrictMode cleanup", () => {
  assert.match(source, /useEffect\(\(\) => \{\s*mounted\.current = true;\s*return \(\) => \{\s*mounted\.current = false;/);
});

test("fleet sorting puts severe alerts first with stable name ordering", () => {
  const result = filterAndSortFleetItems([station("z", "ready"), station("b", "estop"), station("a", "fault")]);
  assert.deepEqual(result.map((item) => item.metadata.name), ["a", "b", "z"]);
});

test("virtual windows clamp bounds and retain fixed row geometry", () => {
  assert.deepEqual(computeVirtualWindow({ totalCount: 1000, scrollTop: 1000, viewportHeight: 400, rowHeight: 40, overscan: 2 }), {
    startIndex: 23, endIndex: 37, offsetTop: 920, totalHeight: 40000,
  });
  assert.deepEqual(computeVirtualWindow({ totalCount: 2, scrollTop: 900, viewportHeight: 300, rowHeight: 50 }), {
    startIndex: 2, endIndex: 2, offsetTop: 100, totalHeight: 100,
  });
  assert.equal(defaultViewModeForCount(13, null), "table");
  assert.equal(defaultViewModeForCount(13, "wall"), "wall");
});

test("five power targets use one fleet refresh per round and settle from the returned Node state", async () => {
  const { outputText } = ts.transpileModule(source, { compilerOptions: {
    target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.CommonJS,
  } });
  const state = [];
  const hooks = {
    useEffect: (effect) => effect(),
    useRef: (current) => ({ current }),
    useState: (initial) => {
      const slot = { value: initial };
      state.push(slot);
      return [initial, (next) => { slot.value = typeof next === "function" ? next(slot.value) : next; }];
    },
  };
  const timers = new Map();
  let sequence = 0;
  let now = 0;
  let writes = 0;
  const exports = {};
  runInNewContext(outputText, {
    exports, performance: { now: () => now },
    window: {
      confirm: () => true,
      setTimeout: (callback, delay) => {
        const timerId = ++sequence;
        timers.set(timerId, { callback, at: now + delay });
        return timerId;
      },
      clearTimeout: (timerId) => timers.delete(timerId),
    },
    require(name) {
      if (name === "react") return hooks;
      if (name === "../api") return { stationsApi: { setRobotPower: async () => { writes++; } } };
      if (name === "../lib/siteNodeDisplay") return siteNodeDisplay;
      throw new Error(`Unexpected module ${name}`);
    },
  });
  const targets = Array.from({ length: 5 }, (_, index) => ({
    key: `robots/robot-${index}`, node: `robot-${index}`, namespace: "robots", desired: "ready", action: "driver_on",
  }));
  let refreshes = 0;
  let individualReads = 0;
  const power = exports.useRobotPower(false, undefined, {
    observe: async (live) => { individualReads += live.length; return {}; },
  }, {
    onCoalescedRefresh: async () => {
      refreshes++;
      if (refreshes === 1) throw new Error("temporary Nodes list failure");
      return Object.fromEntries(targets.slice(0, 4).map(({ key, node, namespace }, index) => [key, {
        metadata: { name: node, namespace, labels: { "rlark.io/power-state": index === 0 ? "ready" : "powered" },
          annotations: { "rlark.io/power": JSON.stringify({ waiting: "driver starting" }) } },
      }]));
    },
  });
  // Requests are the hook state slot initialized as the hook's exported requests object.
  const requests = state.find((slot) => slot.value === power.requests);
  await power.set(targets, "Start drivers");
  assert.equal(writes, 5);
  assert.equal(requests.value[targets[0].key].phase, "accepted");

  for (let round = 1; round <= 6; round++) {
    now = round * 5000;
    for (const [timerId, { at, callback }] of [...timers]) {
      if (at <= now) {
        timers.delete(timerId);
        await callback();
      }
    }
    assert.equal(refreshes, round);
    assert.equal(individualReads, 0);
    if (round === 2) {
      assert.equal(requests.value[targets[0].key].phase, "observed");
      assert.equal(requests.value[targets[1].key].phase, "accepted");
      assert.equal(requests.value[targets[1].key].waiting, "driver starting");
    }
  }
  for (const { key } of targets.slice(1)) assert.equal(requests.value[key].phase, "unconfirmed");
  assert.equal(requests.value[targets[1].key].waiting, "driver starting");
  assert.equal(requests.value[targets[4].key].waiting, undefined);
  assert.equal(timers.size, 0);
});

test("fleet summaries use canonical phases and per-cluster counts", () => {
  const ready = { ...station("robot", "ready"), clusterID: "second" };
  const offline = { ...station("offline", "off"), clusterID: "second",
    status: { node: { state: "offline" } } };
  const retired = { ...station("removed"), clusterID: "first", labels: {}, spec: { nodeUID: "removed", retired: true } };
  const summary = computeFleetSummary([ready, offline, retired]);
  assert.deepEqual(summary.byPhase, { Bound: 1, Offline: 1, Conflict: 0, Retired: 1, Unknown: 0 });
  assert.equal(summary.robots, 2);
  assert.deepEqual(summary.clusters.map(({ clusterId, namespace, total, robots, bound, offline }) =>
    ({ clusterId, namespace, total, robots, bound, offline })), [
    { clusterId: "first", namespace: "robots", total: 1, robots: 0, bound: 0, offline: 0 },
    { clusterId: "second", namespace: "robots", total: 2, robots: 2, bound: 1, offline: 1 },
  ]);
  const legacy = { ...summary, byPhase: { bound: 1, offline: 1, conflict: 0, retired: 1 } };
  assert.deepEqual(parseStationSummary(legacy), summary);
  const pendingReport = computeFleetSummary([{ ...station("pending"), status: undefined }]);
  assert.equal(pendingReport.byPhase.Unknown, 1);
  assert.deepEqual(parseStationSummary(pendingReport), pendingReport);
  assert.equal(parseStationSummary({ ...legacy, clusters: 2 }), undefined);
  assert.equal(parseStationSummary({ ...legacy, clusters: [] }), undefined);
  assert.equal(parseStationSummary({ ...legacy, robots: undefined }), undefined);
  assert.equal(parseStationSummary({ ...legacy, clusters: [{ clusterId: "first" }] }), undefined);
});

test("the default view uses locally detected robots when enrollment counts miss them", () => {
  const local = computeFleetSummary(Array.from({ length: 13 }, (_, index) => station(`robot-${index}`, "ready")));
  const server = parseStationSummary({
    ...local, robots: 0, otherNodes: 13, clusters: local.clusters.map((cluster) => ({ ...cluster, robots: 0 })),
  });
  assert.equal(server.robots, 0);
  assert.equal(defaultViewModeForCount(selectFleetSummary(server, local).robots, null), "table");
  assert.equal(selectFleetSummary(undefined, local), local);
  assert.equal(selectFleetSummary(server, computeFleetSummary([])), server);
});
