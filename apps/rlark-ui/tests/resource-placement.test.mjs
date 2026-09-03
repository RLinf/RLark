import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const pickerSource = await readFile(
  new URL("../src/components/ResourcePlacementPicker.tsx", import.meta.url),
  "utf8",
);

test("lists nodes without discovered device resources", () => {
  assert.match(pickerSource, /else if \(resources\.length === 0\)/);
  assert.match(pickerSource, /model: "__undiscovered_device__"/);
  assert.match(pickerSource, /resourceKey: ""/);
  assert.match(pickerSource, /"未分类"/);
  assert.match(pickerSource, /"未发现设备"/);
});

test("shows a truthful placeholder when a device resource has no model", () => {
  assert.match(pickerSource, /const deviceModel = getNodeDeviceModel\(fullNode\)/);
  assert.match(pickerSource, /resourceKey: deviceModel \? deviceKey : ""/);
  assert.match(pickerSource, /configured: Boolean\(deviceModel\)/);
  assert.match(pickerSource, /"未设置设备型号"/);
  assert.match(
    pickerSource,
    /仅按未设置设备型号的节点调度，不申请设备资源/,
  );
  assert.match(pickerSource, /resourceAmount} × \$\{displayModel}/);
});

test("unconfigured devices constrain nodes without requesting fake resources", () => {
  assert.match(pickerSource, /!resource\.configured \|\| resource\.free >= nextPerWorker/);
  assert.match(pickerSource, /candidates\[0\]\?\.resourceKey/);
  assert.match(pickerSource, /disabled=\{isUnconfiguredDevice\}/);
  assert.match(
    pickerSource,
    /仅按未分类节点调度，不申请未发现的设备资源/,
  );
});

test("edit mode restores the original resource and selected nodes", () => {
  assert.match(pickerSource, /nodeSelector: string/);
  assert.match(pickerSource, /parseNodeSelectorStr\(nodeSelector\)/);
  assert.match(pickerSource, /restoresAutomaticMode/);
  assert.match(pickerSource, /placementMode \?\?/);
  assert.match(pickerSource, /onPlacementModeChange\?\.\("manual"\)/);
  assert.match(pickerSource, /selectedNames\.size === matchingNames\.length/);
  assert.match(pickerSource, /setMode\("manual"\)/);
  assert.match(pickerSource, /selectedNames\.has\(name\)/);
  assert.match(pickerSource, /initializedRef\.current = true/);
});

test("new placement defaults to manual node selection", () => {
  assert.match(
    pickerSource,
    /useState<Mode>\(placementMode \?\? "manual"\)/,
  );
});

test("node cards keep long node names readable", () => {
  assert.match(pickerSource, /title=\{name\}/);
  assert.match(pickerSource, /className="placement-node-meta"/);
  assert.match(pickerSource, /className="placement-node-state"/);
});
