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

function adminSession(role = "admin") {
  return { ok: true, token: "opaque-session-token", role, expiresAt: "2026-12-02T00:00:00Z", user: { username: "admin" } };
}

// A console tab behind the portal: the page at `path`, what its sessionStorage and IAM hold.
function consoleTab({ path = "/admin/iam", stored = {}, session, fetchOverride } = {}) {
  const store = storage(stored);
  Object.defineProperty(globalThis, "sessionStorage", { value: store, configurable: true, writable: true });
  const calls = [];
  const navigations = [];
  const listeners = {};
  let hidden = 0;
  let nativeSignIn = () => {};
  const [pathname, ...query] = path.split("?");
  const portal = createIamPortal({
    location: {
      pathname,
      search: query.length ? `?${query.join("?")}` : "",
      replace: (url) => void navigations.push(url),
    },
    fetch: async (url, init = {}) => {
      calls.push({ url, method: init.method ?? "GET", headers: init.headers ?? {} });
      if (fetchOverride) return fetchOverride(url, init);
      return session
        ? new Response(JSON.stringify(session), { status: 200 })
        : new Response("{}", { status: 401 });
    },
    intercept: (type, listener) => void ((listeners[type] ??= []).push(listener)),
    hide: () => void (hidden += 1),
    onNativeSignIn: (callback) => void (nativeSignIn = callback),
  });
  return {
    portal,
    calls,
    navigations,
    store,
    hidden: () => hidden,
    bringUpNativeSignIn: () => nativeSignIn(),
    // The console's own handlers come after these; the portal's run first.
    click: (target) => {
      const event = keptEvent(target);
      for (const listener of listeners.click ?? []) listener(event);
      return event;
    },
    unauthorized: () => {
      for (const listener of listeners["rlark:unauthorized"] ?? []) listener(keptEvent());
    },
  };
}

function keptEvent(target) {
  const kept = { prevented: 0, stopped: 0, immediate: 0 };
  return {
    kept,
    target,
    preventDefault: () => void (kept.prevented += 1),
    stopPropagation: () => void (kept.stopped += 1),
    stopImmediatePropagation: () => void (kept.immediate += 1),
  };
}

function nativeButton(label) {
  return { closest: (selector) => (selector === ".account-popover button" ? { textContent: label } : null) };
}

test("a tab with the session cookie but no token enters as that session", async () => {
  const tab = consoleTab({ path: "/admin/iam?tab=users", session: adminSession() });
  await tab.portal.enter();
  assert.deepEqual(
    Object.fromEntries([
      "rlark-auth-token",
      "rlark-auth-role",
      "rlark-admin-auth",
      "rlark-admin-user-name",
      "rlark-user-auth",
      "rlark-user-name",
    ].map((key) => [key, tab.store.getItem(key)])),
    {
      "rlark-auth-token": "opaque-session-token",
      "rlark-auth-role": "admin",
      "rlark-admin-auth": "1",
      "rlark-admin-user-name": "admin",
      "rlark-user-auth": null,
      "rlark-user-name": null,
    },
  );
  assert.deepEqual(tab.calls, [
    { url: "/api/iam/session", method: "GET", headers: { Accept: "application/json" } },
  ]);
  assert.deepEqual(tab.navigations, []);
});

test("a session of another role replaces the roles the tab remembered", async () => {
  const tab = consoleTab({
    stored: { "rlark-admin-auth": "1", "rlark-admin-user-name": "someone" },
    session: { ...adminSession("operator"), user: { username: "operator1" } },
  });
  await tab.portal.enter();
  assert.equal(tab.store.getItem("rlark-auth-role"), "user");
  assert.equal(tab.store.getItem("rlark-user-auth"), "1");
  assert.equal(tab.store.getItem("rlark-user-name"), "operator1");
  assert.equal(tab.store.getItem("rlark-admin-auth"), null);
  assert.equal(tab.store.getItem("rlark-admin-user-name"), null);
});

test("a tab that holds the token does not ask IAM", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "opaque-session-token" } });
  await tab.portal.enter();
  assert.deepEqual(tab.calls, []);
  assert.deepEqual(tab.navigations, []);
});

for (const [name, options] of [
  ["no session", { session: undefined }],
  ["a session answer without a token", { session: { ok: true, user: { username: "admin" } } }],
  ["a session answer that is not JSON", { fetchOverride: () => new Response("<html>", { status: 200 }) }],
  [
    "IAM out of reach",
    { session: adminSession(), fetchOverride: () => new Promise((_, reject) => reject(new TypeError("Failed to fetch"))) },
  ],
]) {
  test(`with ${name} the console leaves for the portal and never renders`, async () => {
    const tab = consoleTab({ path: "/terminal?worker=w", ...options });
    let rendered = false;
    void tab.portal.enter().then(() => void (rendered = true));
    await settle();
    assert.equal(rendered, false);
    assert.deepEqual(tab.navigations, ["/login?next=%2Fterminal%3Fworker%3Dw"]);
    assert.equal(tab.hidden(), 1);
  });
}

