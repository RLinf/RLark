import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("maps node status reasons to localized labels", async () => {
  const source = await read("../src/utils/nodes.ts");
  assert.match(source, /export function nodeStatusReasonLabel\(/);
  assert.match(source, /AgentLeaseActive: \{ zh: "节点代理心跳正常"/);
  assert.match(source, /AgentLeaseExpired: \{ zh: "节点代理心跳已过期"/);
  assert.match(source, /NodeAgentLeaseMissing: \{/);
  // Unknown reasons stay verbatim so new controller reasons remain visible.
  assert.match(source, /if \(!label\) return reason;/);
});

test("renders node status reasons through the localized label", async () => {
  const clusters = await read("../src/pages/Clusters.tsx");
  assert.match(clusters, /nodeStatusReasonLabel\(node\.status\.reason, zh\)/);
  assert.doesNotMatch(clusters, /\{node\.status\.reason\}/);

  const overview = await read("../src/pages/Overview.tsx");
  assert.match(overview, /nodeStatusReasonLabel\(n\.status\?\.reason, isZh\)/);
});

test("keeps the node kind filter key stable across languages", async () => {
  const jobs = await read("../src/pages/Jobs.tsx");
  assert.match(jobs, /function getNodeKind\(worker: WorkerItem\)/);
  assert.match(jobs, /function nodeKindLabel\(kind: string, zh: boolean\)/);
  assert.match(jobs, /set\.add\(getNodeKind\(w\)\)/);
  assert.match(jobs, /workerKindFilter\.includes\(getNodeKind\(worker\)\)/);
  assert.match(jobs, /nodeKindLabel\(getNodeKind\(worker\), zh\)/);
});

test("localizes addon phases reported by the controller", async () => {
  const addons = await read("../src/admin/Addons.tsx");
  assert.match(addons, /const phaseLabel = \(phase: string\)/);
  assert.match(addons, /if \(!zh\) return phase;/);
  assert.match(addons, /phaseLabel\(a\.status\?\.phase \|\| "Pending"\)/);
});

test("passes the language into the terminal and login pages", async () => {
  const terminal = await read("../src/components/terminal.tsx");
  assert.match(terminal, /const zh = lang === "zh";/);
  assert.match(terminal, /function workerStatusLabel\(/);

  const login = await read("../src/pages/Login.tsx");
  assert.match(login, /const zh = lang === "zh";/);
  assert.match(login, /rlark-logo-\$\{zh \? "zh" : "en"\}-light\.png/);

  const app = await read("../src/App.tsx");
  assert.match(app, /<UserLogin\s+lang=\{lang\}/);
  assert.match(app, /<TerminalPage[\s\S]*?lang=\{lang\}/);
});

test("uses a locale-aware list separator", async () => {
  for (const path of [
    "../src/pages/Clusters.tsx",
    "../src/pages/Storage.tsx",
    "../src/admin/AdminPage.tsx",
    "../src/components/NodeResourceBrowser.tsx",
  ]) {
    const source = await read(path);
    // 中文顿号只允许出现在语言分支里：同一处必须给出英文分隔符的对应写法，
    // 否则英文界面会直接显示中文标点。
    for (const match of source.matchAll(/join\("、"\)/g)) {
      const context = source.slice(match.index, match.index + 160);
      assert.match(context, /join\(", "\)/, `${path} @${match.index}`);
    }
  }
});

test("keeps the node worker table readable without horizontal scrolling", async () => {
  const styles = await readStyles();
  assert.match(
    styles,
    /\.node-worker-table-wrap \.node-worker-table \{[^}]*min-width: 860px/,
  );
  assert.match(
    styles,
    /\.node-worker-name \{[^}]*max-width: 200px[^}]*text-overflow: ellipsis/,
  );
});

test("truncates the long domain value in the pod sub-table", async () => {
  const styles = await readStyles();
  assert.match(styles, /\.pod-subtable-truncated \{[^}]*max-width: 260px/);
  assert.match(
    styles,
    /\.pod-subtable-truncated > span \{[^}]*text-overflow: ellipsis/,
  );

  const clusters = await read("../src/pages/Clusters.tsx");
  assert.match(clusters, /className="pod-subtable-truncated"/);
  assert.match(clusters, /title=\{worker\.domain\}/);
});
