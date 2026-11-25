import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

// AdminNodeRowMenu 的取消纳管/驱逐门控输入必须来自 rlarkManaged。
// activeWorkloads.running 统计的是节点上全部非终态 Pod（含 longhorn/metallb 等
// 系统组件），用它门控会让任何带系统 Pod 的节点永远无法取消纳管。
const browserSource = readFileSync(
  new URL("../src/components/NodeResourceBrowser.tsx", import.meta.url),
  "utf8",
);

test("node action gating is fed by rlarkManaged, not the all-Pod running count", () => {
  assert.match(
    browserSource,
    /runningJobs=\{\s*node\.status\?\.activeWorkloads\?\.rlarkManaged \?\? 0\s*\}/,
  );
  assert.doesNotMatch(
    browserSource,
    /runningJobs=\{\s*node\.status\?\.activeWorkloads\?\.running \?\? 0\s*\}/,
  );
});

test("unmanage stays gated on a cordoned node with no RLark workloads", () => {
  assert.match(
    browserSource,
    /const canUnmanage = agentUnreachable \|\| \(cordoned && runningJobs === 0\)/,
  );
});

// 暂停调度/恢复调度与取消纳管都以 spec.unschedulable（持久意图）为准。
// status.phase 由控制面稍后写入，用它门控会在暂停调度后的窗口里把
// “驱逐任务/取消纳管”误禁用，并短暂显示“取消驱逐”。
test("node action gating ignores the controller-reported Cordoned phase", () => {
  assert.doesNotMatch(browserSource, /node\.status\?\.phase === "Cordoned"/);
  assert.doesNotMatch(browserSource, /cordonedPhase/);
});
