import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { readStyles } from "./read-styles.mjs";

const createJobSource = await readFile(
  new URL("../src/pages/CreateJob.tsx", import.meta.url),
  "utf8",
);

test("only single-pod roles can be picked as the ray header", () => {
  assert.match(
    createJobSource,
    /const canBeHeader = \(role: string\) =>\s*roleReplicas\(role\) === 1;/,
  );
  assert.match(
    createJobSource,
    /const selectHeaderRole = \(role: string\) => \{\s*if \(!canBeHeader\(role\)\) return;/,
  );
  // Both the role step and the shared-config step must route through the guard
  // instead of calling setHeaderRole directly.
  assert.equal(
    createJobSource.match(/onClick=\{\(\) => selectHeaderRole\(entry\.id\)\}/g)
      ?.length,
    2,
  );
  assert.equal(
    createJobSource.match(/const headerAllowed = canBeHeader\(entry\.id\);/g)
      ?.length,
    2,
  );
  assert.equal(
    createJobSource.match(/disabled=\{!headerAllowed\}/g)?.length,
    1,
  );
});

test("an invalidated header falls back to the first single-pod role", () => {
  assert.match(
    createJobSource,
    /const effectiveHeader = canBeHeader\(selectedHeader\)\s*\? selectedHeader\s*: \(roles\.find\(\(entry\) => canBeHeader\(entry\.id\)\)\?\.id \?\? selectedHeader\);/,
  );
  // The validation that bounces the wizard back to a header-less step reads the
  // fallback value, so a recoverable state never deadlocks the wizard.
  assert.match(
    createJobSource,
    /if \(effectiveHeader && !canBeHeader\(effectiveHeader\)\)/,
  );
});

test("multi-pod roles explain why they cannot become the header", () => {
  assert.match(
    createJobSource,
    /Header 角色只能有一个 Pod，请将该角色副本数设为 1。/,
  );
  assert.match(
    createJobSource,
    /The header role must have exactly one pod\. Set its replicas to 1\./,
  );
  assert.match(createJobSource, /"Worker（多 Pod）"/);
  assert.match(createJobSource, /"Worker \(multi-pod\)"/);
});

test("dark mode keeps the role name readable while typing", async () => {
  const styles = await readStyles();
  assert.match(
    styles,
    /\.theme-dark \.role-edit-row input:focus \{[^}]*background: #0f1725;/,
  );
  assert.match(
    styles,
    /\.theme-dark \.role-edit-row input::placeholder \{[^}]*color: #7c8798;/,
  );
  assert.match(
    styles,
    /\.theme-dark \.role-edit-row small \{[^}]*color: #9ca8ba;/,
  );
});
