import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { compareRobotBody, robotModelMock } from "../dist/test/astribot/lib/robotBody.js";

const read = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("mock robot body fixtures cover matched, EEF mismatch, partial evidence and legacy model", () => {
  const model = robotModelMock("s1u");
  assert.ok(model?.body);
  assert.equal(compareRobotBody(model, "s1u-01-x86").status, "matched");
  const mismatch = compareRobotBody(model, "s1u-02-x86");
  assert.equal(mismatch.status, "mismatch");
  assert.deepEqual(mismatch.differences.map((item) => [item.section, item.name, item.kind]), [["endEffectors", "right_tool", "conflict"]]);
  const pending = compareRobotBody(model, "s1u-03-x86");
  assert.equal(pending.status, "unknown");
  assert.equal(pending.reason, "InsufficientEvidence");
  assert.equal(robotModelMock("s1u-legacy")?.body, undefined);
  assert.equal(compareRobotBody(robotModelMock("s1u-legacy"), "s1u-01-x86").reason, "ModelBodyUndefined");
});

test("RobotModel detail renders four body sections and a responsive comparison without API clients", async () => {
  const detail = await read("../src/astribot/pages/RobotModelDetail.tsx");
  const styles = await read("../src/astribot/pages/RobotModelDetail.css");
  assert.match(detail, /关节/);
  assert.match(detail, /末端执行器/);
  assert.match(detail, /传感器/);
  assert.match(detail, /本体参数/);
  assert.match(detail, /尚未发布本体定义/);
  assert.match(detail, /站点机器人实例对照/);
  assert.match(detail, /Insufficient evidence/);
  assert.doesNotMatch(detail, /fetch\(|robotModelsApi|siteNodesApi/);
  assert.match(styles, /\.robot-body-facts\s*\{[^}]*grid-template-columns/);
  assert.match(styles, /@media \(max-width: 560px\)/);
  assert.match(styles, /\.theme-dark \.robot-body-undefined/);
});

test("RobotModel route handles model detail and card shows body status plus expanded expected/observed rows", async () => {
  const routes = await read("../src/astribot/AdminPages.tsx");
  const card = await read("../src/astribot/pages/StationCard.tsx");
  const styles = await read("../src/astribot/pages/RobotBodyCard.css");
  assert.match(routes, /sub\.startsWith\("models\/"\)/);
  assert.match(card, /robot-body-card-badge/);
  assert.match(card, /型号定义 vs 实例上报/);
  assert.match(card, /kind === "missing"/);
  assert.match(card, /kind === "extra"/);
  assert.match(card, /kind === "conflict"/);
  assert.match(card, /证据不足/);
  assert.match(styles, /\.theme-dark \.robot-body-card-comparison/);
  assert.match(styles, /@media \(max-width: 650px\)/);
});
