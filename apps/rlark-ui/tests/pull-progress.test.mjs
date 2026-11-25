import assert from "node:assert/strict";
import test from "node:test";
import { pullProgressLabel } from "../dist/test/utils/pullProgress.js";

test("localizes fixed node-agent pull messages", () => {
  assert.equal(
    pullProgressLabel("extracting layers", "pulling", true),
    "解压镜像层",
  );
  assert.equal(
    pullProgressLabel("image already present", "completed", true),
    "镜像已存在",
  );
  assert.equal(
    pullProgressLabel("image pulled successfully", "completed", true),
    "镜像拉取完成",
  );
  assert.equal(
    pullProgressLabel("waiting for pull to start", "pulling", true),
    "等待开始拉取",
  );
  assert.equal(
    pullProgressLabel("waiting for image to be pulled", "pulling", true),
    "等待镜像拉取完成",
  );
});

test("localizes messages that carry variables", () => {
  assert.equal(
    pullProgressLabel("downloading: 1.2GB / 3.4GB (12 blobs)", "pulling", true),
    "下载中：1.2GB / 3.4GB（12 个 blob）",
  );
  assert.equal(
    pullProgressLabel("downloading: 1.2GB (12 blobs)", "pulling", true),
    "下载中：1.2GB（12 个 blob）",
  );
  assert.equal(
    pullProgressLabel(
      "waiting for pull to start (5s elapsed)",
      "pulling",
      true,
    ),
    "等待开始拉取（已等待 5 秒）",
  );
  assert.equal(
    pullProgressLabel(
      "waiting for image to be pulled (5s elapsed)",
      "pulling",
      true,
    ),
    "等待镜像拉取完成（已等待 5 秒）",
  );
});

test("keeps unrecognized messages for troubleshooting", () => {
  assert.equal(
    pullProgressLabel("custom agent message", "pulling", true),
    "custom agent message",
  );
});

test("localizes status when the message is empty", () => {
  assert.equal(pullProgressLabel(undefined, "pulling", true), "拉取中");
  assert.equal(pullProgressLabel("", "completed", true), "已完成");
  assert.equal(pullProgressLabel(undefined, "failed", true), "失败");
  assert.equal(pullProgressLabel(undefined, "unknown", true), "unknown");
});

test("keeps backend values unchanged for the English UI", () => {
  assert.equal(
    pullProgressLabel("extracting layers", "pulling", false),
    "extracting layers",
  );
  assert.equal(pullProgressLabel(undefined, "pulling", false), "pulling");
  assert.equal(pullProgressLabel(undefined, undefined, false), "");
});
