import assert from "node:assert/strict";
import test from "node:test";
import {
  findSSHKeyDuplicate,
  groupSSHUserKeys,
  resolveSSHKeyOwners,
  splitSSHPublicKeys,
} from "../dist/test/utils/sshKeys.js";
import { generateJobCRD } from "../dist/test/utils/job.js";

const aliceKey = "ssh-ed25519 AAAA-alice";
const bobKey = "ssh-rsa AAAA-bob";

test("splits all injected SSH public keys from a Job", () => {
  assert.deepEqual(splitSSHPublicKeys(` ${aliceKey}\r\n\n${bobKey} `), [
    aliceKey,
    bobKey,
  ]);
});

test("rejects duplicate SSH key names and contents", () => {
  const knownKeys = [
    { index: 0, user: "alice", public_key: aliceKey, added_at: "" },
  ];

  assert.equal(findSSHKeyDuplicate(" alice ", bobKey, knownKeys), "name");
  assert.equal(
    findSSHKeyDuplicate("bob", ` ${aliceKey} `, knownKeys),
    "publicKey",
  );
  assert.equal(findSSHKeyDuplicate("bob", bobKey, knownKeys), null);
});

test("groups duplicate public key records into one selectable option", () => {
  assert.deepEqual(
    groupSSHUserKeys([
      { index: 0, user: "alice", public_key: aliceKey, added_at: "" },
      { index: 0, user: "bob", public_key: bobKey, added_at: "" },
      { index: 1, user: "carol", public_key: aliceKey, added_at: "" },
      { index: 2, user: "empty", public_key: "  ", added_at: "" },
    ]),
    [
      {
        publicKey: aliceKey,
        owners: [
          { user: "alice", index: 0 },
          { user: "carol", index: 1 },
        ],
      },
      { publicKey: bobKey, owners: [{ user: "bob", index: 0 }] },
    ],
  );
});

test("resolves every owner for each injected key", () => {
  assert.deepEqual(
    resolveSSHKeyOwners(`${aliceKey}\n${bobKey}`, [
      { index: 1, user: "alice", public_key: aliceKey, added_at: "" },
      { index: 0, user: "bob", public_key: bobKey, added_at: "" },
    ]),
    [
      { publicKey: aliceKey, owners: [{ user: "alice", index: 1 }] },
      { publicKey: bobKey, owners: [{ user: "bob", index: 0 }] },
    ],
  );
});

test("keeps injected keys visible after their registered owner is removed", () => {
  assert.deepEqual(resolveSSHKeyOwners(aliceKey, []), [
    { publicKey: aliceKey, owners: [] },
  ]);
});

test("preserves every selected SSH public key in the submitted Job", () => {
  const sshPublicKey = `${aliceKey}\n${bobKey}`;
  const roleResources = {
    actor: {
      role: "actor",
      cluster: "cluster-a",
      nodeSelector: "",
      replicas: 1,
      cpu: "1",
      memory: "1Gi",
      gpu: "0",
      devices: [],
      image: "busybox:latest",
      prepareScript: "",
      envs: [],
      mounts: [],
    },
  };

  const job = generateJobCRD({
    name: "jo-ssh-keys",
    type: "Custom",
    headerRole: "actor",
    roles: ["actor"],
    roleResources,
    runScript: "echo ready",
    domain: "",
    sshPublicKey,
  });

  assert.equal(job.spec.sshPublicKey, sshPublicKey);
});
