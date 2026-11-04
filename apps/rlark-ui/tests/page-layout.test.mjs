import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const readSource = (path) =>
  readFile(new URL(`../src/${path}`, import.meta.url), "utf8");

const [styles, overview, addons] = await Promise.all([
  readStyles(),
  readSource("pages/Overview.tsx"),
  readSource("admin/Addons.tsx"),
]);

test("header-adjacent pages use the shared page shell", () => {
  assert.match(overview, /page-content resource-page overview-page/);
  assert.equal(
    addons.match(/className="page-content resource-page addon-page"/g)?.length,
    3,
  );
});

test("the overview live job list navigates to the job detail", () => {
  assert.match(overview, /navigate\("jobs", job\.name\)/);
});

test("overview uses the same business worker filter as cluster management", () => {
  assert.match(
    overview,
    /const displayNodes = realNodes\.filter\(isBusinessWorkerNode\);/,
  );
});

test("page-specific classes do not override shared outer spacing", () => {
  assert.match(
    styles,
    /\.page-content \{[\s\S]*?padding: 28px 28px 38px;[\s\S]*?width: 100%;[\s\S]*?overflow-y: auto;[\s\S]*?overflow-x: hidden;/,
  );

  const adminNodeRule = styles.match(
    /\.admin-node-management-page \{([\s\S]*?)\}/,
  )?.[1];
  assert.ok(adminNodeRule);
  assert.doesNotMatch(adminNodeRule, /padding|margin/);

  const adminNodeHeadingRule = styles.match(
    /\.admin-node-page-heading \{([\s\S]*?)\}/,
  )?.[1];
  assert.ok(adminNodeHeadingRule);
  assert.doesNotMatch(
    adminNodeHeadingRule,
    /padding|margin|min-height|border|font-size/,
  );

  for (const pageClass of [
    "node-detail-page",
    "overview-page",
    "cluster-overview-page",
    "files-page",
    "cluster-detail-page",
    "storage-class-page",
    "storage-files-page",
    "storage-detail-page",
  ]) {
    const rule = styles.match(
      new RegExp(`\\.${pageClass} \\{([\\s\\S]*?)\\}`),
    )?.[1];
    if (!rule) continue;
    assert.doesNotMatch(
      rule,
      /(?:^|\s)(?:padding|margin|width|height|min-height|overflow(?:-x|-y)?):/,
    );
  }

  for (const headingClass of ["storage-files-hero", "storage-detail-hero"]) {
    const rule = styles.match(
      new RegExp(`\\.${headingClass} \\{([\\s\\S]*?)\\}`),
    )?.[1];
    assert.ok(rule);
    assert.doesNotMatch(
      rule,
      /padding|margin|min-height|border|font-size|background|box-shadow/,
    );
  }
});

test("the job tag column stays on the row's middle line", () => {
  // `td` inherits `vertical-align: middle` from the row, which is what keeps
  // the type chip and the status badge centred. The tag cell used to opt out
  // with `vertical-align: top` and floated above its neighbours.
  const rule = styles.match(/\.job-table-tag-cell \{([\s\S]*?)\}/)?.[1];
  assert.ok(rule);
  assert.doesNotMatch(rule, /vertical-align/);
});

const zIndexOf = (selector) => {
  const rules = [
    ...styles.matchAll(new RegExp(`\\${selector} \\{([\\s\\S]*?)\\}`, "g")),
  ];
  assert.ok(rules.length > 0, `${selector} rule not found`);
  const values = rules
    .map((rule) => rule[1].match(/z-index:\s*(-?\d+)/)?.[1])
    .filter((value) => value !== undefined)
    .map(Number);
  assert.ok(values.length > 0, `${selector} declares no z-index`);
  return Math.max(...values);
};

test("full-screen overlays stack above the sticky topbar", () => {
  const topbar = zIndexOf(".topbar");

  for (const selector of [
    ".modal-backdrop",
    ".addon-drawer-overlay",
    ".addon-drawer",
    ".log-terminal-fullscreen",
  ]) {
    assert.ok(
      zIndexOf(selector) > topbar,
      `${selector} must stack above .topbar (${topbar}) or it cannot cover the header`,
    );
  }

  assert.ok(zIndexOf(".modal-backdrop") > zIndexOf(".action-dropdown"));
  assert.ok(zIndexOf(".modal-backdrop") > zIndexOf(".log-terminal-fullscreen"));
});

test("modal backdrops do not need per-dialog z-index patches", () => {
  for (const selector of [
    ".restart-choice-backdrop",
    ".delete-job-backdrop",
    ".job-lifecycle-backdrop",
    ".storage-create-backdrop",
    ".job-tags-modal-backdrop",
  ]) {
    const rules = [
      ...styles.matchAll(new RegExp(`\\${selector} \\{([\\s\\S]*?)\\}`, "g")),
    ];
    for (const rule of rules) {
      assert.doesNotMatch(
        rule[1],
        /z-index/,
        `${selector} must inherit the shared .modal-backdrop stacking level`,
      );
    }
  }
});
