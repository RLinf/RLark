import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const readSource = (path) =>
  readFile(new URL(`../src/${path}`, import.meta.url), "utf8");

const [styles, domains, workflows, registries, sshKeys, addons] =
  await Promise.all([
    readStyles(),
    readSource("pages/Domains.tsx"),
    readSource("pages/Workflows.tsx"),
    readSource("pages/ImageRegistries.tsx"),
    readSource("pages/SSHKeys.tsx"),
    readSource("admin/Addons.tsx"),
  ]);

test("resource table action columns use the shared compact layout", () => {
  for (const source of [domains, workflows, registries, sshKeys]) {
    assert.match(source, /className="table-actions-col"/);
    assert.match(source, /className="row-actions"/);
    assert.match(
      source,
      /<td[\s\S]{0,160}className="table-actions-col"[\s\S]{0,160}<div className="row-actions">/,
    );
  }

  assert.match(styles, /\.table-panel \.row-actions \.icon-button/);
  assert.match(styles, /width: 30px;/);
  assert.match(styles, /border-radius: 10px;/);
});

test("addon text actions use the shared table action button", () => {
  assert.match(addons, /className="table-action-button"/);
  assert.match(addons, /className="table-action-button danger"/);
  assert.doesNotMatch(addons, /padding: "4px 10px"/);
});
