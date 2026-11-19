import assert from "node:assert/strict";
import test from "node:test";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { ApiError } from "../dist/test/api.js";
import { clustersApi, nodesApi } from "../dist/test/backend.js";
import { ClusterName } from "../dist/test/astribot/components/ClusterName.js";
import { NodeName } from "../dist/test/astribot/components/NodeName.js";
import { installMockBackend } from "../dist/test/mockBackend.js";
import {
  clusterLabel,
  displayNameError,
  displayNameMessage,
  displayNameRefusal,
  nodeLabel,
  nodeSearchNames,
} from "../dist/test/astribot/lib/displayName.js";

const node = (name, displayName, namespace = "rlark-baidu-gpu") => ({
  metadata: { name, namespace },
  spec: displayName === undefined ? {} : { displayName },
});

test("a node shows its display name, else its Kubernetes name", () => {
  assert.equal(nodeLabel(node("instance-gt4l6j5v-1", "百度GPU-1")), "百度GPU-1");
  assert.equal(nodeLabel(node("instance-gt4l6j5v-1", "")), "instance-gt4l6j5v-1");
  assert.equal(nodeLabel({ metadata: { name: "s1u-02-x86" } }), "s1u-02-x86");
});

test("NodeName adds the Kubernetes name only when it differs from the label", () => {
  const renamed = renderToStaticMarkup(React.createElement(NodeName, { node: node("instance-gt4l6j5v-1", "百度GPU-1") }));
  assert.match(renamed, /百度GPU-1/);
  assert.match(renamed, /<span class="real-name"> · instance-gt4l6j5v-1<\/span>/);
  const plain = renderToStaticMarkup(React.createElement(NodeName, { node: node("s1u-02-x86") }));
  assert.equal(plain, '<span class="node-name">s1u-02-x86</span>');
  const same = renderToStaticMarkup(React.createElement(NodeName, { node: node("s1u-02-x86", "s1u-02-x86") }));
  assert.doesNotMatch(same, /real-name/);
});

test("a namespace shows as its cluster's name, and as itself when no single cluster owns it", () => {
  const baidu = { id: "rlark-baidu-gpu", agentNamespace: "rlark-baidu-gpu", name: "百度GPU", displayName: "百度GPU" };
  assert.equal(clusterLabel([baidu], "rlark-baidu-gpu"), "百度GPU");
  assert.equal(clusterLabel([baidu], "rlark-other"), "rlark-other");
  assert.equal(clusterLabel([baidu, { ...baidu, id: "twin", name: "twin" }], "rlark-baidu-gpu"), "rlark-baidu-gpu");
  assert.equal(clusterLabel([{ ...baidu, agentNamespaceConflict: true }], "rlark-baidu-gpu"), "rlark-baidu-gpu");
});

test("a node is found by its display name, its Kubernetes name and its cluster's name", () => {
  const searchable = nodeSearchNames(node("instance-gt4l6j5v-1", "百度GPU-1"), "百度GPU").toLowerCase();
  for (const query of ["百度gpu-1", "gt4l6j5v", "rlark-baidu-gpu", "百度gpu"]) assert.ok(searchable.includes(query), query);
});

test("a new display name follows the Gateway's rule: format first, then uniqueness against names and display names", () => {
  const clusters = [
    { id: "rlark-baidu-gpu", name: "rlark-baidu-gpu", displayName: "百度GPU" },
    { id: "rlark-local-dev-gpu", name: "rlark-local-dev-gpu" },
  ];
  assert.equal(displayNameError("  ", "rlark-local-dev-gpu", clusters), undefined);
  assert.equal(displayNameError("办公室GPU", "rlark-local-dev-gpu", clusters), undefined);
  assert.equal(displayNameError(" 百度GPU ", "rlark-baidu-gpu", clusters), undefined);
  assert.equal(displayNameError("rlark-baidu-gpu", "rlark-baidu-gpu", clusters), undefined);
  assert.equal(displayNameError("bad name", "rlark-local-dev-gpu", clusters)?.status, 400);
  assert.equal(displayNameError("a".repeat(65), "rlark-local-dev-gpu", clusters)?.status, 400);
  assert.equal(displayNameError("a".repeat(64), "rlark-local-dev-gpu", clusters), undefined);
  assert.deepEqual(displayNameError("百度GPU", "rlark-local-dev-gpu", clusters), {
    status: 409,
    message: "display name already used by rlark-baidu-gpu",
    usedBy: "rlark-baidu-gpu",
  });
  assert.equal(displayNameError("rlark-baidu-gpu", "rlark-local-dev-gpu", clusters)?.usedBy, "rlark-baidu-gpu");
});

