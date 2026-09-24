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
