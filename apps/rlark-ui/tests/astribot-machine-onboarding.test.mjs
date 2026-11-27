import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("scanned onboarding selects Robot or Server and uses the scanned identity for SSH", async () => {
  const dialog = await read("../src/astribot/pages/JoinMachinesDialog.tsx");
  assert.match(dialog, /value="Robot"/);
  assert.match(dialog, /value="Server"/);
  assert.match(dialog, /machine_name: selectedMachine\.name/);
  assert.match(dialog, /host_key: selectedMachine\.host_key/);
  assert.match(dialog, /host_key_algorithm: selectedMachine\.host_key_algorithm/);
  assert.match(dialog, /直连失败，正在通过站点中继重试/);
  assert.match(dialog, /改用入网码/);
  assert.doesNotMatch(dialog, /relay_endpoint|relay_address/i);
});

test("join-code robot model choices are loaded from the RobotModel API", async () => {
  const page = await read("../src/astribot/pages/JoinNode.tsx");
  assert.match(page, /robotModelsApi\.list\(\)/);
  assert.match(page, /robotModels\.map/);
  assert.match(page, /node_class: form\.nodeClass, machine: \{ category: "Robot" as const, model \}/);
  assert.doesNotMatch(page, /node_class: "s1u"/);
});

test("RobotModel page exposes list and guarded deletion only", async () => {
  const page = await read("../src/astribot/pages/RobotModels.tsx");
  assert.match(page, /robotModelsApi\.list/);
  assert.match(page, /robotModelsApi\.delete/);
  assert.match(page, /references/);
  assert.doesNotMatch(page, /robotModelsApi\.(create|update)/);
});
