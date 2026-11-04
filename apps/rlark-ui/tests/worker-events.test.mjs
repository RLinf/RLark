import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import {
  partitionWorkerEvents,
  refineWorkerEventReason,
  workerEventTime,
} from "../dist/test/utils/workerEvents.js";
import { readStyles } from "./read-styles.mjs";

const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);

const failedScheduling = {
  type: "Warning",
  reason: "FailedScheduling",
  message:
    "0/3 nodes are available: waiting for ephemeral volume controller to create the persistentvolumeclaim",
  lastTime: "2026-08-08T09:05:00Z",
};
const diskPressure = {
  type: "Warning",
  reason: "NodeHasDiskPressure",
  lastTime: "2026-08-08T09:06:00Z",
};

test("keeps every event active while the pod is still unscheduled", () => {
  const { active, resolved } = partitionWorkerEvents(
    [failedScheduling, diskPressure],
    false,
  );
  assert.deepEqual(active, [failedScheduling, diskPressure]);
  assert.deepEqual(resolved, []);
});

test("moves pre-binding FailedScheduling to resolved once scheduled", () => {
  const { active, resolved } = partitionWorkerEvents(
    [failedScheduling, diskPressure],
    true,
  );
  assert.deepEqual(active, [diskPressure]);
  assert.deepEqual(resolved, [failedScheduling]);
});

test("does not resolve events unrelated to scheduling", () => {
  const { active, resolved } = partitionWorkerEvents([diskPressure], true);
  assert.deepEqual(active, [diskPressure]);
  assert.deepEqual(resolved, []);
});

test("tolerates entries without a reason", () => {
  const nameless = { type: "Warning" };
  const { active, resolved } = partitionWorkerEvents([nameless], true);
  assert.deepEqual(active, [nameless]);
  assert.deepEqual(resolved, []);
});

test("worker status summary only reports FailedScheduling before binding", () => {
  assert.match(jobsSource, /if \(schedulingFailed && !scheduled\)/);
  assert.match(
    jobsSource,
    /if \(message === "FailedScheduling"\) return undefined;/,
  );
  assert.match(jobsSource, /const scheduled = Boolean\(pod\.node\);/);
  assert.match(jobsSource, /partitionWorkerEvents\(workerEvents, scheduled\)/);
  assert.match(jobsSource, /events: activeWorkerEvents/);
  assert.match(jobsSource, /resolvedEvents: resolvedWorkerEvents/);
});

