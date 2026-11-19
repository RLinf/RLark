import assert from "node:assert/strict";
import test from "node:test";
import { registryClusterOption } from "../dist/test/astribot/lib/clusterOptions.js";

test("registry delivery keeps its stable identity when the cluster is renamed", () => {
  const before = registryClusterOption({ id: "rlark-ksyun-gpu", name: "rlark-ksyun-gpu" });
  const after = registryClusterOption({ id: "rlark-ksyun-gpu", name: "金山云西北宁夏裸金属集群" });
  assert.equal(after.id, before.id);
  assert.equal(after.id, "ksyun-gpu");
  assert.equal(after.name, "金山云西北宁夏裸金属集群");
});

test("a cluster without a display name is shown by its ID", () => {
  assert.deepEqual(registryClusterOption({ id: "rlark-local-dev-gpu" }), { id: "local-dev-gpu", name: "rlark-local-dev-gpu" });
});
