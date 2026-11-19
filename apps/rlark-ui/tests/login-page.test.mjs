import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const loginSource = await readFile(
  new URL("../src/pages/Login.tsx", import.meta.url),
  "utf8",
);
const adminLoginSource = await readFile(
  new URL("../src/admin/AdminApp.tsx", import.meta.url),
  "utf8",
);

test("user login shows the RLark brand logo", () => {
  assert.match(
    loginSource,
    /src=\{`\/rlark-logo-\$\{zh \? "zh" : "en"\}-light\.png`\}/,
  );
  assert.match(loginSource, /"RLark 具身智能云原生纳管平台"/);
  assert.match(
    loginSource,
    /"RLark cloud-native embodied intelligence platform"/,
  );
});

test("password visibility control is accessible and does not submit", () => {
  assert.match(loginSource, /type=\{showPassword \? "text" : "password"\}/);
  assert.match(
    loginSource,
    /placeholder=\{zh \? "请输入密码" : "Enter your password"\}/,
  );
  assert.doesNotMatch(loginSource, /placeholder="•+/);
  assert.match(loginSource, /type="button"/);
  assert.match(loginSource, /aria-label=\{\s*showPassword/);
  assert.match(loginSource, /"隐藏密码"/);
  assert.match(loginSource, /"Hide password"/);
  assert.match(loginSource, /aria-pressed=\{showPassword\}/);
});

test("login errors render inline below the credentials", () => {
  assert.match(loginSource, /className="admin-login-panel"/);
  assert.match(loginSource, /className="login-inline-error"/);
  assert.match(loginSource, /role="alert"/);
  assert.match(
    loginSource,
    /window\.setTimeout\(\(\) => setError\(""\), 4000\)/,
  );
});

test("admin login shares password and inline error controls", () => {
  assert.doesNotMatch(adminLoginSource, /className="admin-login-topbar"/);
  assert.match(adminLoginSource, /className="admin-login-panel"/);
  assert.match(adminLoginSource, /className="admin-login-badge">ADMIN/);
  assert.match(
    adminLoginSource,
    /type=\{showPassword \? "text" : "password"\}/,
  );
  assert.match(
    adminLoginSource,
    /placeholder=\{zh \? "请输入密码" : "Enter password"\}/,
  );
  assert.match(adminLoginSource, /className="login-inline-error"/);
  assert.match(
    adminLoginSource,
    /window\.setTimeout\(\(\) => setError\(""\), 4000\)/,
  );
});
