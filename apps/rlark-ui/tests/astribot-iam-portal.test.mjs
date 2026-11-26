import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import { createIamPortal, portalPath } from "../dist/test/astribot/iam/portal.js";

function storage(values = {}) {
  const map = new Map(Object.entries(values));
  return {
    getItem: (key) => map.get(key) ?? null,
    setItem: (key, value) => void map.set(key, String(value)),
    removeItem: (key) => void map.delete(key),
  };
}

const settle = async () => {
  for (let turn = 0; turn < 20; turn += 1) await new Promise((resolve) => setImmediate(resolve));
};

function adminSession(role = "admin", token = "opaque-session-token", username = "admin") {
  return { ok: true, token, role, expiresAt: "2026-12-02T00:00:00Z", user: { username } };
}

function consoleTab({ path = "/admin/iam", stored = {}, session, sessionStatus, fetchOverride } = {}) {
  const store = storage(stored);
  Object.defineProperty(globalThis, "sessionStorage", { value: store, configurable: true, writable: true });
  const calls = [];
  const navigations = [];
  const listeners = new Map();
  const native = { click: 0, unauthorized: 0 };
  const counters = { covers: [], broadcasts: 0 };
  let nativeSignIn = () => {};
  let pageShow = () => {};
  let logoutBroadcast = () => {};
  const [pathname, ...query] = path.split("?");
  const portal = createIamPortal({
    location: {
      pathname,
      search: query.length ? `?${query.join("?")}` : "",
      replace: (url) => void navigations.push(url),
    },
    fetch: async (url, init = {}) => {
      calls.push({ url, method: init.method ?? "GET", headers: init.headers ?? {}, signal: init.signal });
      if (fetchOverride) return fetchOverride(url, init, calls);
      if (url === "/api/iam/session") {
        return new Response(session ? JSON.stringify(session) : "{}", { status: sessionStatus ?? (session ? 200 : 401) });
      }
      return new Response(null, { status: 204 });
    },
    intercept: (type, listener) => {
      const values = listeners.get(type) ?? [];
      values.push(listener);
      listeners.set(type, values);
    },
    cover: (message) => counters.covers.push(message),
    onNativeSignIn: (callback) => { nativeSignIn = callback; },
    onPageShow: (callback) => { pageShow = callback; },
    onLogoutBroadcast: (callback) => { logoutBroadcast = callback; },
    broadcastLogout: () => { counters.broadcasts += 1; },
  });

  function registerNativeHandlers() {
    listeners.get("click").push((event) => { native.click += 1; });
    listeners.get("rlark:unauthorized").push((event) => { native.unauthorized += 1; });
  }

  function dispatch(type, event) {
    for (const listener of listeners.get(type) ?? []) {
      listener(event);
      if (event.immediateStopped) break;
    }
    return event;
  }

  return {
    portal, calls, navigations, store, native, counters,
    bringUpNativeSignIn: () => nativeSignIn(),
    pageshow: (persisted) => pageShow(persisted),
    receiveLogoutBroadcast: () => logoutBroadcast(),
    registerNativeHandlers,
    click: (target) => dispatch("click", keptEvent(target)),
    unauthorized: () => dispatch("rlark:unauthorized", keptEvent()),
  };
}

function keptEvent(target) {
  return {
    target,
    defaultPrevented: false,
    propagationStopped: false,
    immediateStopped: false,
    preventDefault() { this.defaultPrevented = true; },
    stopPropagation() { this.propagationStopped = true; },
    stopImmediatePropagation() { this.immediateStopped = true; this.propagationStopped = true; },
  };
}

function nativeButton(label) {
  return { closest: (selector) => (selector === ".account-popover button" ? { textContent: label } : null) };
}

test("cookie session synchronization replaces an older token and role", async () => {
  const tab = consoleTab({
    stored: { "rlark-auth-token": "old-token", "rlark-auth-role": "user", "rlark-user-auth": "1", "rlark-user-name": "old" },
    session: adminSession("admin", "fresh-token", "fresh-admin"),
  });
  await tab.portal.enter();
  assert.equal(tab.calls[0].url, "/api/iam/session");
  assert.equal(tab.store.getItem("rlark-auth-token"), "fresh-token");
  assert.equal(tab.store.getItem("rlark-auth-role"), "admin");
  assert.equal(tab.store.getItem("rlark-admin-auth"), "1");
  assert.equal(tab.store.getItem("rlark-admin-user-name"), "fresh-admin");
  assert.equal(tab.store.getItem("rlark-user-auth"), null);
  assert.deepEqual(tab.navigations, []);
});

