import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { validateDiscovery, isSiteNetworkConflict, siteDisplay, siteOfCluster } from "../dist/test/astribot/lib/siteNetwork.js";
import { parseAdminRoute } from "../dist/test/utils/route.js";

const discovery = (ranges = ["192.168.31.0/24"], intervalSeconds = 30) => ({
  enabled: true, ranges, intervalSeconds, from: { clusterID: "office", nodeName: "cailiang" },
});

test("discovery form accepts /22-or-smaller IPv4 ranges and configured limits", () => {
  assert.equal(validateDiscovery(discovery(["10.11.12.0/22", "192.168.31.0/24"], 3600), false), "");
  assert.match(validateDiscovery(discovery(["10.11.8.0/21"]), false), /\/22 or shorter/);
  assert.match(validateDiscovery(discovery(["2001:db8::/32"]), false), /IPv4/);
  assert.match(validateDiscovery(discovery(Array.from({ length: 9 }, (_, index) => `10.0.${index}.0/24`)), false), /1 and 8/);
});

test("discovery form explains duplicate ranges and missing dial-out Hub in both languages", () => {
  assert.match(validateDiscovery(discovery(["10.0.0.0/24", "10.0.0.0/24"]), false), /unique/);
  assert.match(validateDiscovery({ ...discovery(), from: { clusterID: "", nodeName: "" } }, true), /请选择/);
});

test("unclassified discovery ports accept bounded unique ports", () => {
  assert.equal(validateDiscovery({ ...discovery(), includeUnclassified: true, sshPorts: [22, 2222] }, false), "");
  assert.match(validateDiscovery({ ...discovery(), includeUnclassified: true, sshPorts: [22, 22] }, false), /unique/);
  assert.match(validateDiscovery({ ...discovery(), includeUnclassified: true, sshPorts: [65536] }, false), /unique integers/);
});

test("admin stations network subroute parses and page exposes conflict recovery and fixed warnings", async () => {
  assert.deepEqual(parseAdminRoute("/admin/stations/network"), { page: "stations", sub: "network" });
  const source = await readFile(new URL("../src/astribot/pages/SiteNetworkSettings.tsx", import.meta.url), "utf8");
  assert.match(source, /isSiteNetworkConflict\(reason\)/);
  assert.match(source, /includeUnclassified/);
  assert.match(source, /sshPorts/);
  assert.match(source, /已重新加载最新配置/);
  assert.match(source, /自动识别依赖 NodeClass\.spec\.discovery\.hostKeyFingerprints/);
  assert.match(source, /Hub 直连的私网也会扫描/);
  assert.match(source, /已有 Machine 不会自动删除/);
  assert.match(source, /没有已入网 Hub 的站点不会扫描/);
  assert.equal(isSiteNetworkConflict({ status: 409 }), true);
  assert.equal(isSiteNetworkConflict({ status: 403 }), false);
});

test("a cluster's discovered machines are scoped to the site that lists it", () => {
  const view = { sites: [
    { name: "office", clusters: ["rlark-local-dev-gpu"] },
    { name: "lab", clusters: ["rlark-robot-s1u-01"] },
  ] };
  assert.equal(siteOfCluster(view, "rlark-local-dev-gpu"), "office");
  assert.equal(siteOfCluster(view, "rlark-robot-s1u-01"), "lab");
  assert.equal(siteOfCluster(view, "unknown"), "");
  assert.equal(siteOfCluster(undefined, "rlark-local-dev-gpu"), "");
  assert.equal(siteOfCluster(view, ""), "");
});

