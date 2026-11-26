import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(
  new URL("../src/pages/ClusterManagement.tsx", import.meta.url),
  "utf8",
);

test("delisted clusters expose restore and delete, and use the delist action", () => {
  assert.match(
    source,
    /cluster\.phase === "Delisted" \? \([\s\S]*?handleClusterAction\(cluster, "restore"\)[\s\S]*?handleClusterAction\(cluster, "delete"\)[\s\S]*?\) : \(/,
  );
  assert.match(source, /handleClusterAction\(cluster, "delist"\)/);
  assert.doesNotMatch(source, /handleClusterAction\(cluster\.id, "offline"\)/);
});

test("drain requires Cordoned and running jobs while delist and delete require no running jobs", () => {
  assert.match(
    source,
    /const drainDisabled =\s*actionPending \|\| cluster\.phase !== "Cordoned"/,
  );
  assert.match(
    source,
    /const delistOrDeleteDisabled =\s*drainDisabled \|\| cluster\.runningJobs !== 0/,
  );
  assert.match(source, /const noRunningJobs = cluster\.runningJobs === 0/);
  assert.match(
    source,
    /disabled=\{drainDisabled \|\| noRunningJobs\}[\s\S]{0,900}handleClusterAction\(cluster, "drain"\)/,
  );
  assert.match(
    source,
    /cluster\.phase === "Cordoned" &&\s*noRunningJobs\s*\?[\s\S]{0,120}"无运行中任务"/,
  );
  assert.equal(source.match(/disabled=\{delistOrDeleteDisabled\}/g)?.length, 2);
});
