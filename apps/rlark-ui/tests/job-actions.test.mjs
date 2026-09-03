import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);
const jobsStyles = await readFile(
  new URL("../src/styles.css", import.meta.url),
  "utf8",
);
const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
  "utf8",
);
const clustersSource = await readFile(
  new URL("../src/pages/Clusters.tsx", import.meta.url),
  "utf8",
);
const appSource = await readFile(
  new URL("../src/App.tsx", import.meta.url),
  "utf8",
);

test("failed jobs clean residual workers before starting", () => {
  assert.match(
    jobsSource,
    /job\.phase === "Failed"\s*\? "clean-start"\s*:\s*"start"/,
  );
  assert.match(
    jobsSource,
    /action === "clean-start"\s*\? await handleCleanStart\(job\)/,
  );
  assert.match(jobsSource, /job\.phase === "Failed"/);
  assert.match(
    jobsSource,
    /body: JSON\.stringify\(\{ spec: \{ stopped: true \} \}\)/,
  );
  assert.match(jobsSource, /await waitForFailedJobCleanup\(job\)/);
  assert.match(
    jobsSource,
    /current\.phase === "Stopped" && current\.runningWorkers === 0/,
  );
  assert.match(
    jobsSource,
    /body: JSON\.stringify\(\{ spec: \{ stopped: false \} \}\)/,
  );
  assert.match(jobsSource, /isStartable \? <Play size=\{14\} \/> : <Square/);
});

test("primary job actions are visible and deletion stays in more actions", () => {
  assert.match(jobsSource, /className="job-row-action"[\s\S]*?"复制"/);
  assert.match(jobsSource, /className="job-row-action"[\s\S]*?"重启"/);
  assert.match(jobsSource, /job-quick-lifecycle[\s\S]*?"停止"/);
  assert.match(
    jobsSource,
    /data-tooltip=\{zh \? "更多操作" : "More actions"\}/,
  );
  assert.match(
    jobsSource,
    /className="action-dropdown-item danger"[\s\S]*?"删除"/,
  );
  assert.match(jobsSource, /onRestart=\{\(\) => setRestartTarget\(job\)\}/);
  assert.match(jobsStyles, /\.job-row-action/);
});

test("job lifecycle actions use the shared in-app confirmation dialog", () => {
  assert.doesNotMatch(jobsSource, /\bconfirm\(/);
  assert.match(jobsSource, /function JobLifecycleConfirmDialog/);
  assert.match(jobsSource, /className="modal-backdrop job-lifecycle-backdrop"/);
  assert.match(jobsSource, /清理后启动任务？/);
});

test("worker event tooltip stays compact and shows only recent events", () => {
  assert.match(jobsSource, /\.sort\(\(left, right\) =>/);
  assert.match(jobsSource, /\.slice\(0, 4\)/);
  assert.match(jobsSource, /recentEvents\.map/);
  assert.match(jobsStyles, /-webkit-line-clamp: 2/);
});

test("long job IDs remain fully readable", () => {
  assert.match(jobsSource, /job-id-cell/);
  assert.match(jobsSource, /title=\{job\.id\}/);
  assert.match(jobsStyles, /overflow-wrap: anywhere/);
  assert.match(jobsStyles, /\.job-id-cell\.is-long strong/);
});

test("job deletion waits for worker cleanup before deleting", () => {
  assert.match(jobsSource, /await waitForJobWorkersStopped\(job\)/);
  assert.match(jobsSource, /task\.status\?\.phase === "Stopped"/);
  assert.match(
    jobsSource,
    /await waitForJobWorkersStopped\(job\);[\s\S]*?method: "DELETE"/,
  );
});

test("job actions report success and return to the list", () => {
  assert.match(jobsSource, /className="job-action-notice" role="status"/);
  assert.match(
    jobsSource,
    /setActionNotice\(zh \? "任务已删除" : "Job deleted"\)/,
  );
  assert.match(jobsSource, /if \(selectedName\) onSelect\(undefined\)/);
  assert.match(jobsStyles, /\.job-action-notice/);
});

test("job submission reports success and returns to the job list", () => {
  assert.match(createJobSource, /onSuccess: \(message: string\) => void/);
  assert.match(createJobSource, /\? "任务提交成功"/);
  assert.match(appSource, /setJobSubmitNotice\(message\)/);
  assert.match(appSource, /navigate\("jobs", undefined, \{ replace: true \}\)/);
  assert.match(
    appSource,
    /className="job-action-notice app-job-submit-notice"/,
  );
  assert.match(appSource, /role="status"/);
});

test("worker SSH copy is disabled when the jump host is unavailable", () => {
  assert.match(jobsSource, /if \(!value\) return false/);
  assert.match(jobsSource, /disabled=\{!sshCommand\}/);
  assert.match(jobsSource, /未配置 SSH 跳板地址/);
  assert.match(jobsSource, /\{sshCommand && \(/);
});

test("worker details show cluster and link node names", () => {
  assert.match(
    jobsSource,
    /cluster: pod\.taskNamespace \|\| pod\.namespace \|\| "—"/,
  );
  assert.match(jobsSource, /zh \? "集群" : "Cluster"/);
  assert.doesNotMatch(jobsSource, /zh \? "申请 CPU" : "CPU request"/);
  assert.doesNotMatch(jobsSource, /zh \? "申请内存" : "Memory request"/);
  assert.match(jobsSource, /function WorkerNodeLink/);
  assert.match(jobsSource, /onClick=\{\(\) => onSelectNode\(node\)\}/);
  assert.match(jobsStyles, /\.worker-node-link/);
  assert.match(
    clustersSource,
    /realNodes\.find\(\(n\) => n\.metadata\.name === selectedNodeName\)/,
  );
});

test("cloning a job without a node selector preserves automatic placement", () => {
  assert.match(createJobSource, /sourceJob\.resources\s*\.filter/);
  assert.match(
    createJobSource,
    /parseNodeSelectorStr\(resource\.nodeSelector\)/,
  );
  assert.match(
    createJobSource,
    /\.map\(\(resource\) => \[resource\.role, "model" as const\]\)/,
  );
});
