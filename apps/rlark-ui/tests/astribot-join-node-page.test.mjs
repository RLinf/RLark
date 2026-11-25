import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const page = await readFile(new URL("../src/astribot/pages/JoinNode.tsx", import.meta.url), "utf8");
const styles = await readFile(new URL("../src/astribot/pages/JoinNode.css", import.meta.url), "utf8");
const dialog = await readFile(new URL("../src/astribot/pages/JoinMachinesDialog.tsx", import.meta.url), "utf8");
const dialogStyles = await readFile(new URL("../src/astribot/pages/JoinMachinesDialog.css", import.meta.url), "utf8");

test("the join page is a compact toolbar and one full-width workspace", () => {
  assert.match(page, /className="join-page-head"/);
  assert.match(page, /className="join-toolbar"/);
  assert.match(page, /className="join-method-tabs"/);
  assert.match(page, /className="join-body"/);
  assert.match(page, /className="join-main"/);
  assert.match(page, /className="join-rail-wrap"/);
  assert.match(page, /className="join-machine-grid"/);
  assert.match(page, /className="join-machine-identity"/);
  assert.match(page, /className="join-machine-location"/);
  assert.match(page, /className="join-machine-reach"/);
  assert.match(page, /className="join-machine-action"/);
  assert.match(page, /className={`join-selection-bar/);
  assert.doesNotMatch(page, /className="join-hero"/);
  assert.doesNotMatch(page, /className="join-stepper"/);
  assert.doesNotMatch(page, /join-step is-current/);
  assert.doesNotMatch(page, /join-machine-table/, "the old wide machine table is gone");
  assert.match(page, /<JoinMachinesDialog/);
});

test("the join page has real scanned and manual sources, and opens SSH records", () => {
  assert.match(page, /nodeJoinsApi\.list\(clusterID\)/);
  assert.match(page, /join-machine-filter/);
  assert.match(page, /添加普通电脑\/GPU/);
  assert.match(page, /onOpenSSHJoin/);
  assert.match(page, /打开 SSH 记录/);
});

test("the join page CSS is full width at desktop and rows become cards on phones", () => {
  for (const token of ["--accent", "--accent-bg", "--accent-ink", "--panel-soft", "--success", "--text-muted"]) {
    assert.ok(!styles.includes(`var(${token}`), `${token} is not defined in the production tokens`);
  }
  assert.match(styles, /\.join-node-page \.join-body\s*\{[^}]*grid-template-columns: minmax\(0, 1fr\)/);
  assert.match(styles, /@media \(min-width: 1600px\)[\s\S]*\.join-node-page \.join-body\s*\{[^}]*380px/);
  assert.match(styles, /\.join-node-page \.join-machine-card\s*\{[^}]*grid-template-columns:/);
  assert.match(styles, /\.join-node-page \.join-selection-bar\s*\{/);
  assert.match(styles, /\.join-method-layout\s*\{[^}]*minmax\(0, 1fr\) 320px/);
  assert.match(styles, /\.join-node-page \.join-machine-card\s*\{[^}]*border-bottom/);
});

test("the password dialog writes class credentials before admitting", () => {
  assert.match(dialog, /role="dialog"/);
  assert.match(dialog, /aria-modal="true"/);
  assert.match(dialog, /joinCredentialGroups\(machines, classes\)/);
  assert.match(dialog, /joinCredentialProblem\(groups, credentials, zh\)/);
  assert.ok(dialog.indexOf("machinesApi.setClassAccess") < dialog.indexOf("machinesApi.admit"), "credentials are written before admit");
  assert.match(dialog, /node_password: typed\.password/);
  assert.match(dialog, /cluster_id: clusterID/);
  assert.match(dialog, /type="password"/);
  assert.doesNotMatch(dialog, /window\.location|URLSearchParams|new URL\(/);
  assert.match(dialogStyles, /\.modal\.join-machines-modal/);
  assert.match(dialogStyles, /@media \(max-width: 620px\)/);
});
