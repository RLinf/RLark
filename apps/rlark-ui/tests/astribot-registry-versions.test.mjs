import assert from "node:assert/strict";
import test from "node:test";
import {
  formatBytes,
  groupRegistryVersions,
  isRecent,
  splitRepository,
} from "../dist/test/astribot/lib/registryVersions.js";

const digest = (c) => `sha256:${c.repeat(64)}`;

test("tags sharing a digest become one version with the timestamped tag first", () => {
  const versions = groupRegistryVersions([
    { tag: "v0.2.0", digest: digest("a"), created: "2026-11-05T12:09:00Z", size: 10, platforms: ["linux/amd64"] },
    { tag: "20261105-2009-v0.2.0", digest: digest("a"), created: "2026-11-05T12:09:00Z", size: 10, platforms: ["linux/amd64"] },
    { tag: "latest", digest: digest("b"), created: "2026-11-11T07:12:00Z", size: 20 },
  ]);
  assert.equal(versions.length, 2);
  assert.deepEqual(versions[0].tags, ["latest"], "newest build first");
  assert.deepEqual(versions[1].tags, ["20261105-2009-v0.2.0", "v0.2.0"]);
  assert.deepEqual(versions[0].platforms, []);
});

test("unreadable tags stay separate and sort after known build times", () => {
  const versions = groupRegistryVersions([
    { tag: "broken", error: "unavailable" },
    { tag: "no-time", digest: digest("c") },
    { tag: "v1", digest: digest("d"), created: "2026-01-01T00:00:00Z" },
    { tag: "also-broken", digest: digest("d"), error: "unavailable" },
  ]);
  assert.deepEqual(versions.map((v) => v.tags[0]), ["v1", "no-time", "broken", "also-broken"]);
  assert.equal(versions.find((v) => v.tags[0] === "broken").unavailable, true);
  assert.equal(versions.find((v) => v.tags[0] === "also-broken").unavailable, true, "an error never merges into a readable version");
  assert.equal(versions.find((v) => v.tags[0] === "no-time").unavailable, false);
});

test("without a timestamped tag the longest tag is the main tag", () => {
  const [version] = groupRegistryVersions([
    { tag: "3.1", digest: digest("e") },
    { tag: "3.1.2-alpine", digest: digest("e") },
  ]);
  assert.deepEqual(version.tags, ["3.1.2-alpine", "3.1"]);
});

test("repository names split into namespace and short name", () => {
  assert.deepEqual(splitRepository("rlark/agent"), { namespace: "rlark", short: "agent" });
  assert.deepEqual(splitRepository("csi-rclone/sub/path"), { namespace: "csi-rclone", short: "sub/path" });
  assert.deepEqual(splitRepository("busybox"), { namespace: "", short: "busybox" });
});

test("sizes and freshness handle missing and invalid values", () => {
  assert.equal(formatBytes(undefined), "—");
  assert.equal(formatBytes(-1), "—");
  assert.equal(formatBytes(512), "512 B");
  assert.equal(formatBytes(61_234_567), "58.4 MB");
  assert.equal(formatBytes(2 * 1024 ** 3), "2.00 GB");
  const now = Date.parse("2026-11-12T08:00:00Z");
  assert.equal(isRecent("2026-11-12T01:00:00Z", now), true);
  assert.equal(isRecent("2026-11-10T01:00:00Z", now), false);
  assert.equal(isRecent("2026-11-13T01:00:00Z", now), false, "future times are not fresh");
});
