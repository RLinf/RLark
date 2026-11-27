import assert from "node:assert/strict";
import test from "node:test";
import {
  normalizeStorageClass,
  storageClassToForm,
} from "../dist/test/astribot/lib/storageClass.js";

test("mount endpoint survives API normalization and edit submission", () => {
  const item = normalizeStorageClass({
    name: "cloud",
    endpoint: "https://public.example.com",
    mount_endpoint: "https://internal.example.com",
  });
  const form = storageClassToForm(item);
  assert.equal(form.endpoint, "https://public.example.com");
  assert.equal(form.mount_endpoint, "https://internal.example.com");
  assert.equal(
    storageClassToForm(normalizeStorageClass({ ...form, mount_endpoint: "" }))
      .mount_endpoint,
    "",
  );
});

test("bucket prefix and VFS cache mode survive form normalization", () => {
  const item = normalizeStorageClass({
    name: "fleet-ks3",
    prefix: "ks3-regular",
    vfs_cache_mode: "off",
  });
  const form = storageClassToForm(item);
  assert.equal(form.prefix, "ks3-regular");
  assert.equal(form.vfs_cache_mode, "off");
  assert.equal(storageClassToForm().vfs_cache_mode, "writes");
  assert.equal(normalizeStorageClass({}).prefix, "");
});

test("mount profiles, overrides and effective options survive editing", () => {
  const raw = { name: "archive", mount_profile: "archive", mount_options: ["tpslimit=2"], effective_mount_options: ["vfs-cache-mode=off"], effective_backend_options: {chunk_size: "32Mi"}, vfs_cache_mode: "off" };
  const item = normalizeStorageClass(raw);
  const form = storageClassToForm(item);
  assert.equal(form.mount_profile, "archive");
  assert.deepEqual(form.mount_options, ["tpslimit=2"]);
  form.mount_options.push("low-level-retries=10");
  assert.deepEqual(item.mount_options, ["tpslimit=2"]);
  assert.deepEqual(item.effective_backend_options, {chunk_size: "32Mi"});
  assert.equal(storageClassToForm().mount_profile,"default");
});

test("a host-path storage submits only its path, clusters and file system type", async () => {
  const { storageClassPayload } = await import(
    "../dist/test/astribot/lib/storageClass.js"
  );
  const form = storageClassToForm(
    normalizeStorageClass({
      name: "kpfs",
      type: "hostpath",
      clusters: ["rlark-ksyun-gpu"],
      path: "/mnt/xc_kpfs/regular",
      fs_type: "kingstorage",
    }),
  );
  assert.equal(form.fs_type, "kingstorage");
  assert.deepEqual(storageClassPayload(form), {
    name: "kpfs",
    clusters: ["rlark-ksyun-gpu"],
    description: "",
    type: "hostpath",
    path: "/mnt/xc_kpfs/regular",
    fs_type: "kingstorage",
  });
});

test("an S3 storage submits endpoints only for its own clusters", async () => {
  const { storageClassPayload } = await import(
    "../dist/test/astribot/lib/storageClass.js"
  );
  const form = storageClassToForm(
    normalizeStorageClass({
      name: "ks3",
      clusters: ["rlark-ksyun-gpu", "rlark-robot"],
      cluster_mount_endpoints: { "rlark-ksyun-gpu": "https://internal.example.com" },
    }),
  );
  form.cluster_mount_endpoints["rlark-robot"] = "  ";
  form.cluster_mount_endpoints["removed-cluster"] = "https://x.example.com";
  const payload = storageClassPayload(form);
  assert.deepEqual(payload.cluster_mount_endpoints, {
    "rlark-ksyun-gpu": "https://internal.example.com",
  });
  assert.equal(payload.type, "s3");
  assert.equal("path" in payload, false);
  assert.equal("password" in payload, false);
});
