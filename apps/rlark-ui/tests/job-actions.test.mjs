import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);
const jobsStyles = await readStyles();
const mockBackendSource = await readFile(
  new URL("../src/mockBackend.ts", import.meta.url),
  "utf8",
);
const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
  "utf8",
);
const backendSource = await readFile(
  new URL("../src/backend.ts", import.meta.url),
  "utf8",
);
const sharedSource = await readFile(
  new URL("../src/components/shared.tsx", import.meta.url),
  "utf8",
);
const clustersSource = await readFile(
  new URL("../src/pages/Clusters.tsx", import.meta.url),
  "utf8",
);
const nodesSource = await readFile(
  new URL("../src/utils/nodes.ts", import.meta.url),
  "utf8",
);
const nodeResourceBrowserSource = await readFile(
  new URL("../src/components/NodeResourceBrowser.tsx", import.meta.url),
  "utf8",
);
const appSource = await readFile(
  new URL("../src/App.tsx", import.meta.url),
  "utf8",
);
const { crdToJob } = await import("../dist/test/utils/crd.js");

test("cloning uses the user-facing task role instead of the resource name", () => {
  const job = crdToJob({
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Job",
    metadata: { name: "jo-a746801868d9410e" },
    spec: {
      tasks: [
        {
          name: "65b0-89d2-8272",
          head: true,
          agentType: "Kubernetes",
          role: "Actor",
          nodeSelector: {},
          kubernetes: {
            workload: {
              template: {
                spec: {
                  containers: [
                    {
                      name: "main",
                      image: "busybox",
                      env: [{ name: "RLARK_TASK_ROLE", value: "Learner" }],
                    },
                  ],
                },
              },
            },
          },
        },
      ],
    },
  });

  assert.deepEqual(job.defaultRoles, ["Learner"]);
  assert.equal(job.resources[0].role, "Learner");
  assert.equal(job.headerRole, "Learner");
});

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
    /jobsApi\.setStopped\(job\.name, true\)/,
  );
  assert.match(jobsSource, /await waitForFailedJobCleanup\(job\)/);
  assert.match(
    jobsSource,
    /current\.phase === "Stopped" && current\.runningWorkers === 0/,
  );
  assert.match(
    jobsSource,
    /jobsApi\.setStopped\(job\.name, false\)/,
  );
  assert.match(
    jobsSource,
    /isStartable && !isFailed \? <Play size=\{14\} \/> : <Square size=\{13\} \/>/,
  );
});

test("job icon actions provide visible hover hints", () => {
  assert.match(jobsSource, /title=\{lifecycleLabel\}/);
  assert.match(
    jobsSource,
    /data-tooltip=\{zh \? "更多操作" : "More actions"\}/,
  );
  assert.match(jobsSource, /onRestart=\{\(\) => setRestartTarget\(job\)\}/);
});

