// Our pages under src/astribot/ that take the place of official ones pass the
// official tests of the pages they replace: each official test file below
// runs once more with the official paths pointed at our files.
import assert from "node:assert/strict";
import { mkdir, readFile, writeFile } from "node:fs/promises";
import test from "node:test";

const ours = {
  "pages/ClusterManagement.tsx": "astribot/pages/ClusterManagement.tsx",
  "pages/Clusters.tsx": "astribot/pages/Clusters.tsx",
  "pages/Storage.tsx": "astribot/pages/Storage.tsx",
  "components/NodeResourceBrowser.tsx": "astribot/components/NodeResourceBrowser.tsx",
};

const officialTests = [
  "cluster-lifecycle-actions",
  "form-modal-dismiss",
  "job-actions",
  "node-batch-metadata",
  "node-worker-localization",
  "refresh-feedback",
];

// The copies sit beside the official tests' own directory, so every relative
// path in them still resolves.
const copies = new URL("../dist/astribot-pages/", import.meta.url);
await mkdir(copies, { recursive: true });
for (const name of officialTests) {
  let source = await readFile(new URL(`${name}.test.mjs`, import.meta.url), "utf8");
  for (const [official, own] of Object.entries(ours)) source = source.replaceAll(official, own);
  const copy = new URL(`${name}.mjs`, copies);
  await writeFile(copy, source.replaceAll('"../', '"../../').replaceAll("`../", "`../../").replaceAll('"./', '"../../tests/'));
  await import(copy.href);
}

// clipboard.test.mjs fixes the official import path of copyText, so what it
// asks of the node page is asked of ours here.
test("our node page copies the SSH command through copyText and shows it copied only when that worked", async () => {
  const source = await readFile(new URL("../src/astribot/pages/Clusters.tsx", import.meta.url), "utf8");
  assert.doesNotMatch(source, /navigator\.clipboard/);
  assert.match(source, /import \{ copyText \} from "\.\.\/\.\.\/utils\/clipboard"/);
  assert.equal(source.match(/if \(!\(await copyText\(sshCommand\)\)\) return;/g)?.length, 2);
});

test("the consoles mount our pages in place of the official ones", async () => {
  const read = (path) => readFile(new URL(`../src/${path}`, import.meta.url), "utf8");
  const [app, adminApp, adminPage] = await Promise.all([read("App.tsx"), read("admin/AdminApp.tsx"), read("admin/AdminPage.tsx")]);
  for (const page of ["Clusters", "ClusterManagement", "Storage"]) assert.match(app, new RegExp(`from "\\./astribot/pages/${page}"`));
  for (const page of ["ClusterManagement", "Storage"]) assert.match(adminApp, new RegExp(`from "\\.\\./astribot/pages/${page}"`));
  assert.match(adminPage, /from "\.\.\/astribot\/pages\/Clusters"/);
  assert.match(adminPage, /from "\.\.\/astribot\/components\/NodeResourceBrowser"/);
  for (const source of [app, adminApp, adminPage]) assert.doesNotMatch(source, /from "\.\.?\/(pages\/(Clusters|ClusterManagement|Storage)|components\/NodeResourceBrowser)"/);
});
