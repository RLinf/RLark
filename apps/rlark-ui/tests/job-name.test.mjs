import assert from "node:assert/strict";
import test from "node:test";

import {
  isValidJobDisplayName,
  isValidRoleName,
  JOB_DISPLAY_NAME_MAX_LENGTH,
  ROLE_NAME_MAX_LENGTH,
} from "../dist/test/utils/job.js";

test("accepts valid job display names", () => {
  for (const value of [
    "任务一",
    "job-1",
    "train_llm_v2",
    "model.eval.01",
    "RL-训练_任务.001",
    "a".repeat(JOB_DISPLAY_NAME_MAX_LENGTH),
  ]) {
    assert.equal(isValidJobDisplayName(value), true, value);
  }
});

test("rejects empty or overlong job display names", () => {
  assert.equal(isValidJobDisplayName(""), false);
  assert.equal(
    isValidJobDisplayName("a".repeat(JOB_DISPLAY_NAME_MAX_LENGTH + 1)),
    false,
  );
});

test("rejects unsupported characters in job display names", () => {
  for (const value of [
    "任务 名称",
    "job:name",
    "a/b",
    "邮箱@test",
    "emoji🚀",
    "逗号，名称",
  ]) {
    assert.equal(isValidJobDisplayName(value), false, value);
  }
});

test("accepts valid role names with the same rules", () => {
  assert.equal(ROLE_NAME_MAX_LENGTH, 64);
  for (const value of [
    "Actor",
    "rollout-1",
    "env_worker",
    "模型.评估",
    "a".repeat(ROLE_NAME_MAX_LENGTH),
  ]) {
    assert.equal(isValidRoleName(value), true, value);
  }
});

test("rejects invalid role names", () => {
  assert.equal(isValidRoleName(""), false);
  assert.equal(isValidRoleName("a".repeat(ROLE_NAME_MAX_LENGTH + 1)), false);
  for (const value of ["Actor 2", "role:name", "a/b", "邮箱@test"]) {
    assert.equal(isValidRoleName(value), false, value);
  }
});
