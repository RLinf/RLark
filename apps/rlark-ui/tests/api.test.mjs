import assert from "node:assert/strict";
import test from "node:test";

import {
  AUTH_ROLE_KEY,
  AUTH_TOKEN_KEY,
  clearAuthSession,
  hasAuthSession,
  storeAuthSession,
  UNAUTHORIZED_EVENT,
  request,
} from "../dist/test/api.js";
import {
  apiReferenceApi,
  domainsApi,
  nodesApi,
  systemConfigApi,
} from "../dist/test/backend.js";

function storage() {
  const values = new Map();
  return {
    getItem: (key) => values.get(key) ?? null,
    setItem: (key, value) => values.set(key, String(value)),
    removeItem: (key) => values.delete(key),
  };
}

test("request attaches the JWT and preserves headers", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  storeAuthSession("jwt-token", "admin");
  globalThis.fetch = async (_input, init) => {
    assert.equal(init.headers.get("Authorization"), "Bearer jwt-token");
    assert.equal(init.headers.get("Content-Type"), "application/json");
    return new Response(null, { status: 200 });
  };

  const response = await request("/api/v1/clusters", {
    headers: { "Content-Type": "application/json" },
  });
  assert.equal(response.status, 200);
  assert.equal(hasAuthSession("admin"), true);
});

test("request clears the session after a 401", async () => {
  globalThis.sessionStorage = storage();
  let eventType = "";
  globalThis.window = { dispatchEvent: (event) => (eventType = event.type) };
  sessionStorage.setItem(AUTH_TOKEN_KEY, "expired");
  sessionStorage.setItem(AUTH_ROLE_KEY, "user");
  globalThis.fetch = async () => new Response(null, { status: 401 });

  await assert.rejects(() => request("/api/v1/clusters"));
  assert.equal(sessionStorage.getItem(AUTH_TOKEN_KEY), null);
  assert.equal(eventType, UNAUTHORIZED_EVENT);
  clearAuthSession();
});

test("resource APIs encode paths, query values, and JSON bodies", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  const calls = [];
  globalThis.fetch = async (input, init) => {
    calls.push({ input: String(input), init });
    return new Response(JSON.stringify({ items: [] }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  await nodesApi.list({ labelSelector: "rlark.io/cluster-id=a/b" });
  await domainsApi.create({ metadata: { name: "domain-a" } });

  assert.equal(
    calls[0].input,
    "/api/v1/rlinf.io/v1alpha1/nodes?labelSelector=rlark.io%2Fcluster-id%3Da%2Fb",
  );
  assert.equal(calls[1].init.method, "POST");
  assert.equal(calls[1].init.headers.get("Content-Type"), "application/json");
  assert.equal(calls[1].init.body, '{"metadata":{"name":"domain-a"}}');
});

test("system config API shares reads and refreshes its cache after update", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  let calls = 0;
  globalThis.fetch = async (_input, init) => {
    calls += 1;
    const config =
      init?.method === "PUT"
        ? { ssh: { jumpHost: "new.example.com", jumpPort: "22" } }
        : { ssh: { jumpHost: "old.example.com", jumpPort: "22" } };
    return new Response(JSON.stringify(config), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  };

  const [first, second] = await Promise.all([
    systemConfigApi.get({ refresh: true }),
    systemConfigApi.get(),
  ]);
  assert.equal(calls, 1);
  assert.equal(first.ssh.jumpHost, "old.example.com");
  assert.equal(second.ssh.jumpHost, "old.example.com");

  await systemConfigApi.update({ ssh: { jumpHost: "new.example.com" } });
  const cached = await systemConfigApi.get();
  assert.equal(calls, 2);
  assert.equal(cached.ssh.jumpHost, "new.example.com");
});

test("API reference is loaded from Gateway", async () => {
  globalThis.sessionStorage = storage();
  globalThis.window = { dispatchEvent() {} };
  globalThis.fetch = async (input) => {
    assert.equal(String(input), "/api/v1/api-reference");
    return new Response(
      JSON.stringify({
        title: { zh: "接口参考", en: "API Reference" },
        description: { zh: "接口", en: "APIs" },
        sections: [],
      }),
      { status: 200, headers: { "Content-Type": "application/json" } },
    );
  };

  const reference = await apiReferenceApi.get();
  assert.equal(reference.title.en, "API Reference");
});
