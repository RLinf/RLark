import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  clusterNodeNamespaces,
  isClusterSchedulable,
  isSchedulableClusterValue,
  schedulableNodeNamespaces,
} from "../dist/test/utils/job.js";

const online = { id: "cluster-a", name: "cluster-a", phase: "Online" };
const downgraded = { id: "cluster-b", name: "cluster-b", phase: "Downgraded" };
const cordoned = { id: "gpu136", name: "gpu136", phase: "Cordoned" };
const offline = { id: "cluster-c", name: "cluster-c", phase: "Offline" };
const delisted = { id: "cluster-d", name: "cluster-d", phase: "Delisted" };

test("only Online and Downgraded clusters accept new placement", () => {
  assert.equal(isClusterSchedulable(online), true);
  assert.equal(isClusterSchedulable(downgraded), true);
  assert.equal(isClusterSchedulable(cordoned), false);
  assert.equal(isClusterSchedulable(offline), false);
  assert.equal(isClusterSchedulable(delisted), false);
});

test("nodes of stopped clusters are excluded from placement candidates", () => {
  const namespaces = schedulableNodeNamespaces([
    online,
    downgraded,
    cordoned,
    offline,
    delisted,
  ]);
  assert.deepEqual([...namespaces].sort(), ["cluster-a", "cluster-b"]);
});

test("placement candidates follow the cluster agent namespace", () => {
  const namespaces = schedulableNodeNamespaces([
    {
      id: "legacy",
      name: "legacy",
      agentNamespace: "workers-legacy",
      phase: "Online",
    },
  ]);
  assert.equal(namespaces.has("workers-legacy"), true);
  assert.equal(namespaces.has("legacy"), true);
});

test("a cloned job's stale or stopped cluster is not a valid target", () => {
  const schedulable = [online, downgraded];
  assert.equal(isSchedulableClusterValue("gpu136", schedulable), false);
  assert.equal(isSchedulableClusterValue("cluster-a", schedulable), true);
  assert.equal(isSchedulableClusterValue("cluster-b", schedulable), true);
  assert.equal(
    isSchedulableClusterValue("workers-legacy", [
      {
        id: "legacy",
        name: "legacy",
        agentNamespace: "workers-legacy",
        phase: "Online",
      },
    ]),
    true,
  );
  // A cluster outside the schedulable list must be rejected even when its
  // name matches an id alias.
  assert.equal(
    isSchedulableClusterValue("renamed", [
      { id: "renamed", name: "renamed", phase: "Cordoned" },
    ]),
    false,
  );
});

test("a cluster selection resolves every namespace alias", () => {
  const legacy = {
    id: "legacy",
    name: "legacy",
    agentNamespace: "workers-legacy",
    phase: "Online",
  };
  assert.deepEqual([...clusterNodeNamespaces("legacy", [legacy])].sort(), [
    "legacy",
    "workers-legacy",
  ]);
  assert.deepEqual(
    [...clusterNodeNamespaces("workers-legacy", [legacy])].sort(),
    ["legacy", "workers-legacy"],
  );
  assert.deepEqual([...clusterNodeNamespaces("unknown", [])], ["unknown"]);
});

test("CreateJob validates and renders only schedulable placement targets", async () => {
  const source = await readFile(
    new URL("../src/pages/CreateJob.tsx", import.meta.url),
    "utf8",
  );
  assert.match(source, /schedulableNodeNamespaces\(list\)/);
  assert.match(source, /nodes=\{placementNodes\}/);
  assert.match(
    source,
    /!isSchedulableClusterValue\(resource\.cluster, availableClusters\)/,
  );
  assert.match(source, /selectedNames\.length > selectedNodes\.length/);
  assert.match(source, /matchedNodes\.length === 0/);
});

test("cloned or restarted drafts drop nodes that are no longer schedulable", async () => {
  const source = await readFile(
    new URL("../src/pages/CreateJob.tsx", import.meta.url),
    "utf8",
  );
  // 复制或编辑后重启时，源任务选择的节点可能已不可用或不属于当前集群。
  // 表单数据必须移除这些节点，提交内容和 YAML 预览才不会继续引用它们。
  assert.match(source, /nodeAvailabilityState\(node\)\.kind !== "unavailable"/);
  assert.match(
    source,
    /const keptNames = hostnames\.filter\(\(name\) => selectableNames\.has\(name\)\)/,
  );
  assert.match(source, /nodeSelector: selectorToStr\(selector\)/);
  assert.match(source, /replicas: keptNames\.length/);
  assert.match(source, /delete selector\["kubernetes\.io\/hostname"\]/);
});
