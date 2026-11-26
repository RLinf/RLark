import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import YAML from "yaml";
import { generateJobCRD, JOB_TYPE_LABEL } from "../dist/test/utils/job.js";
import { crdToJob, mapRoleToJobType } from "../dist/test/utils/crd.js";
import { toYaml } from "../dist/test/utils/yaml.js";

// Kubernetes 工具链按 YAML 1.1 读取清单，预览的保真度必须按同一版本校验。
function parse(text) {
  return YAML.parse(text, { version: "1.1" });
}

const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
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

function buildCRD(type, roles) {
  return generateJobCRD({
    name: "jo-e89a97ca9a5c46ff",
    type,
    headerRole: roles[0],
    roles,
    roleResources: Object.fromEntries(
      roles.map((role) => [role, roleResource(role)]),
    ),
    runScript: "echo ready",
    domain: "",
  });
}

test("任务类型写入 label 并在读取时还原", () => {
  for (const type of ["RL", "DataCollection", "Evaluation", "Custom"]) {
    const crd = buildCRD(type, ["worker"]);
    assert.equal(crd.metadata.labels[JOB_TYPE_LABEL], type);
    assert.equal(crdToJob(crd).type, type);
  }
});

test("自定义类型的任务不再被识别成 RL", () => {
  const crd = buildCRD("Custom", ["worker", "driver"]);
  // 修复前写入的存量数据：角色被折叠成 Actor 且没有类型 label。
  const legacy = structuredClone(crd);
  delete legacy.metadata.labels;
  legacy.spec.tasks.forEach((task) => {
    task.role = "Actor";
  });
  assert.equal(mapRoleToJobType(legacy.spec.tasks), "RL");
  assert.equal(crdToJob(legacy).type, "RL");
  // 修复后：label 优先，类型保持 Custom。
  assert.equal(crdToJob(crd).type, "Custom");
});

test("自定义类型原样保留用户填写的角色名", () => {
  const crd = buildCRD("Custom", ["worker", "driver"]);
  assert.deepEqual(
    crd.spec.tasks.map((task) => task.role),
    ["worker", "driver"],
  );
});

test("内置类型仍归一到粗粒度分类", () => {
  const crd = buildCRD("RL", ["Actor", "Rollout", "Environment"]);
  assert.deepEqual(
    crd.spec.tasks.map((task) => task.role),
    ["Actor", "Rollout", "Env"],
  );
});

test("缺少 label 的存量任务回落到按角色反推类型", () => {
  const crd = buildCRD("DataCollection", ["Environment"]);
  delete crd.metadata.labels;
  assert.equal(crdToJob(crd).type, "DataCollection");
});

test("空数组序列化为同一行的内联标量", () => {
  const yaml = toYaml({ spec: { volumes: [], name: "a" } });
  assert.match(yaml, /^ {2}volumes: \[\]$/m);
  assert.doesNotMatch(yaml, /^\[\]$/m);
});

test("未填写的字段和条目被全部丢弃的嵌套块不会出现在预览里", () => {
  const yaml = toYaml({
    name: "a",
    empty: "",
    missing: null,
    placement: { nodeSelector: {} },
  });
  assert.equal(yaml, "name: a\n");
  assert.deepEqual(parse(yaml), { name: "a" });
});

test("多行字符串序列化为字面量块标量而不是跨行的引号标量", () => {
  const yaml = toYaml({ spec: { prepareScript: "echo a\n\necho b" } });
  assert.equal(yaml, "spec:\n  prepareScript: |-\n    echo a\n\n    echo b\n");
});

// 预览里的脚本会被用户复制进 kubectl，解析回来必须和表单里填的一模一样。
// 这些取值覆盖的都是手写序列化器曾经改写过语义的边界。
const ROUND_TRIP_CASES = [
  ["首行为空的缩进脚本", "\n  echo first\n  echo second"],
  ["首行缩进的脚本", "  echo a\necho b"],
  ["Python 风格的递进缩进", "if True:\n    print(1)\n\n    print(2)"],
  ["行尾带空格的脚本", "echo a \necho b"],
  ["以换行结尾的脚本", "echo ready\n"],
  ["以空行结尾的脚本", "echo a\n\n"],
  ["只有空白的取值", "   "],
  ["带制表符和回车的取值", "a\tb\r\nc"],
  ["下划线分隔的数字", "1_000"],
  ["布尔拼写", "yes"],
  ["大写布尔拼写", "False"],
  ["开关拼写", "on"],
  ["空值拼写", "~"],
  ["六十进制字面量", "1:2:3"],
  ["时间戳", "2026-11-23"],
  ["十六进制数字", "0x1f"],
  ["无穷大", ".inf"],
  ["冒号分隔的镜像名", "nginx:1.25"],
  ["带引号和反斜杠的取值", 'a\\b"c: d'],
  ["以指示符开头的取值", "- not a sequence"],
  ["单个等号", "="],
];

for (const [name, value] of ROUND_TRIP_CASES) {
  test(`预览里的${name}能原样解析回来`, () => {
    assert.deepEqual(parse(toYaml({ k: value })), { k: value });
    // 序列条目的 "- " 会把块标量整体右移，缩进基准必须跟着变。
    assert.deepEqual(parse(toYaml({ k: ["x", value] })), { k: ["x", value] });
    // 嵌套一层后缩进基准同样要重新计算。
    assert.deepEqual(parse(toYaml({ a: { b: { c: value } } })), {
      a: { b: { c: value } },
    });
  });
}

test("非字符串标量保持原类型", () => {
  const value = { t: true, f: false, n: 1, z: 0, neg: -1.5, arr: [] };
  assert.deepEqual(parse(toYaml(value)), value);
});

test("新建任务的完整预览能解析回提交的取值", () => {
  const crd = generateJobCRD({
    name: "jo-e89a97ca9a5c46ff",
    type: "Custom",
    headerRole: "worker",
    roles: ["worker"],
    roleResources: {
      worker: {
        ...roleResource("worker"),
        prepareScript: "\n  pip install -r requirements.txt",
        envs: [{ key: "RETRIES", value: "1_000" }],
      },
    },
    runScript: "if True:\n    print('ready')\n",
    domain: "",
  });
  const parsed = parse(toYaml(crd));
  assert.equal(parsed.metadata.labels[JOB_TYPE_LABEL], "Custom");
  const task = parsed.spec.tasks[0];
  const expected = crd.spec.tasks[0];
  assert.equal(task.role, "worker");
  assert.equal(
    task.workload.parameters.prepare.inline,
    expected.workload.parameters.prepare.inline,
  );
  assert.equal(
    task.workload.parameters.run.inline,
    expected.workload.parameters.run.inline,
  );
  assert.deepEqual(
    task.workload.template.spec.containers[0].env,
    expected.workload.template.spec.containers[0].env,
  );
  // 表单没填的字段不会出现在预览里，其余结构必须和提交的对象一致。
  assert.doesNotMatch(toYaml(crd), /nodeSelector/);
});

test("新建任务的 YAML 预览不展示前端生成的占位名", () => {
  assert.match(createJobSource, /const previewCrd = isEdit/);
  assert.match(createJobSource, /const yaml = toYaml\(previewCrd\)/);
  assert.match(createJobSource, /assigned by the server on submit/);
});