test("the way back after signing in is the whole address, as one query parameter", () => {
  assert.equal(portalPath("/"), "/login?next=%2F");
  assert.equal(portalPath("/admin/iam?tab=users&select=u_1"), "/login?next=%2Fadmin%2Fiam%3Ftab%3Dusers%26select%3Du_1");
});

test("the native sign-out is kept from the console and revokes the session", async () => {
  const tab = consoleTab({ path: "/admin/iam", stored: { "rlark-auth-token": "opaque-session-token", "rlark-auth-role": "admin" } });
  await tab.portal.enter();
  await tab.click(nativeButton("Sign out"));
  assert.deepEqual(tab.calls, [
    {
      url: "/api/iam/logout",
      method: "POST",
      headers: { Authorization: "Bearer opaque-session-token" },
    },
  ]);
  assert.deepEqual(tab.navigations, ["/login?next=%2Fadmin%2Fiam"]);
  assert.equal(tab.store.getItem("rlark-auth-token"), null);
  assert.equal(tab.hidden(), 1);
});

test("a sign-out that IAM does not answer still leaves for the portal", async () => {
  const tab = consoleTab({
    stored: { "rlark-auth-token": "opaque-session-token" },
    fetchOverride: () => new Promise((_, reject) => reject(new TypeError("Failed to fetch"))),
  });
  await tab.portal.enter();
  await tab.portal.signOut("/");
  assert.deepEqual(tab.navigations, ["/login?next=%2F"]);
  assert.equal(tab.store.getItem("rlark-auth-token"), null);
});

test("another button of the account menu reaches the console untouched", async () => {
  const tab = consoleTab({ stored: { "rlark-auth-token": "opaque-session-token" } });
  await tab.portal.enter();
  const event = keptEvent(nativeButton("个人资料"));
  await tab.click(event.target);
  assert.equal(event.kept.prevented, 0);
  assert.equal(event.kept.stopped, 0);
  assert.equal(event.kept.immediate, 0);
  assert.deepEqual(tab.calls, []);
  assert.deepEqual(tab.navigations, []);
});

test("an unauthorized answer leaves for the portal before the console hears it", async () => {
  const tab = consoleTab({ path: "/jobs/demo", stored: { "rlark-auth-token": "opaque-session-token" } });
  await tab.portal.enter();
  tab.unauthorized();
  await settle();
  assert.equal(tab.store.getItem("rlark-auth-token"), null);
  assert.deepEqual(tab.navigations, ["/login?next=%2Fjobs%2Fdemo"]);
});

test("a native sign-in brought up by anything else never stays", async () => {
  const signedOut = consoleTab({ path: "/admin/iam", stored: {} });
  void signedOut.portal.enter();
  await settle();
  signedOut.bringUpNativeSignIn();
  await settle();
  assert.ok(signedOut.navigations.length >= 1);
  assert.ok(signedOut.navigations.every((url) => url === "/login?next=%2Fadmin%2Fiam"));

  const otherRole = consoleTab({ path: "/admin/iam", stored: { "rlark-auth-token": "opaque-session-token", "rlark-auth-role": "user" } });
  await otherRole.portal.enter();
  otherRole.bringUpNativeSignIn();
  await settle();
  assert.deepEqual(otherRole.navigations, ["/"]);
});

test("the console renders only after the portal let it", async () => {
  const main = await readFile(new URL("../src/main.tsx", import.meta.url), "utf8");
  assert.match(main, /import \{ enterIamPortal \} from "\.\/astribot\/iam\/portal";/);
  assert.match(main, /enterIamPortal\(\)\.then\(\(\) => \{\s*ReactDOM\.createRoot\([^\n]*\)\.render\(/);
  assert.equal(main.match(/ReactDOM\.createRoot/g).length, 1);

  const dockerfile = await readFile(new URL("../../../astribot/release/rlark-ui.Dockerfile", import.meta.url), "utf8");
  assert.match(dockerfile, /^ENV VITE_IAM_PORTAL=1$/m);

  const page = await readFile(new URL("../src/astribot/iam/IAMPage.tsx", import.meta.url), "utf8");
  assert.match(page, /if \(iamPortalEnabled\(\)\) \{\s+await signOutOfConsole\(window\.location\.pathname\);/);
});
