import assert from "node:assert/strict";
import test from "node:test";

import { imageReferenceHasWhitespace } from "../dist/test/utils/imageReference.js";

test("accepts valid container image references", () => {
  assert.equal(
    imageReferenceHasWhitespace("docker.io/library/ubuntu:22.04"),
    false,
  );
  assert.equal(imageReferenceHasWhitespace("registry/team/image@sha256:abc"), false);
});

test("rejects spaces and other whitespace in image references", () => {
  for (const value of [
    "docker.io/library/ubuntu: 22.04",
    " docker.io/library/ubuntu:22.04",
    "docker.io/library/ubuntu:22.04 ",
    "docker.io/library/ubuntu:\t22.04",
    "docker.io/library/ubuntu:\n22.04",
  ]) {
    assert.equal(imageReferenceHasWhitespace(value), true, value);
  }
});