test("display names are set through their own endpoints, keyed by the real names", async () => {
  globalThis.sessionStorage = { getItem: () => null, setItem() {}, removeItem() {} };
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), method: init.method, body: init.body });
    return new Response(JSON.stringify({ data: { id: "rlark-baidu-gpu" } }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  assert.deepEqual(await clustersApi.setDisplayName("rlark-baidu-gpu", "百度GPU"), { id: "rlark-baidu-gpu" });
  await nodesApi.setDisplayName("instance-gt4l6j5v-1", "rlark-baidu-gpu", "");
  assert.deepEqual(calls, [
    { input: "/api/v1/clusters/rlark-baidu-gpu/display-name", method: "PUT", body: JSON.stringify({ displayName: "百度GPU" }) },
    {
      input: "/api/v1/rlinf.io/v1alpha1/nodes/instance-gt4l6j5v-1/display-name?namespace=rlark-baidu-gpu",
      method: "PUT",
      body: JSON.stringify({ displayName: "" }),
    },
  ]);
});

test("the mock refuses a display name as the Gateway does, and the console says why in its language", async () => {
  globalThis.sessionStorage = { getItem: () => null, setItem() {}, removeItem() {} };
  globalThis.window = globalThis;
  globalThis.location = { origin: "http://console.test" };
  installMockBackend();
  // Node has no page to resolve the console's relative paths against.
  const mock = globalThis.fetch;
  globalThis.fetch = (input, init) => mock(new URL(String(input), location.origin), init);
  const refused = async (save) => {
    const error = await save().then(() => assert.fail("saved"), (e) => e);
    return displayNameRefusal(error);
  };

  const east = "云训练集群 East-A";
  const north = "云评测集群 North-B";
  assert.equal((await clustersApi.setDisplayName(east, "训练A")).name, "训练A");
  const usedCluster = await refused(() => clustersApi.setDisplayName(north, "训练A"));
  assert.deepEqual(usedCluster, { status: 409, usedBy: east });
  assert.equal(displayNameMessage(usedCluster, "cluster", true), `该名称已被集群 ${east} 使用（作为显示名或 ID）`);
  const badCluster = await refused(() => clustersApi.setDisplayName(north, "bad name"));
  assert.equal(badCluster.status, 400);
  assert.equal(displayNameMessage(badCluster, "cluster", false), "Invalid name: at most 64 Chinese/English letters, digits, -, _ and .");
  assert.equal((await clustersApi.setDisplayName(east, "")).name, east);

  assert.equal((await nodesApi.setDisplayName("gpu-cloud-01", east, "GPU-1")).spec.displayName, "GPU-1");
  const usedNode = await refused(() => nodesApi.setDisplayName("gpu-cloud-03", east, "GPU-1"));
  assert.deepEqual(usedNode, { status: 409, usedBy: `${east}/gpu-cloud-01` });
  assert.equal(displayNameMessage(usedNode, "node", true), `该名称已被节点 ${east}/gpu-cloud-01 使用（作为显示名或 Kubernetes 名）`);
  assert.deepEqual(await refused(() => nodesApi.setDisplayName("gpu-cloud-03", east, "gpu-cloud-01")), {
    status: 409,
    usedBy: `${east}/gpu-cloud-01`,
  });
  assert.equal((await refused(() => nodesApi.setDisplayName("gpu-cloud-03", east, "bad name"))).status, 400);
  assert.equal((await nodesApi.setDisplayName("gpu-cloud-01", east, "")).spec.displayName, undefined);
});

test("only the Gateway's 400 and 409 are display-name refusals", () => {
  assert.equal(displayNameRefusal(new Error("network down")), undefined);
  assert.equal(displayNameRefusal(new ApiError(500, "HTTP 500: boom")), undefined);
  assert.deepEqual(displayNameRefusal(new ApiError(409, "HTTP 409: display name already used by rlark-baidu-gpu")), {
    status: 409,
    usedBy: "rlark-baidu-gpu",
  });
});

test("ClusterName adds the cluster ID only when it differs from the label", () => {
  const baidu = { id: "rlark-baidu-gpu", agentNamespace: "rlark-baidu-gpu", name: "百度GPU", displayName: "百度GPU" };
  const renamed = renderToStaticMarkup(React.createElement(ClusterName, { clusters: [baidu], namespace: "rlark-baidu-gpu" }));
  assert.equal(renamed, '<span class="cluster-name">百度GPU<span class="real-name"> · rlark-baidu-gpu</span></span>');
  const plain = renderToStaticMarkup(
    React.createElement(ClusterName, { clusters: [{ ...baidu, name: baidu.id, displayName: undefined }], namespace: "rlark-baidu-gpu" }),
  );
  assert.equal(plain, '<span class="cluster-name">rlark-baidu-gpu</span>');
});