test("status tooltip renders superseded events in a separate muted section", () => {
  assert.match(jobsSource, /resolvedEvents = \[\]/);
  assert.match(jobsSource, /const recentResolvedEvents = summarizeEvents\(/);
  // 被取代的"镜像已拉取"并没有"恢复"，文案改成中性的历史事件。
  assert.match(jobsSource, /"历史事件" : "Earlier Events"/);
  assert.doesNotMatch(jobsSource, /已恢复/);
  assert.match(jobsSource, /event-chip event-resolved/);
});

const pulling = {
  type: "Normal",
  reason: "Pulling",
  message: 'Pulling image "123"',
  lastTime: "2026-08-08T09:47:15Z",
};
const pulled = {
  type: "Normal",
  reason: "Pulled",
  message: 'Successfully pulled image "rlark/network-sidecar:v1"',
  lastTime: "2026-08-08T09:47:14Z",
};
const pullFailed = {
  type: "Warning",
  reason: "Failed",
  message: 'Failed to pull image "123": invalid reference format',
  lastTime: "2026-08-08T09:47:16Z",
};

test("workerEventTime falls back to 0 for missing or invalid timestamps", () => {
  assert.equal(workerEventTime(pulling), Date.parse(pulling.lastTime));
  assert.equal(workerEventTime({ type: "Normal" }), 0);
  assert.equal(workerEventTime({ type: "Normal", lastTime: "nope" }), 0);
});

test("refineWorkerEventReason narrows the generic kubelet Failed reason", () => {
  assert.equal(
    refineWorkerEventReason("Failed", 'Failed to pull image "123": invalid reference format'),
    "InvalidImageName",
  );
  assert.equal(
    refineWorkerEventReason(
      "Failed",
      'Failed to pull image "a/b:c": manifest unknown',
    ),
    "ErrImagePull",
  );
  // 真正的容器启动失败保持原样，仍然显示为 Worker 启动失败。
  assert.equal(
    refineWorkerEventReason("Failed", 'failed to start container "worker"'),
    "Failed",
  );
  assert.equal(refineWorkerEventReason("Failed", undefined), "Failed");
  assert.equal(refineWorkerEventReason(undefined, "anything"), "");
  assert.equal(refineWorkerEventReason("BackOff", "pull image"), "BackOff");
});

test("keeps only the newest image pull progress event", () => {
  const { active, resolved } = partitionWorkerEvents([pulled, pulling], true);
  assert.deepEqual(active, [pulling]);
  assert.deepEqual(resolved, [pulled]);
});

test("supersedes every pull progress event once the image pull failed", () => {
  const { active, resolved } = partitionWorkerEvents(
    [pulled, pulling, pullFailed],
    true,
  );
  assert.deepEqual(active, [pullFailed]);
  assert.deepEqual(resolved, [pulled, pulling]);
});

test("supersedes pull progress older than the newest warning", () => {
  const staleWarning = {
    type: "Warning",
    reason: "BackOff",
    message: "Back-off restarting failed container",
    lastTime: "2026-08-08T09:48:00Z",
  };
  const { active, resolved } = partitionWorkerEvents(
    [pulling, staleWarning],
    true,
  );
  assert.deepEqual(active, [staleWarning]);
  assert.deepEqual(resolved, [pulling]);
});

test("keeps pull progress that is newer than the last warning", () => {
  const earlierWarning = {
    type: "Warning",
    reason: "FailedCreatePodSandBox",
    lastTime: "2026-08-08T09:47:00Z",
  };
  const { active, resolved } = partitionWorkerEvents(
    [earlierWarning, pulling],
    true,
  );
  assert.deepEqual(active, [earlierWarning, pulling]);
  assert.deepEqual(resolved, []);
});

test("worker event chips are deduped by label keeping the newest timestamp", () => {
  assert.match(jobsSource, /const newest = new Map<string, NodeEventEntry>\(\)/);
  assert.match(
    jobsSource,
    /workerEventTime\(event\) >= workerEventTime\(existing\)/,
  );
  assert.match(
    jobsSource,
    /\.sort\(\(left, right\) => workerEventTime\(right\) - workerEventTime\(left\)\)/,
  );
  assert.match(jobsSource, /\{workerEventLabel\(ev, zh, failed\)\}/);
});

test("worker event labels go through refineWorkerEventReason", () => {
  assert.match(jobsSource, /refineWorkerEventReason\(event\.reason, event\.message\)/);
});

test("status tooltip clears the sticky topbar", async () => {
  const styles = await readStyles();
  const tooltip = styles.match(/\.status-info-tooltip \{[^}]*\}/)?.[0] ?? "";
  const topbar = styles.match(/\.topbar \{[^}]*\}/)?.[0] ?? "";
  const tooltipZ = Number(tooltip.match(/z-index:\s*(\d+)/)?.[1]);
  const topbarZ = Number(topbar.match(/z-index:\s*(\d+)/)?.[1]);
  assert.ok(Number.isFinite(tooltipZ) && Number.isFinite(topbarZ));
  assert.ok(
    tooltipZ > topbarZ,
    `tooltip z-index ${tooltipZ} must stack above topbar ${topbarZ}`,
  );
  assert.match(jobsSource, /document\.querySelector\("\.topbar"\)/);
  assert.match(jobsSource, /const spaceAbove = iconRect\.top - topInset;/);
  assert.match(jobsSource, /Math\.max\(\s*topInset \+ margin,/);
});