test("a session of another console role replaces stale role markers", async () => {
  const tab = consoleTab({
    stored: { "rlark-admin-auth": "1", "rlark-admin-user-name": "someone" },
    session: adminSession("operator", "operator-token", "operator1"),
  });
  await tab.portal.enter();
  assert.equal(tab.store.getItem("rlark-auth-role"), "user");
  assert.equal(tab.store.getItem("rlark-user-auth"), "1");
  assert.equal(tab.store.getItem("rlark-user-name"), "operator1");
  assert.equal(tab.store.getItem("rlark-admin-auth"), null);
});

test("a held token is cleared and sent to the portal when cookie sync returns 401", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "old-token" }, sessionStatus: 401 });
  void tab.portal.enter();
  await settle();
  assert.equal(tab.store.getItem("rlark-auth-token"), null);
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
});

test("an unreachable IAM keeps a held token and permits rendering", async () => {
  const tab = consoleTab({
    stored: { "rlark-auth-token": "offline-token" },
    fetchOverride: async () => { throw new TypeError("offline"); },
  });
  await tab.portal.enter();
  assert.equal(tab.store.getItem("rlark-auth-token"), "offline-token");
  assert.deepEqual(tab.navigations, []);
});

test("an unreachable IAM without a token goes to the portal", async () => {
  const tab = consoleTab({ fetchOverride: async () => { throw new TypeError("offline"); } });
  void tab.portal.enter();
  await settle();
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
  assert.equal(tab.counters.covers.length, 1);
});

test("other unsuccessful responses keep an existing token but leave without one", async () => {
  const withToken = consoleTab({ stored: { "rlark-auth-token": "held" }, sessionStatus: 503 });
  await withToken.portal.enter();
  assert.equal(withToken.store.getItem("rlark-auth-token"), "held");
  assert.deepEqual(withToken.navigations, []);

  const withoutToken = consoleTab({ sessionStatus: 503 });
  void withoutToken.portal.enter();
  await settle();
  assert.deepEqual(withoutToken.navigations, ["/login?next=%2Fadmin%2Fiam"]);
});

test("the way back after signing in is the whole address, as one query parameter", () => {
  assert.equal(portalPath("/"), "/login?next=%2F");
  assert.equal(portalPath("/admin/iam?tab=users&select=u_1"), "/login?next=%2Fadmin%2Fiam%3Ftab%3Dusers%26select%3Du_1");
});

for (const [name, logoutResult] of [
  ["204", async () => new Response(null, { status: 204 })],
  ["401", async () => new Response("{}", { status: 401 })],
  ["503", async () => new Response("{}", { status: 503 })],
  ["network error", async () => { throw new TypeError("offline"); }],
  ["timeout", async (_url, init) => new Promise((resolve, reject) => {
    init.signal.addEventListener("abort", () => reject(new DOMException("Aborted", "AbortError")), { once: true });
    if (init.signal.aborted) reject(new DOMException("Aborted", "AbortError"));
  })],
]) {
  test(`sign out with ${name} reaches portal once, clears storage and covers content`, async () => {
    const tab = consoleTab({
      stored: { "rlark-auth-token": "session-token", "rlark-auth-role": "admin" },
      session: adminSession("admin", "session-token"),
      fetchOverride: (url, init) => url === "/api/iam/logout" ? logoutResult(url, init) : Promise.resolve(new Response(JSON.stringify(adminSession("admin", "session-token")), { status: 200 })),
    });
    await tab.portal.enter();
    await tab.portal.signOut("/admin/iam");
    assert.equal(tab.calls.find((call) => call.url === "/api/iam/logout").headers.Authorization, "Bearer session-token");
    assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
    assert.equal(tab.store.getItem("rlark-auth-token"), null);
    assert.equal(tab.counters.covers.length, 1);
    assert.equal(tab.counters.broadcasts, 1);
  });
}

test("without AbortSignal.timeout the AbortController fallback times out and cleans its timer", async () => {
  const originalTimeout = AbortSignal.timeout;
  const originalSetTimeout = globalThis.setTimeout;
  const originalClearTimeout = globalThis.clearTimeout;
  let cleared = false;
  try {
    AbortSignal.timeout = undefined;
    globalThis.setTimeout = (callback) => { callback(); return 17; };
    globalThis.clearTimeout = (timer) => { cleared = timer === 17; };
    const tab = consoleTab({
      stored: { "rlark-auth-token": "session-token" },
      session: adminSession(),
      fetchOverride: async (url, init) => {
        if (url === "/api/iam/logout") {
          assert.equal(init.signal.aborted, true);
          throw new DOMException("Aborted", "AbortError");
        }
        return new Response(JSON.stringify(adminSession()), { status: 200 });
      },
    });
    await tab.portal.enter();
    await tab.portal.signOut("/");
    assert.equal(cleared, true);
    assert.deepEqual(tab.navigations, ["/login?next=%2F"]);
  } finally {
    AbortSignal.timeout = originalTimeout;
    globalThis.setTimeout = originalSetTimeout;
    globalThis.clearTimeout = originalClearTimeout;
  }
});

