import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import {
  getRemovedLabelKeys,
  updateNodeCategoryLabels,
  updateNodeModelMetadata,
} from "../dist/test/utils/nodeBatchMetadata.js";

function node(category, labels = {}, annotations = {}) {
  return {
    metadata: {
      name: `${category}-node`,
      labels: { "rlark.io/node-category": category, ...labels },
      annotations,
    },
    spec: {},
  };
}

test("updates a GPU model on an edge node", () => {
  const result = updateNodeModelMetadata(node("edge"), {
    gpuModel: " NVIDIA RTX 4090 ",
  });
  assert.equal(result.annotations["rlark.io/gpu-model"], "NVIDIA RTX 4090");
  assert.equal(result.labels["rlark.io/gpu-model"], undefined);
});

test("updates a device model on a cloud node", () => {
  const result = updateNodeModelMetadata(node("cloud"), {
    deviceModel: "Unitree G1",
  });
  assert.equal(result.annotations["rlark.io/device-model"], "Unitree G1");
});

test("blank values remove the selected model annotation", () => {
  const result = updateNodeModelMetadata(
    node(
      "edge",
      { "rlark.io/gpu-model": "legacy" },
      { "rlark.io/gpu-model": "NVIDIA A100" },
    ),
    { gpuModel: "  " },
  );
  assert.equal(result.annotations["rlark.io/gpu-model"], undefined);
  assert.equal(result.labels["rlark.io/gpu-model"], undefined);
  assert.equal(result.removedAnnotationKeys.has("rlark.io/gpu-model"), true);
});

test("removes deselected categories from a multi-category node", () => {
  const result = updateNodeCategoryLabels(
    {
      "rlark.io/node-category-edge": "true",
      "rlark.io/node-category-robot": "true",
    },
    ["edge"],
  );
  assert.equal(result.labels["rlark.io/node-category-edge"], "true");
  assert.equal(result.labels["rlark.io/node-category-robot"], undefined);
  assert.equal(result.removedKeys.has("rlark.io/node-category-robot"), true);
});

test("selected category must NOT appear in removedKeys (regression: patch would null out the new value)", () => {
  const result = updateNodeCategoryLabels({}, ["cloud"]);
  assert.equal(result.labels["rlark.io/node-category-cloud"], "true");
  // 关键断言：新写入的 key 不能同时出现在 removedKeys 里，
  // 否则调用方在 merge-patch 里会把它置 null 把刚写入的 "true" 覆盖掉。
  assert.equal(result.removedKeys.has("rlark.io/node-category-cloud"), false);
  // 未选中的仍应被标记删除
  assert.equal(result.removedKeys.has("rlark.io/node-category-edge"), true);
  assert.equal(result.removedKeys.has("rlark.io/node-category-robot"), true);
  assert.equal(result.removedKeys.has("rlark.io/node-category"), true);
});

test("detects labels removed in the detail editor", () => {
  assert.deepEqual(
    getRemovedLabelKeys({ zone: "a", team: "robot" }, { zone: "a" }),
    ["team"],
  );
});

test("node management keeps selection separate from detail navigation", () => {
  const browserSource = readFileSync(
    new URL("../src/components/NodeResourceBrowser.tsx", import.meta.url),
    "utf8",
  );
  const adminSource = readFileSync(
    new URL("../src/admin/AdminPage.tsx", import.meta.url),
    "utf8",
  );

  assert.match(browserSource, /className="node-row-primary node-detail-link"/);
  assert.doesNotMatch(browserSource, /role="button"\s+tabIndex=\{0\}/);
  assert.match(adminSource, /className="admin-node-overview"/);
});
