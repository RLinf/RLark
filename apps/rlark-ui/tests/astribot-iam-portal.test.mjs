import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

import {
  addressPath,
  createIamPortal,
  portalHomeRedirect,
  portalPath,
  resolvePortalView,
} from "../dist/test/astribot/iam/portal.js";

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

function consoleTab({ path = "/admin/iam", hash = "", stored = {}, session, sessionStatus, fetchOverride } = {}) {
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
      hash,
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

test("an unreachable IAM is an error state, never the stored session", async () => {
  const tab = consoleTab({
    stored: { "rlark-auth-token": "offline-token" },
    fetchOverride: async () => { throw new TypeError("offline"); },
  });
  const state = await tab.portal.enter();
  assert.equal(state.status, "error");
  assert.equal(state.reason, "unreachable");
  // The held token is kept for a retry but never authorizes the console.
  assert.equal(tab.store.getItem("rlark-auth-token"), "offline-token");
  assert.deepEqual(tab.navigations, []);
});

test("an unreachable IAM without a token is an error state, not a redirect", async () => {
  const tab = consoleTab({ fetchOverride: async () => { throw new TypeError("offline"); } });
  const state = await tab.portal.enter();
  assert.equal(state.status, "error");
  assert.deepEqual(tab.navigations, []);
  assert.equal(tab.counters.covers.length, 0);
});

test("an unsuccessful session check is an error whatever is stored", async () => {
  const withToken = consoleTab({ stored: { "rlark-auth-token": "held" }, sessionStatus: 503 });
  const held = await withToken.portal.enter();
  assert.equal(held.status, "error");
  assert.equal(held.reason, "http_503");
  assert.equal(withToken.store.getItem("rlark-auth-token"), "held");
  assert.deepEqual(withToken.navigations, []);

  const withoutToken = consoleTab({ sessionStatus: 503 });
  const empty = await withoutToken.portal.enter();
  assert.equal(empty.status, "error");
  assert.equal(empty.reason, "http_503");
  assert.deepEqual(withoutToken.navigations, []);
});

test("an incomplete session response is an error, not a signed-out console", async () => {
  for (const body of ["{}", '{"token":"token-only"}', '{"token":"token-only","user":{}}']) {
    const tab = consoleTab({ fetchOverride: async () => new Response(body, { status: 200 }) });
    const state = await tab.portal.enter();
    assert.equal(state.status, "error", body);
    assert.equal(state.reason, "incomplete");
    assert.equal(tab.store.getItem("rlark-auth-token"), null);
    assert.deepEqual(tab.navigations, []);
  }
});

test("a retry re-reads the session and replaces the error state", async () => {
  let attempt = 0;
  const tab = consoleTab({
    fetchOverride: async () => {
      attempt += 1;
      if (attempt === 1) return new Response("upstream failure", { status: 500 });
      return new Response(JSON.stringify(adminSession("admin", "retry-token", "admin")), { status: 200 });
    },
  });
  const failed = await tab.portal.enter();
  assert.equal(failed.status, "error");
  assert.equal(failed.reason, "http_500");
  const recovered = await tab.portal.retry();
  assert.equal(recovered.status, "authenticated");
  assert.equal(tab.store.getItem("rlark-auth-token"), "retry-token");
  assert.equal(tab.store.getItem("rlark-auth-role"), "admin");
  assert.deepEqual(tab.navigations, []);
});

test("the address carried to the portal keeps its query and hash", async () => {
  const tab = consoleTab({
    path: "/admin/iam?tab=users",
    hash: "#audit",
    stored: { "rlark-auth-token": "old" },
    sessionStatus: 401,
  });
  await tab.portal.enter();
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam%3Ftab%3Dusers%23audit"]);
});

test("the startup state alone decides which view the console renders", () => {
  const admin = { status: "authenticated", role: "admin", userName: "admin" };
  const user = { status: "authenticated", role: "user", userName: "operator" };
  assert.deepEqual(resolvePortalView({ status: "loading" }, "/admin"), { kind: "pending" });
  assert.deepEqual(resolvePortalView({ status: "unauthenticated" }, "/admin"), { kind: "pending" });
  assert.deepEqual(resolvePortalView({ status: "error", reason: "unreachable" }, "/admin"), {
    kind: "error",
    reason: "unreachable",
  });
  assert.deepEqual(resolvePortalView(user, "/admin"), { kind: "home" });
  assert.deepEqual(resolvePortalView(user, "/admin/iam"), { kind: "home" });
  assert.deepEqual(resolvePortalView(user, "/administrator"), { kind: "console" });
  assert.deepEqual(resolvePortalView(user, "/"), { kind: "console" });
  assert.deepEqual(resolvePortalView(admin, "/"), { kind: "console" });
  assert.deepEqual(resolvePortalView(admin, "/admin/iam"), { kind: "admin" });

  // No portal view is a form: the only screens are pending, error, console,
  // admin and home.
  const views = ["pending", "error", "console", "admin", "home"];
  for (const state of [{ status: "loading" }, { status: "unauthenticated" }, { status: "error", reason: "x" }, user, admin]) {
    for (const path of ["/", "/admin", "/admin/iam", "/terminal"]) {
      assert.ok(views.includes(resolvePortalView(state, path).kind));
    }
  }
});

test("only a role without admin access at an admin address is sent home", () => {
  const user = { status: "authenticated", role: "user", userName: "operator" };
  const admin = { status: "authenticated", role: "admin", userName: "admin" };
  assert.equal(portalHomeRedirect(resolvePortalView(user, "/admin/iam")), "/");
  assert.equal(portalHomeRedirect(resolvePortalView(admin, "/admin/iam")), undefined);
  assert.equal(portalHomeRedirect(resolvePortalView(admin, "/")), undefined);
  assert.equal(portalHomeRedirect(resolvePortalView({ status: "unauthenticated" }, "/admin")), undefined);
});

test("the address carried to the portal is the whole location", () => {
  assert.equal(addressPath({ pathname: "/admin/iam", search: "?tab=users", hash: "#audit" }), "/admin/iam?tab=users#audit");
  assert.equal(addressPath({ pathname: "/jobs", search: "" }), "/jobs");
  assert.equal(portalPath("/admin/iam?tab=users#audit"), "/login?next=%2Fadmin%2Fiam%3Ftab%3Dusers%23audit");
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
  assert.match(main, /import \{\s+enterIamPortal,\s+iamPortalEnabled,\s+type PortalAuthState,\s+\} from "\.\/astribot\/iam\/portal";/);
  // A status placeholder is rendered first, so no native form can flash while
  // the session check is still in flight.
  assert.match(main, /render\(\{ status: "loading" \}\);\s+void enterIamPortal\(\)\.then\(render\);/);
  assert.match(main, /<App iamState=\{iamState\} \/>/);
  assert.equal(main.match(/ReactDOM\.createRoot/g).length, 1);

  const dockerfile = await readFile(new URL("../../../astribot/release/rlark-ui.Dockerfile", import.meta.url), "utf8");
  assert.match(dockerfile, /^ENV VITE_IAM_PORTAL=1$/m);

  const page = await readFile(new URL("../src/astribot/iam/IAMPage.tsx", import.meta.url), "utf8");
  assert.match(page, /if \(iamPortalEnabled\(\)\) \{\s+await signOutOfConsole\(addressPath\(window\.location\)\);/);
  assert.match(page, /void returnToPortal\(addressPath\(window\.location\)\)/);
  assert.match(page, /!isMockMode && !sessionChecked \? <div className="iam-state" role="status"/);
});

test("the IAM page keeps its credentials form out of the portal build", async () => {
  const page = await readFile(new URL("../src/astribot/iam/IAMPage.tsx", import.meta.url), "utf8");
  const portal = await readFile(new URL("../src/astribot/iam/portal.ts", import.meta.url), "utf8");

  // renderLogin() refuses to build the form behind the portal, whatever
  // /api/iam/me answered (403, 5xx or a network failure).
  assert.match(page, /const renderLogin = \(\) => \{\s+\/\/[^\n]*\n\s+if \(iamPortalEnabled\(\)\) return renderPortalSession\(\);/);
  // The failed-session screen offers a retry and the portal entrance.
  assert.match(page, /const renderPortalSession = \(\) => \(\s+<div className="iam-state iam-portal-state" role="alert">/);
  assert.match(page, /<button type="button" onClick=\{retrySession\}>\{text\(zh, "retry"\)\}<\/button>/);
  assert.match(page, /<a href="\/login">/);
  // A signed-out visit is rendered by the gate alone.
  assert.match(page, /const renderSessionGate = \(\) => \(iamPortalEnabled\(\) \? renderPortalSession\(\) : renderLogin\(\)\);/);
  assert.match(page, /: renderSessionGate\(\)\}/);
  // Retrying re-reads the IAM identity behind the page.
  assert.match(page, /const retrySession = \(\) => \{\s+setApiError\(""\);\s+setSessionChecked\(false\);\s+setSessionAttempt\(\(attempt\) => attempt \+ 1\);/);
  assert.match(page, /\}, \[sessionAttempt\]\);/);
  // The observation net watches both native forms.
  assert.match(portal, /const nativeSignIn = "\.admin-login-page, \.iam-login-screen";/);
});
