import assert from "node:assert/strict";
import test from "node:test";
import { imageReferenceTag } from "../dist/test/utils/imageReference.js";

test("running image tags exclude the registry port and image digest", () => {
  assert.equal(imageReferenceTag("registry:5443/fleet/device:v1.2.3"), "v1.2.3");
  assert.equal(imageReferenceTag("registry:5443/fleet/device:v1.2.3@sha256:abc"), "v1.2.3");
  assert.equal(imageReferenceTag("registry:5443/fleet/device@sha256:abc"), undefined);
  assert.equal(imageReferenceTag("registry:5443/fleet/device"), undefined);
  assert.equal(imageReferenceTag("busybox"), undefined);
  assert.equal(imageReferenceTag("busybox:latest"), "latest");
});
