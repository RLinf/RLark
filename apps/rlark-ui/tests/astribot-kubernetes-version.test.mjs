import test from "node:test";
import assert from "node:assert/strict";
import {
  kubernetesVersionsOf,
  kubernetesVersionText,
} from "../dist/test/astribot/lib/kubernetesVersion.js";

function node(name, annotations = {}) {
  return { metadata: { name, annotations } };
}

function read(kubelet) {
  return { "station.rlark.io/kubernetes-version": "v1.36.6", "station.rlark.io/kubelet-version": kubelet };
}

test("a cluster shows its API server's version, the nodes whose kubelet differs and a failed read", () => {
  const nodes = [
    node("s1u-04-x86"),
    node("alexfu", read("v1.36.6")),
    node("s1u-01-x86", read("v1.33.13")),
    node("s1u-02-x86", read("v1.33.13")),
    node("s1u-03-x86", read("v1.35.0")),
  ];
  const versions = kubernetesVersionsOf(nodes);
  assert.equal(kubernetesVersionText(versions, true), "v1.36.6 · 2 节点 v1.33.13 · 1 节点 v1.35.0");
  assert.equal(kubernetesVersionText(versions, false), "v1.36.6 · 2 nodes v1.33.13 · 1 node v1.35.0");
  assert.equal(kubernetesVersionText(kubernetesVersionsOf(nodes.slice(0, 2)), true), "v1.36.6");

  const unreadable = { ...read("v1.36.6"), "station.rlark.io/kubernetes-version-error": "member unreachable" };
  assert.equal(kubernetesVersionText(kubernetesVersionsOf([node("10-0-2-57", unreadable)]), true), "v1.36.6 · 未能读取");
  assert.equal(kubernetesVersionText(kubernetesVersionsOf([node("10-0-2-57", unreadable)]), false), "v1.36.6 · unreadable");
  assert.equal(kubernetesVersionText(kubernetesVersionsOf([node("s1u-04-x86")]), true), "—");
  assert.equal(kubernetesVersionText(undefined, true), "—");
});
