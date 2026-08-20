import assert from "node:assert/strict";
import test from "node:test";
import { updateNodeModelMetadata } from "../dist/test/utils/nodeBatchMetadata.js";

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
