import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const pickerSource = await readFile(
  new URL("../src/components/ResourcePlacementPicker.tsx", import.meta.url),
  "utf8",
);

test("lists nodes without discovered device resources", () => {
  assert.match(pickerSource, /else if \(resources\.length === 0\)/);
  assert.match(pickerSource, /model: "__undiscovered_device__"/);
  assert.match(pickerSource, /resourceKey: ""/);
  assert.match(pickerSource, /"未分类"/);
  assert.match(pickerSource, /"未发现设备"/);
});

test("shows a truthful placeholder when a device resource has no model", () => {
  assert.match(
    pickerSource,
    /const deviceModel = getNodeDeviceModel\(fullNode\)/,
  );
  assert.match(pickerSource, /resourceKey: deviceModel \? deviceKey : ""/);
  assert.match(pickerSource, /configured: Boolean\(deviceModel\)/);
  assert.match(pickerSource, /"未设置设备型号"/);
  assert.match(pickerSource, /仅按未设置设备型号的节点调度，不申请设备资源/);
  assert.match(pickerSource, /resourceAmount} × \$\{displayModel}/);
});

test("unconfigured devices constrain nodes without requesting fake resources", () => {
  assert.match(
    pickerSource,
    /!resource\.configured \|\| resource\.free >= nextPerWorker/,
  );
  assert.match(pickerSource, /candidates\[0\]\?\.resourceKey/);
  assert.match(pickerSource, /disabled=\{isUnconfiguredDevice\}/);
  assert.match(pickerSource, /仅按未分类节点调度，不申请未发现的设备资源/);
});

test("edit mode restores the original resource and selected nodes", () => {
  assert.match(pickerSource, /nodeSelector: string/);
  assert.match(pickerSource, /parseNodeSelectorStr\(nodeSelector\)/);
  assert.match(pickerSource, /restoresAutomaticMode/);
  assert.match(pickerSource, /placementMode \?\?/);
  assert.match(pickerSource, /onPlacementModeChange\?\.\("manual"\)/);
  assert.match(pickerSource, /selectedNames\.size === matchingNames\.length/);
  assert.match(pickerSource, /setMode\("manual"\)/);
  assert.match(pickerSource, /selectedNames\.has\(name\)/);
  assert.match(pickerSource, /initializedRef\.current = true/);
});

test("translates the unlabeled GPU placeholder instead of hardcoding Chinese", () => {
  assert.match(pickerSource, /const UNLABELED_GPU = "__unlabeled_gpu__";/);
  assert.match(pickerSource, /getNodeGPUModel\(fullNode\) \|\| UNLABELED_GPU/);
  assert.doesNotMatch(pickerSource, /\|\| "未标注 GPU"/);
  assert.match(pickerSource, /zh \? "未标注 GPU" : "Unlabeled GPU"/);
  // Every display site must map the sentinel back to a localized label.
  assert.equal(pickerSource.match(/=== UNLABELED_GPU/g)?.length, 3);
});

test("new placement defaults to manual node selection", () => {
  assert.match(pickerSource, /useState<Mode>\(placementMode \?\? "manual"\)/);
});

test("per-worker summary punctuates by locale instead of reusing a full-width colon", () => {
  assert.match(pickerSource, /zh \? "单 Worker：" : "Per worker: "/);
  assert.doesNotMatch(pickerSource, /"Per worker"\}：/);
});

test("node cards keep long node names readable", () => {
  assert.match(pickerSource, /title=\{name\}/);
  assert.match(pickerSource, /className="placement-node-meta"/);
  assert.match(pickerSource, /className="placement-node-state"/);
});

test("number fields stay clearable while editing", () => {
  // 数字输入框必须经过 IntegerField，由 draft 保存编辑期的原始输入，
  // 失焦后才归一化，否则删除最后一位会被立刻写回 min（0/1）而无法清空。
  assert.match(pickerSource, /function IntegerField\(/);
  assert.match(
    pickerSource,
    /const \[draft, setDraft\] = useState<string \| null>\(null\)/,
  );
  assert.match(pickerSource, /value=\{draft \?\? String\(value\)\}/);
  assert.match(pickerSource, /onBlur=\{\(\) => setDraft\(null\)\}/);
  assert.match(
    pickerSource,
    /raw\.trim\(\) === "" \|\| !Number\.isFinite\(parsed\)\s*\?\s*min/,
  );
  // 两个数字输入框（单 Worker 资源数量、Worker 数量）都必须复用 IntegerField，
  // 全文件只应保留 IntegerField 内部这一处裸 <input type="number">。
  assert.equal(pickerSource.match(/type="number"/g)?.length, 1);
  assert.match(pickerSource, /<IntegerField\s+value=\{resourceAmount\}/);
  assert.match(pickerSource, /<IntegerField\s+value=\{replicas \|\| 1\}/);
});

test("changing resources per worker keeps nodes that still fit", () => {
  // 改单 Worker 资源数量不得清空已选节点：只剔除空闲量不再满足新规格的节点。
  // 编辑期的空值会被 IntegerField 归一化成 min，中间态同样不能误伤选择。
  assert.match(pickerSource, /const selectionWithin = \(amount: number\) =>/);
  assert.match(
    pickerSource,
    /selected\.has\(resource\.node\.metadata\.name\) &&\s*\(!resource\.configured \|\| resource\.free >= amount\)/,
  );
  assert.match(
    pickerSource,
    /commitSelection\(selectionWithin\(nextAmount\), nextAmount\)/,
  );
  // commitSelection 必须能带上新规格，否则 onChange 会回写旧的 gpu/device 数量。
  assert.match(
    pickerSource,
    /const commitSelection = \(next: Set<string>, amount = resourceAmount\)/,
  );
  assert.match(pickerSource, /gpu: kind === "gpu" \? String\(amount\) : "0"/);
});
