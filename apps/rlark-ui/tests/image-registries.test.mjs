import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

const page = readFileSync("src/pages/ImageRegistries.tsx", "utf8");
const admin = readFileSync("src/admin/AdminApp.tsx", "utf8");
const mock = readFileSync("src/mockBackend.ts", "utf8");
const backend = readFileSync("src/backend.ts", "utf8");

test("image registries use immutable IDs for routes and mutations", () => {
  assert.match(page, /key=\{item\.id\}/);
  assert.match(page, /onSelect\?\.\(item\.id\)/);
  assert.match(page, /imageRegistriesApi\.remove\(item\.id\)/);
  assert.match(backend, /image-registries\/\$\{encodeURIComponent\(id\)\}/);
  assert.match(admin, /selectedID=\{adminSub \|\| undefined\}/);
});

test("image registry forms submit explicit distribution scope", () => {
  assert.match(page, /"None" \| "Selected" \| "All"/);
  assert.match(page, /clusterSelection: \{ mode: "All", clusters: \[\] \}/);
  assert.match(page, /clusterSelection: form\.clusterSelection/);
  assert.match(page, /form\.password \? \{ password: form\.password \} : \{\}/);
});

test("mock backend implements ID-based image registry CRUD", () => {
  assert.match(mock, /path === "\/api\/v1\/image-registries"/);
  assert.match(mock, /item\.id === id/);
  assert.match(mock, /return json\(\{ ok: true \}, 202\)/);
});