test("job lifecycle actions use the shared in-app confirmation dialog", () => {
  assert.doesNotMatch(jobsSource, /\bconfirm\(/);
  assert.match(jobsSource, /function JobLifecycleConfirmDialog/);
  assert.match(jobsSource, /className="modal-backdrop job-lifecycle-backdrop"/);
  assert.match(jobsSource, /清理后启动任务？/);
});

test("worker node shows the shared info tooltip in the warning color", () => {
  assert.match(jobsSource, /isDiskUsageWarning\(n\)/);
  assert.match(
    jobsSource,
    /\.\.\.observedNodes.*nodesApi\.get\(nodeName, \{ namespace \}\)/s,
  );
  assert.match(jobsSource, /setDetailNodeDiskWarningMap\(diskWarningMap\)/);
  assert.match(
    jobsSource,
    /detailNodeDiskWarningMap\[worker\.node\].*nodeDiskWarningMap\[worker\.node\]/s,
  );
  assert.match(jobsSource, /variant="danger"/);
  assert.match(jobsSource, /className=\{`status-info\$\{variant/);
  assert.match(jobsSource, /onMouseEnter=\{show\}/);
  assert.match(jobsSource, /onFocus=\{show\}/);
  assert.match(jobsSource, /磁盘即将用满，请及时清理空间/);
  assert.match(jobsSource, /statusTitle=\{\s*zh \? "健康与容量告警"/);
  assert.match(
    jobsStyles,
    /\.status-info\.status-info-danger \{\s*color: var\(--danger\);\s*\}/,
  );
  assert.doesNotMatch(
    jobsStyles,
    /\.status-info\.status-info-danger \{[^}]*border-color:/s,
  );
  assert.doesNotMatch(
    jobsStyles,
    /\.status-info\.status-info-danger \{[^}]*background:/s,
  );
  assert.doesNotMatch(jobsStyles, /\.worker-disk-warning-popover/);
  assert.doesNotMatch(jobsStyles, /\.worker-node-with-warning\.is-warning/);
  assert.match(jobsStyles, /--danger: #e05270/);
});

test("mock topology exposes a real disk warning on gpu-cloud-01", () => {
  assert.match(nodesSource, /node\.id === "gpu-cloud-01" \? 90 : 40/);
  assert.match(nodesSource, /storage: \{/);
  assert.match(nodesSource, /usedBytes: diskUsedBytes/);
});

test("node detail shows real disk usage with the requested card layout", () => {
  assert.match(clustersSource, /label: zh \? "磁盘" : "Storage"/);
  assert.match(clustersSource, /getNodeDiskUsage\(node\)/);
  assert.match(clustersSource, /node\.status\?\.diskPressure === true/);
  assert.match(clustersSource, /磁盘使用率已达到/);
  assert.match(clustersSource, /<AlertCircle size=\{16\} \/>/);
  assert.match(clustersSource, /className="node-capacity-alert"/);
  assert.match(clustersSource, /className="node-capacity-alert-tooltip"/);
  assert.match(clustersSource, /健康与容量告警/);
  assert.match(
    jobsStyles,
    /\.node-capacity-alert:hover \.node-capacity-alert-tooltip/,
  );
  assert.match(clustersSource, /used\[key\]\?\.endsWith\("%"\)/);
  assert.match(clustersSource, /total - \(requested \?\? 0\)/);
  assert.match(clustersSource, /className="node-capacity-progress"/);
  assert.match(clustersSource, /<em>\{zh \? "剩余量" : "Available"\}<\/em>/);
  assert.match(
    jobsStyles,
    /grid-template-columns: repeat\(3, minmax\(0, 1fr\)\)/,
  );
  assert.match(jobsStyles, /\.node-capacity-card\.is-warning \{/);
  assert.match(
    jobsStyles,
    /\.node-capacity-card\.is-warning \.node-capacity-track i \{\s*background: linear-gradient\([\s\S]*var\(--danger-strong\),[\s\S]*var\(--danger-soft\)/,
  );
  assert.match(
    jobsStyles,
    /\.node-capacity-card\.is-warning \.node-capacity-progress b \{/,
  );
  assert.match(
    jobsStyles,
    /\.node-capacity-alert-tooltip \{[\s\S]*right: calc\(100% \+ 10px\);[\s\S]*bottom: calc\(100% \+ 10px\)/,
  );
  assert.match(jobsStyles, /background: var\(--panel\) !important/);
  assert.doesNotMatch(
    jobsStyles,
    /\.node-capacity-card\.is-warning \{[^}]*background:/s,
  );
  assert.doesNotMatch(
    jobsStyles,
    /\.node-capacity-card\.is-warning \.node-capacity-title > span/,
  );
});

test("worker pending tooltip shows readable reasons without raw Kubernetes messages", () => {
  assert.match(jobsSource, /FailedScheduling: \{ zh: "调度失败"/);
  assert.match(jobsSource, /NodeNotReady: \{ zh: "节点不可用"/);
  assert.match(jobsSource, /ErrImagePull: \{ zh: "镜像拉取失败"/);
  assert.match(jobsSource, /FailedMount: \{ zh: "存储挂载失败"/);
  assert.match(jobsSource, /FailedBinding: \{ zh: "存储卷绑定失败"/);
  assert.match(jobsSource, /CrashLoopBackOff: \{/);
  assert.match(jobsSource, /PIDPressure: \{ zh: "节点进程资源不足"/);
  assert.match(jobsSource, /Evicted: \{ zh: "Worker 已被节点驱逐"/);
  assert.match(jobsSource, /en: "Volume binding failed"/);
  assert.match(jobsSource, /en: "Container repeatedly failed to start"/);
  assert.match(jobsSource, /en: "Node PID pressure"/);
  assert.match(jobsSource, /workerEventReasonLabel\(ev\.reason, zh, failed\)/);
  assert.doesNotMatch(jobsSource, /className="event-message">\{ev\.message\}/);
  assert.match(jobsStyles, /width: min\(360px, calc\(100vw - 24px\)\)/);
});

test("worker pending tooltip groups duplicate reasons and shows the latest four", () => {
  assert.match(jobsSource, /\.sort\(\(left, right\) =>/);
  assert.match(jobsSource, /new Map\(/);
  assert.match(
    jobsSource,
    /workerEventReasonLabel\(event\.reason, zh, failed\)/,
  );
  assert.match(jobsSource, /\.slice\(0, 4\)/);
  assert.match(jobsSource, /recentEvents\.map/);
});

test("worker event colors follow the worker phase", () => {
  assert.match(
    jobsSource,
    /ev\.type === "Normal"[\s\S]*?"event-normal"[\s\S]*?failed[\s\S]*?"event-failed"[\s\S]*?"event-pending"/,
  );
  assert.match(jobsStyles, /\.event-chip\.event-pending[\s\S]*?#fff4df/);
  assert.match(jobsStyles, /\.event-chip\.event-failed[\s\S]*?#fff0f1/);
});

test("worker failed tooltip reuses readable Kubernetes event reasons", () => {
  assert.match(
    jobsSource,
    /phase === "Pending" \|\| phase === "Failed".*podEventsMap\[pod\.name\]/s,
  );
  assert.match(jobsSource, /failed=\{worker\.phase === "Failed"\}/);
  assert.match(jobsSource, /failed[\s\S]*?"失败原因"[\s\S]*?"Failure Reasons"/);
  assert.match(jobsSource, /workerEventReasonLabel\(ev\.reason, zh, failed\)/);
  assert.match(jobsSource, /if \(failed\) return zh \? "Worker 运行失败"/);
  assert.match(
    jobsSource,
    /statusMessage: workerStatusSummary\([\s\S]*?phase === "Failed"/,
  );
  assert.match(
    jobsSource,
    /return failed \? jobFailureMessage\(message, zh\) : message/,
  );
});

test("job list and detail translate raw Kubernetes failure messages", () => {
  assert.match(jobsSource, /function jobFailureMessage/);
  assert.match(
    jobsSource,
    /back-off .*restarting failed container.*workerEventReasonLabel\("CrashLoopBackOff", zh, true\)/s,
  );
  assert.match(jobsSource, /workerEventReasonLabel\("OOMKilled", zh, true\)/);
  assert.match(
    jobsSource,
    /workerEventReasonLabel\("ErrImagePull", zh, true\)/,
  );
  assert.match(
    jobsSource,
    /\.map\(\(ts\) => jobFailureMessage\(ts\.message, zh\)\)/,
  );
  assert.match(
    jobsSource,
    /worker\.phase === "Failed"[\s\S]*?jobFailureMessage\(worker\.statusMessage, zh\)/,
  );
});

test("mock pending workers expose representative reasons through pod events", () => {
  for (const reason of [
    "FailedScheduling",
    "ImagePullBackOff",
    "FailedMount",
    "NodeNotReady",
    "FailedCreatePodSandBox",
  ]) {
    assert.match(mockBackendSource, new RegExp(`reason: "${reason}"`));
  }
  assert.match(
    mockBackendSource,
    /path\.startsWith\("\/api\/v1\/rlinf\.io\/v1alpha1\/pods\/"\)/,
  );
  assert.match(mockBackendSource, /path\.endsWith\("\/events"\)/);
  assert.match(
    mockBackendSource,
    /return json\(\{ events: pendingWorkerEventMap\[podName\] \?\? \[\] \}\)/,
  );
  assert.match(
    jobsSource,
    /podsApi\.events<\{ events\?: NodeEventEntry\[\] \}>\([\s\S]*?podName/,
  );
  assert.match(jobsSource, /events=\{worker\.events \?\? \[\]\}/);
});

test("job IDs remain readable and can be copied from list and detail", () => {
  assert.match(jobsSource, /job-id-cell/);
  assert.match(jobsSource, /title=\{job\.id\}/);
  assert.match(jobsSource, /handleCopyJobId\(job\.id\)/);
  assert.match(jobsSource, /handleCopyResourceId/);
  assert.match(jobsSource, /复制资源 ID/);
  assert.match(jobsStyles, /white-space: nowrap/);
  assert.match(jobsStyles, /text-overflow: ellipsis/);
  assert.match(jobsStyles, /\.job-id-cell\.is-long strong/);
  assert.match(jobsStyles, /\.job-id-copy/);
});

test("switching worker roles scrolls back to the configuration header", () => {
  assert.match(createJobSource, /const roleConfigTopRef = useRef/);
  assert.match(createJobSource, /modalBody\.scrollTop = 0/);
  assert.match(
    createJobSource,
    /selectRole\(roles\[idx \+ 1\]\?\.id \?\? "", true\)/,
  );
});

test("job deletion is delegated to the controller and remains visible", () => {
  const deleteHandler = jobsSource.slice(
    jobsSource.indexOf("const handleDelete"),
    jobsSource.indexOf("const handleSetStopped"),
  );
  assert.doesNotMatch(deleteHandler, /waitForJobWorkersStopped/);
  assert.match(deleteHandler, /jobsApi\.remove\(job\.name\)/);
  assert.match(deleteHandler, /phase: "Deleting" as Phase/);
});

test("job actions report success and keep the selected job detail", () => {
  assert.match(jobsSource, /className="job-action-notice" role="status"/);
  assert.match(
    jobsSource,
    /setActionNotice\(zh \? "任务正在删除" : "Job deletion started"\)/,
  );
  assert.match(jobsSource, /if \(selectedName\) onSelect\(job\.name\)/);
  assert.match(jobsSource, /if \(succeeded\) onSelect\(job\.name\)/);
  assert.match(jobsStyles, /\.job-action-notice/);
});

test("job submission reports success and opens the saved job detail", () => {
  assert.match(
    createJobSource,
    /onSuccess: \(message: string, jobName: string\) => void/,
  );
  assert.match(createJobSource, /const savedJob = isEdit/);
  assert.match(createJobSource, /jobsApi\.replace/);
  assert.match(createJobSource, /jobsApi\.create/);
  assert.match(createJobSource, /savedJob\.metadata\?\.name/);
  assert.match(createJobSource, /\? "任务提交成功"/);
  assert.match(appSource, /setJobSubmitNotice\(message\)/);
  assert.match(appSource, /navigate\("jobs", jobName, \{ replace: true \}\)/);
  assert.match(
    appSource,
    /className="job-action-notice app-job-submit-notice"/,
  );
  assert.match(appSource, /role="status"/);
});

test("job and worker refresh actions show progress", () => {
  assert.match(jobsSource, /refreshing=\{listRefreshing\}/);
  assert.match(jobsSource, /setWorkerRefreshing\(true\)/);
  assert.match(jobsSource, /aria-busy=\{workerRefreshing\}/);
  assert.match(jobsSource, /刷新中\.\.\./);
  assert.match(sharedSource, /const \[localRefreshing, setLocalRefreshing\]/);
  assert.match(sharedSource, /await onRefresh\(\)/);
  assert.match(sharedSource, /disabled=\{isRefreshing\}/);
  assert.match(sharedSource, /aria-busy=\{isRefreshing\}/);
  assert.match(
    sharedSource,
    /className=\{isRefreshing \? "job-action-loading" : ""\}/,
  );
});

test("job, worker, and node refreshes mask only their data regions", () => {
  assert.match(jobsSource, /jobs-table-panel refreshable-region/);
  assert.match(jobsSource, /visible=\{listRefreshing\}/);
  assert.match(jobsSource, /worker-table-scroll refreshable-region/);
  assert.match(jobsSource, /visible=\{workerRefreshing\}/);
  assert.match(
    nodeResourceBrowserSource,
    /node-resource-table-panel refreshable-region/,
  );
  assert.match(nodeResourceBrowserSource, /visible=\{!!refreshing\}/);
  assert.match(sharedSource, /export function RefreshOverlay/);
  assert.match(sharedSource, /className="refreshable-region-overlay"/);
  assert.match(sharedSource, /role="status"/);
  assert.match(
    jobsStyles,
    /\.refreshable-region\.is-refreshing > :not\(\.refreshable-region-overlay\)/,
  );
  assert.match(jobsStyles, /pointer-events: none/);
  assert.match(jobsStyles, /\.refreshable-region-spinner/);
});

test("stopping a job returns after setting the stop marker", () => {
  assert.doesNotMatch(jobsSource, /waitForJobWorkersStopped/);
  assert.match(
    backendSource,
    /setStopped\(name: string, stopped: boolean\)[\s\S]*?body: \{ spec: \{ stopped \} \}/,
  );
  assert.match(jobsSource, /\.\.\.j,[\s\S]*?stopped,/);
  assert.match(
    jobsSource,
    /setActionNotice\([\s\S]*?"任务已提交停止"[\s\S]*?"Job stop submitted"/,
  );
});

test("stopping jobs disable conflicting lifecycle actions", () => {
  assert.match(
    jobsSource,
    /const isStopping = job\.stopped && job\.phase !== "Stopped"/,
  );
  assert.match(
    jobsSource,
    /disabled=\{pending \|\| isDeleting \|\| isStopping\}/,
  );
  assert.match(
    jobsSource,
    /disabled=\{\s*lifecycleActions\.pending !== null \|\| isDeleting \|\| isStopping\s*\}/,
  );
});

test("deleting jobs disable metadata editing", () => {
  assert.match(
    jobsSource,
    /const isUpdating = lifecycleActions\.pending !== null \|\| isDeleting/,
  );
  assert.match(
    jobsSource,
    /const isUpdating =\s*lifecycleActions\.pending !== null \|\| job\.phase === "Deleting"/,
  );
});

test("job creation does not compute legacy PVC storage mappings", () => {
  assert.doesNotMatch(createJobSource, /computePvcStorageMap/);
  assert.match(createJobSource, /name: jobResourceName/);
});

test("worker details show cluster and link node names", () => {
  assert.match(
    jobsSource,
    /cluster: pod\.taskNamespace \|\| pod\.namespace \|\| "—"/,
  );
  assert.match(jobsSource, /zh \? "集群" : "Cluster"/);
  assert.match(jobsSource, /function WorkerNodeLink/);
  assert.match(jobsSource, /onClick=\{\(\) => onSelectNode\(node\)\}/);
  assert.match(jobsStyles, /\.worker-chip-link/);
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