test("native sign-out is intercepted before the native click listener", async () => {
  const tab = consoleTab({
    stored: { "rlark-auth-token": "session-token" },
    session: adminSession(),
  });
  await tab.portal.enter();
  tab.registerNativeHandlers();
  const event = tab.click(nativeButton("Sign out"));
  assert.equal(event.defaultPrevented, true);
  assert.equal(event.immediateStopped, true);
  assert.equal(tab.native.click, 0);
  await settle();
  assert.equal(tab.calls.filter((call) => call.url === "/api/iam/logout").length, 1);
});

test("other account-menu actions reach the native click listener", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "session-token" }, session: adminSession() });
  await tab.portal.enter();
  tab.registerNativeHandlers();
  const event = tab.click(nativeButton("个人资料"));
  assert.equal(event.defaultPrevented, false);
  assert.equal(tab.native.click, 1);
  assert.deepEqual(tab.navigations, []);
});

test("unauthorized dispatch is stopped before the native handler and leaves only once", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "session-token" }, session: adminSession() });
  await tab.portal.enter();
  tab.registerNativeHandlers();
  const event = tab.unauthorized();
  assert.equal(event.immediateStopped, true);
  assert.equal(tab.native.unauthorized, 0);
  tab.unauthorized();
  await settle();
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
  assert.equal(tab.counters.covers.length, 1);
});

test("a native sign-in leaves for the portal once, and a role mismatch returns home", async () => {
  const signedOut = consoleTab({ sessionStatus: 401 });
  void signedOut.portal.enter();
  await settle();
  const previousCount = signedOut.navigations.length;
  signedOut.bringUpNativeSignIn();
  assert.equal(signedOut.navigations.length, previousCount);

  const otherRole = consoleTab({ stored: { "rlark-auth-token": "user-token", "rlark-auth-role": "user" }, session: adminSession("user") });
  await otherRole.portal.enter();
  otherRole.bringUpNativeSignIn();
  assert.deepEqual(otherRole.navigations, ["/"]);
  assert.equal(otherRole.counters.covers.length, 1);
});

test("bfcache restoration synchronizes the session and leaves on 401", async () => {
  const refreshed = consoleTab({
    stored: { "rlark-auth-token": "old" },
    session: adminSession("admin", "restored-token", "restored"),
  });
  await refreshed.portal.enter();
  refreshed.pageshow(true);
  await settle();
  assert.equal(refreshed.store.getItem("rlark-auth-token"), "restored-token");
  assert.equal(refreshed.calls.filter((call) => call.url === "/api/iam/session").length, 2);

  const expired = consoleTab({ stored: { "rlark-auth-token": "old" }, sessionStatus: 401 });
  void expired.portal.enter();
  await settle();
  expired.pageshow(true);
  await settle();
  assert.deepEqual(expired.navigations, ["/login?next=%2Fadmin%2Fiam"]);
});

test("a logout broadcast leaves this tab without another logout or broadcast", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "session-token" }, session: adminSession() });
  await tab.portal.enter();
  tab.receiveLogoutBroadcast();
  tab.receiveLogoutBroadcast();
  await settle();
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
  assert.equal(tab.calls.filter((call) => call.url === "/api/iam/logout").length, 0);
  assert.equal(tab.counters.broadcasts, 0);
});

test("the observer cannot navigate a second time after startup already left", async () => {
  const tab = consoleTab({ sessionStatus: 401 });
  void tab.portal.enter();
  await settle();
  const previousCount = tab.navigations.length;
  tab.bringUpNativeSignIn();
  await settle();
  assert.equal(tab.navigations.length, previousCount);
});

test("the console renders only after the portal lets it", async () => {
  const main = await readFile(new URL("../src/main.tsx", import.meta.url), "utf8");
  assert.match(main, /import \{ enterIamPortal \} from "\.\/astribot\/iam\/portal";/);
  assert.match(main, /enterIamPortal\(\)\.then\(\(\) => \{\s*ReactDOM\.createRoot\([^\n]*\)\.render\(/);
  assert.equal(main.match(/ReactDOM\.createRoot/g).length, 1);

  const dockerfile = await readFile(new URL("../../../astribot/release/rlark-ui.Dockerfile", import.meta.url), "utf8");
  assert.match(dockerfile, /^ENV VITE_IAM_PORTAL=1$/m);

  const page = await readFile(new URL("../src/astribot/iam/IAMPage.tsx", import.meta.url), "utf8");
  assert.match(page, /if \(iamPortalEnabled\(\)\) \{\s+await signOutOfConsole\(window\.location\.pathname\);/);
  assert.match(page, /void returnToPortal\(window\.location\.pathname \+ window\.location\.search\)/);
  assert.match(page, /!isMockMode && !sessionChecked \? <div className="iam-state" role="status"/);
});
