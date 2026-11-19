import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const source = await readFile(
  new URL("../src/pages/ClusterManagement.tsx", import.meta.url),
  "utf8",
);

test("delisted clusters expose restore and use the delist action", () => {
  assert.match(
    source,
    /cluster\.phase === "Delisted" \? \([\s\S]{0,500}handleClusterAction\(cluster, "restore"\)/,
  );
  assert.match(source, /handleClusterAction\(cluster, "delist"\)/);
  assert.doesNotMatch(source, /handleClusterAction\(cluster\.id, "offline"\)/);
});
