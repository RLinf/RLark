import assert from "node:assert/strict";
import test from "node:test";

import { AUTH_TOKEN_KEY, ApiError, storeAuthSession } from "../dist/test/api.js";
import { imageRegistriesApi } from "../dist/test/backend.js";
import { machinesApi, nodeJoinsApi, registryBrowseApi, robotModelsApi, siteNetworkApi, stationsApi } from "../dist/test/astribot/api.js";

test("Station APIs encode Cluster IDs, names, and pagination parameters", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ items: [], continue: "" }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  const controller = new AbortController();
  await stationsApi.list({ cluster_id: "cluster/a", limit: "200", continue: "token +/=" }, controller.signal);
  await stationsApi.listNodeOperations("cluster/a", controller.signal);
  await stationsApi.getNodeOperation("node-op/a", "cluster/a", controller.signal);
  assert.equal(calls[0].input, "/api/v1/station/stations?cluster_id=cluster%2Fa&limit=200&continue=token+%2B%2F%3D");
  assert.equal(calls[1].input, "/api/v1/station/node-operations?cluster_id=cluster%2Fa&limit=100");
  assert.equal(calls[2].input, "/api/v1/station/node-operations/node-op%2Fa?cluster_id=cluster%2Fa");
  assert.ok(calls.filter((call) => call.init.signal).every((call) => call.init.signal === controller.signal));
});

test("site network reads and updates only the selected site's discovery route", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ resourceVersion: "17", sites: [], members: [], nodeClasses: [], status: { conditions: [] } }), {
      status: 200, headers: { "Content-Type": "application/json" },
    });
  };
  await siteNetworkApi.get();
  const input = { resourceVersion: "17", discovery: { enabled: true, ranges: ["192.168.31.0/24"], intervalSeconds: 45, from: { clusterID: "office", nodeName: "hub/a" } } };
  await siteNetworkApi.updateDiscovery("lab/a", input);
  assert.equal(calls[0].input, "/api/v1/station/site-network");
  assert.equal(calls[0].init.method, undefined);
  assert.equal(calls[1].input, "/api/v1/station/site-network/sites/lab%2Fa/discovery");
  assert.equal(calls[1].init.method, "PUT");
  assert.deepEqual(JSON.parse(calls[1].init.body), input);
});

test("machine discovery lists only the selected cluster's site when one is given", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ items: [], classes: {} }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };
  await machinesApi.list("office");
  await machinesApi.list();
  assert.equal(calls[0].input, "/api/v1/station/machines?site=office");
  assert.equal(calls[1].input, "/api/v1/station/machines");
});

test("an SSH join sends the login once to its own route and reads the reason of a refusal", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ error: "10.0.0.9:22 did not accept the user ops with this password", reason: "LoginRejected" }), {
      status: 422,
      headers: { "Content-Type": "application/json" },
    });
  };
  const error = await nodeJoinsApi.create({ cluster_id: "office", address: "10.0.0.9", user: "ops", password: "pw" }).catch((reason) => reason);
  assert.equal(calls.length, 1);
  assert.equal(calls[0].input, "/api/v1/station/node-joins");
  assert.equal(calls[0].init.method, "POST");
  assert.deepEqual(JSON.parse(calls[0].init.body), { cluster_id: "office", address: "10.0.0.9", user: "ops", password: "pw" });
  assert.ok(error instanceof ApiError);
  assert.equal(error.status, 422);
  assert.equal(error.body.reason, "LoginRejected");
  // A refused login is the machine's answer, not the console's: the session stays.
  storeAuthSession("token", "admin");
  await nodeJoinsApi.create({ cluster_id: "office", address: "10.0.0.9", user: "ops", password: "pw" }).catch(() => undefined);
  assert.equal(sessionStorage.getItem(AUTH_TOKEN_KEY), "token");
});

test("scanned server NodeJoin carries candidate identity without a relay address", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ data: { metadata: { name: "office.box" } } }), { status: 201, headers: { "Content-Type": "application/json" } });
  };
  const input = { cluster_id: "office", machine_name: "candidate-1", machine_type: "Server", address: "192.168.1.20", port: 2222, host_key: "SHA256:pinned", host_key_algorithm: "ssh-ed25519", user: "ops", password: "login", sudo_password: "sudo" };
  await nodeJoinsApi.create(input);
  assert.equal(calls[0].input, "/api/v1/station/node-joins");
  assert.equal(calls[0].init.method, "POST");
  assert.deepEqual(JSON.parse(calls[0].init.body), input);
  assert.equal(JSON.parse(calls[0].init.body).relay_address, undefined);
});

test("RobotModel management lists and deletes only through GET and DELETE", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return init?.method === "DELETE"
      ? new Response(null, { status: 204 })
      : new Response(JSON.stringify({ items: [] }), { status: 200, headers: { "Content-Type": "application/json" } });
  };
  await robotModelsApi.list();
  await robotModelsApi.delete("model/a");
  assert.equal(calls[0].input, "/api/v1/station/robot-models");
  assert.equal(calls[0].init.method, undefined);
  assert.equal(calls[1].input, "/api/v1/station/robot-models/model%2Fa");
  assert.equal(calls[1].init.method, "DELETE");
});

test("registry browse API encodes parameters and forwards cancellation", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(
      JSON.stringify({ items: [], next: "" }),
      {
        status: 200,
        headers: { "Content-Type": "application/json" },
      },
    );
  };
  const controller = new AbortController();
  await imageRegistriesApi.list(controller.signal);
  await registryBrowseApi.repositories("id/a", "a/b", controller.signal);
  await registryBrowseApi.tags("id/a", "a/b", "v1", controller.signal);
  await registryBrowseApi.password("id/a", controller.signal);
  assert.equal(
    calls[1].input,
    "/api/v1/image-registries/id%2Fa/repositories?last=a%2Fb",
  );
  assert.equal(
    calls[2].input,
    "/api/v1/image-registries/id%2Fa/tags?repository=a%2Fb&last=v1",
  );
  assert.equal(calls[0].input, "/api/v1/image-registries");
  assert.equal(calls[3].input, "/api/v1/image-registries/id%2Fa/password");
  assert.ok(calls.every((call) => call.init.signal === controller.signal));
});

test("registry browsing propagates authorization and upstream failures and can recover", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  let status = 403;
  globalThis.fetch = async () =>
    new Response(
      JSON.stringify(
        status === 200
          ? { items: ["image"], next: "" }
          : { message: "unavailable" },
      ),
      { status, headers: { "Content-Type": "application/json" } },
    );
  await assert.rejects(
    () => imageRegistriesApi.list(),
    (error) => error instanceof ApiError && error.status === 403,
  );
  status = 502;
  await assert.rejects(
    () => registryBrowseApi.repositories("registry"),
    (error) => error instanceof ApiError && error.status === 502,
  );
  status = 200;
  assert.deepEqual(await registryBrowseApi.repositories("registry"), {
    items: ["image"],
    next: "",
  });
});

function storage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
}
