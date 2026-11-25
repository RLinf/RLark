import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";
import { getRemovedLabelKeys } from "../dist/test/utils/nodeBatchMetadata.js";

test("detects labels removed in the detail editor", () => {
  assert.deepEqual(
    getRemovedLabelKeys({ zone: "a", team: "robot" }, { zone: "a" }),
    ["team"],
  );
});

test("node management select all only targets the current page", () => {
  const browserSource = readFileSync(
    new URL("../src/components/NodeResourceBrowser.tsx", import.meta.url),
    "utf8",
  );

  assert.match(browserSource, /const pagedKeys = pagedNodes\.map\(nodeKey\)/);
  assert.match(browserSource, /pagedKeys\.forEach/);
  assert.match(browserSource, /onSelectFiltered\?\.\(pagedKeys\)/);
  assert.doesNotMatch(
    browserSource,
    /const filteredKeys = filteredNodes\.map\(nodeKey\)/,
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
