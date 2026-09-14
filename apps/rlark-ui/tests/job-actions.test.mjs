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
const sharedSource = await readFile(
  new URL("../src/components/shared.tsx", import.meta.url),
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
  assert.match(
    jobsSource,
    /isStartable \? <Play size=\{14\} \/> : <Square size=\{13\} \/>/,
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

test("worker event tooltip stays compact and shows only recent events", () => {
  assert.match(jobsSource, /\.sort\(\(left, right\) =>/);
  assert.match(jobsSource, /\.slice\(0, 4\)/);
  assert.match(jobsSource, /recentEvents\.map/);
  assert.match(jobsStyles, /-webkit-line-clamp: 2/);
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
  assert.match(createJobSource, /selectRole\(roles\[idx \+ 1\], true\)/);
});

test("job deletion waits for worker cleanup before deleting", () => {
  assert.match(jobsSource, /await waitForJobWorkersStopped\(job\)/);
  assert.match(jobsSource, /task\.status\?\.phase === "Stopped"/);
  assert.match(
    jobsSource,
    /await waitForJobWorkersStopped\(job\);[\s\S]*?method: "DELETE"/,
  );
});

test("job actions report success and keep the selected job detail", () => {
  assert.match(jobsSource, /className="job-action-notice" role="status"/);
  assert.match(
    jobsSource,
    /setActionNotice\(zh \? "任务已删除" : "Job deleted"\)/,
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
  assert.match(createJobSource, /const savedJob = await resp\.json\(\)/);
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

test("stopping a job immediately uses the latest server status", () => {
  const waitForStoppedSource = jobsSource.slice(
    jobsSource.indexOf("const waitForJobWorkersStopped"),
    jobsSource.indexOf("const handleSetStopped"),
  );
  assert.match(waitForStoppedSource, /return current;/);
  assert.doesNotMatch(waitForStoppedSource, /return;\s*\n\s*}/);
  assert.match(
    jobsSource,
    /const stoppedJob = stopped \? await waitForJobWorkersStopped\(job\) : null/,
  );
  assert.match(jobsSource, /stoppedJob \?\?/);
});

test("job storage mappings are derived from the generated resource ID", () => {
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
