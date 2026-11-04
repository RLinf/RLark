import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { generateJobCRD } from "../dist/test/utils/job.js";
import { crdToJob } from "../dist/test/utils/crd.js";
import { readStyles } from "./read-styles.mjs";

const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
  "utf8",
);
const jobsSource = await readFile(
  new URL("../src/pages/Jobs.tsx", import.meta.url),
  "utf8",
);

function roleResource(role) {
  return {
    role,
    cluster: "cluster-a",
    nodeSelector: "",
    replicas: 1,
    cpu: "8",
    memory: "64Gi",
    gpu: "1",
    devices: [],
    image: "busybox:latest",
    prepareScript: "",
    envs: [],
    mounts: [],
  };
}

// 四个业务角色里 Actor/t1/t2 都会被 mapTaskRole 折叠成 "Actor"，
// Env 折叠成 "Env"，因此 task.role 只剩两个不同取值。
const FOUR_ROLES = ["Actor", "Env", "t1", "t2"];

test("crdToJob 的角色数量按角色名统计，而不是按粗粒度分类", () => {
  const crd = generateJobCRD({
    name: "jo-e89a97ca9a5c46ff",
    type: "Custom",
    headerRole: "Actor",
    roles: FOUR_ROLES,
    roleResources: Object.fromEntries(
      FOUR_ROLES.map((role) => [role, roleResource(role)]),
    ),
    runScript: "echo ready",
    domain: "",
  });

  const coarseRoles = new Set(crd.spec.tasks.map((task) => task.role));
  assert.ok(
    coarseRoles.size < FOUR_ROLES.length,
    "task.role 应当确实把多个业务角色折叠成更少的分类",
  );

  const job = crdToJob(crd);
  assert.equal(job.roleCount, FOUR_ROLES.length);
  assert.deepEqual(job.defaultRoles, FOUR_ROLES);
});

test("任务列表的角色数量列直接渲染 crdToJob 的 roleCount", () => {
  assert.match(jobsSource, /<td>\{job\.roleCount\}<\/td>/);
});

test("预填任务切换任务类型时不改写角色列表", () => {
  const onTypeChange = createJobSource.slice(
    createJobSource.indexOf("const onTypeChange = (next: JobType)"),
    createJobSource.indexOf("const addRole = ("),
  );
  assert.ok(onTypeChange.length > 0, "未找到 onTypeChange 实现");
  // 复制/编辑/编辑后重启场景（sourceJob 非空）必须在改写角色前提前返回。
  assert.match(onTypeChange, /if \(sourceJob\) return;/);
  assert.ok(
    onTypeChange.indexOf("if (sourceJob) return;") <
      onTypeChange.indexOf("setRoles("),
    "提前返回必须出现在 setRoles 之前",
  );
  // 旧实现会把模板角色与已填镜像的预填角色拼接，导致角色重复累积。
  assert.doesNotMatch(onTypeChange, /keptEntries/);
  assert.doesNotMatch(onTypeChange, /templateIds/);
});

test("角色运行配置的镜像复制按钮带复制成功反馈", () => {
  assert.match(jobsSource, /const \[imageCopied, setImageCopied\] = useState/);
  assert.match(jobsSource, /setImageCopied\(true\)/);
  assert.match(jobsSource, /setImageCopied\(false\)/);
  assert.match(
    jobsSource,
    /imageCopied \? "icon-button copy-confirmed" : "icon-button"/,
  );
  assert.match(jobsSource, /imageCopied \? <Check size=\{14\} \/> : <Copy/);
});

test("copy-confirmed 提供明暗两套样式与动画", async () => {
  const styles = await readStyles();
  assert.match(styles, /@keyframes icon-button-copy-pulse/);
  assert.match(
    styles,
    /\.icon-button\.copy-confirmed\s*\{[^}]*animation: icon-button-copy-pulse/,
  );
  assert.match(styles, /\.theme-dark \.icon-button\.copy-confirmed\s*\{/);
  assert.match(
    styles,
    /@media \(prefers-reduced-motion: reduce\)\s*\{\s*\.icon-button\.copy-confirmed/,
  );
});
