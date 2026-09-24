import assert from "node:assert/strict";
import test from "node:test";

import {
  filesPath,
  hasTerminalSession,
  isAdminPath,
  parseAdminRoute,
} from "../dist/test/utils/route.js";

test("admin routes preserve shared file browser context", () => {
  assert.equal(filesPath("cluster a", "shared/data"), "/files/cluster%20a/shared%2Fdata");
  assert.equal(
    filesPath("cluster a", "shared/data", true),
    "/admin/files/cluster%20a/shared%2Fdata",
  );
  assert.deepEqual(parseAdminRoute("/admin/files/cluster%20a/shared%2Fdata"), {
    page: "files",
    sub: "cluster a/shared/data",
  });
});

test("admin path matching observes a route boundary", () => {
  assert.equal(isAdminPath("/admin"), true);
  assert.equal(isAdminPath("/admin/jobs"), true);
  assert.equal(isAdminPath("/administrator"), false);
});

test("terminal requires an access token", () => {
  const storage = (values) => ({ getItem: (key) => values[key] ?? null });
  assert.equal(hasTerminalSession(storage({ "rlark-auth-token": "token" })), true);
  assert.equal(hasTerminalSession(storage({ "rlark-user-auth": "1" })), false);
  assert.equal(hasTerminalSession(storage({})), false);
});