test("discovery workbench uses site tabs and a shared scanning rules panel", async () => {
  const source = await readFile(new URL("../src/astribot/pages/SiteNetworkSettings.tsx", import.meta.url), "utf8");
  assert.match(source, /<div\b[^>]*\bclassName="site-network-sites"[^>]*\brole="tablist"/);
  assert.doesNotMatch(source, /\bsite-network-cards\b/);

  const rulesPanels = [...source.matchAll(/<ul\b[^>]*\bclassName="site-network-rules\b[^"]*"[^>]*>([\s\S]*?)<\/ul>/g)];
  assert.equal(rulesPanels.length, 1, "The page has one shared scanning rules list");
  const rules = rulesPanels[0][1];
  for (const rule of [
    "自动识别依赖 NodeClass.spec.discovery.hostKeyFingerprints。未匹配的 SSH 设备只能作为未分类候选出现。",
    "Hub 直连的私网也会扫描；大于 /22 的网段只扫描 Hub 所在的 /22。",
    "多个站点网段重叠时可能出现重复 Machine。",
    "设置约 30 秒生效；已有 Machine 不会自动删除。",
    "没有已入网 Hub 的站点不会扫描。",
  ]) {
    assert.ok(rules.includes(JSON.stringify(rule)), rule);
  }
});

test("site labels follow cluster display names without losing their site identity", async () => {
  const names = { "office-cluster": "深圳办公集群", "robot-cluster": "S1U 调试集群" };
  assert.deepEqual(siteDisplay({ name: "office", clusters: ["office-cluster"] }, names), {
    title: "深圳办公集群", clusterNames: ["深圳办公集群"],
  });
  assert.deepEqual(siteDisplay({ name: "shared", clusters: ["office-cluster", "robot-cluster"] }, names), {
    title: "shared", clusterNames: ["深圳办公集群", "S1U 调试集群"],
  });
  assert.equal(siteDisplay({ name: "lab", clusters: ["unknown"] }, names).title, "unknown");
  assert.equal(siteDisplay({ name: "lab", clusters: [] }, names).title, "lab");

  const source = await readFile(new URL("../src/astribot/pages/SiteNetworkSettings.tsx", import.meta.url), "utf8");
  assert.match(source, /<strong title=\{display.title\}>\{display.title\}<\/strong>/);
  assert.match(source, /const active = site.name === selectedSite/);
  assert.match(source, /<p>\{selectedDisplay.title\}/);
  assert.match(source, /selectedSiteData.discovery\?\.enabled/);
  assert.match(source, /<details className="site-network-context"/);
  assert.match(source, /<details className="site-network-activity">/);
});

test("the discovery page is one workbench with site activity cards at the bottom", async () => {
  const source = await readFile(new URL("../src/astribot/pages/SiteNetworkSettings.tsx", import.meta.url), "utf8");
  const styles = await readFile(new URL("../src/astribot/pages/SiteNetworkSettings.css", import.meta.url), "utf8");
  assert.ok(!styles.includes("var(--amber)"), "--amber is not defined in production tokens");
  assert.match(styles, /\.site-network-page\s*\{[^}]*width: 100%[^}]*max-width: none/);
  assert.match(source, /className="site-network-board"/);
  assert.match(source, /className="site-network-sites" role="tablist"/);
  assert.match(source, /className="site-network-workspace"/);
  assert.match(source, /className="site-network-bottom"/);
  for (const card of ["最近扫描事件", "站点发现负载", "Astribot 机器人", "查看场站节点"]) assert.match(source, new RegExp(card));
  assert.match(source, /machinesApi\.list\(\)/);
  assert.match(source, /siteNodesApi\.list\(cluster\)/);
  assert.match(styles, /\.site-network-bottom\s*\{[^}]*grid-template-columns/);
  assert.match(styles, /\.site-network-rules-panel:not\(\[open\]\) > \.site-network-rules\s*[,{]/);
  assert.match(styles, /\.site-network-sites\s*\{[^}]*repeat\(auto-fit/);
  assert.match(source, /<footer className="site-network-editor-actions">[\s\S]+保存设置/);
  assert.match(source, /<details className="site-network-diagnostics">[\s\S]+site-network-health/);
  assert.match(source, /<details className="site-network-advanced"[\s\S]+draft\.includeUnclassified/);
  assert.match(source, /已开启[\s\S]+已关闭/);
  assert.match(source, /<SiteScan key=\{selectedSite\}/);
  assert.doesNotMatch(source, /扫描频率|扫描间隔|DiscoveryStale/);
  assert.match(styles, /\.site-network-field select,\s*\.site-network-range-row input\s*\{[^}]*background-color:/);
  assert.match(styles, /\.site-network-page\s*\{[^}]*align-content: start/);
});
