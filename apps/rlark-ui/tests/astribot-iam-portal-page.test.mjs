import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { runInNewContext } from "node:vm";

const login = await readFile(new URL("../../../astribot/release/login.html", import.meta.url), "utf8");
const nginx = await readFile(new URL("../../../astribot/release/rlark-ui.nginx.conf", import.meta.url), "utf8");

function startLoginPage(fetch, language = "zh-CN") {
  const elements = new Map();
  function getElementById(id) {
    if (!elements.has(id)) {
      elements.set(id, {
        type: id === "password" ? "password" : "",
        value: "",
        disabled: false,
        attributes: {},
        setAttribute(name, value) { this.attributes[name] = value; },
        addEventListener(event, listener) { this[event] = listener; },
      });
    }
    return elements.get(id);
  }
  const document = { documentElement: { lang: "zh-CN" }, getElementById };
  const script = login.match(/<script>([\s\S]*?)<\/script>/)?.[1];
  assert.ok(script);
  runInNewContext(script, { document, navigator: { language }, fetch });
  return { document, getElementById };
}

test("failed sign-in restores the submit button and shows localized feedback", async () => {
  const failures = [
    [async () => ({ ok: false, status: 401 }), "账号或密码错误"],
    [async () => { throw new TypeError("Failed to fetch"); }, "暂时无法登录，请稍后重试"],
  ];
  for (const [fetch, message] of failures) {
    const { getElementById: get } = startLoginPage(fetch);
    const submitted = get("form").submit({ preventDefault() {} });
    assert.equal(get("go").disabled, true);
    assert.equal(get("lg").textContent, "登录中…");
    await submitted;
    assert.equal(get("go").disabled, false);
    assert.equal(get("lg").textContent, "登录");
    assert.equal(get("err").textContent, message);
  }
});

test("the password toggle updates its visual and accessible labels", () => {
  const { document, getElementById: get } = startLoginPage(undefined, "en");
  assert.equal(document.documentElement.lang, "en");
  const toggle = get("show");
  assert.equal(toggle.attributes["aria-label"], "Show password");
  toggle.click();
  assert.equal(get("password").type, "text");
  assert.equal(toggle.textContent, "Hide");
  assert.equal(toggle.attributes["aria-label"], "Hide password");
  toggle.click();
  assert.equal(get("password").type, "password");
  assert.equal(toggle.textContent, "Show");
});

test("the sign-in heading uses the short label in both languages", () => {
  assert.match(login, /<h1 id="t">登陆<\/h1>/);
  const { getElementById: get } = startLoginPage(undefined, "en");
  assert.equal(get("t").textContent, "Sign in");
});

const readNextSource = login.match(/function readNext\(\) \{[\s\S]*?\n  \}/)?.[0];
assert.ok(readNextSource, "login.html defines readNext()");
const readNext = (search) =>
  new Function("window", `"use strict"; ${readNextSource}; return readNext();`)({ location: { search } });

test("the address a redirect carries is read back whole", () => {
  // nginx: next=$request_uri, as the browser wrote it.
  assert.equal(readNext("?next=/admin/iam"), "/admin/iam");
  assert.equal(readNext("?next=/terminal?worker=w&job=j"), "/terminal?worker=w&job=j");
  assert.equal(readNext("?next=/jobs/%E6%9C%BA%E5%99%A8%E4%BA%BA"), "/jobs/%E6%9C%BA%E5%99%A8%E4%BA%BA");
  assert.equal(readNext("?next=/jobs/a%2Bb"), "/jobs/a%2Bb");
  assert.equal(readNext("?lang=zh&next=/admin"), "/admin");
  // The console: encodeURIComponent.
  assert.equal(readNext("?next=%2Fjobs%2Fa%20b%3Fx%3D1%26y%3D2"), "/jobs/a b?x=1&y=2");
});

test("without a usable address the way back is the console's entrance", () => {
  for (const search of ["", "?", "?next=", "?x=1", "?xnext=/admin", "?next=%E0%A4%A", "?next=%zz"]) {
    assert.equal(readNext(search), "/", search);
  }
});

test("the way back is a path of this origin only", () => {
  for (const value of [
    "//evil.example/",
    "/\\evil.example/",
    "https://evil.example/",
    "javascript:alert(1)",
    "%2F%2Fevil.example%2F",
    "%2F%5Cevil.example%2F",
    "%2F%2f%2Fevil.example",
  ]) {
    assert.equal(readNext(`?next=${value}`), "/", value);
  }
});

const landingSource = login.match(/function landingPath\(next, admin\) \{[\s\S]*?\n  \}/)?.[0];
assert.ok(landingSource, "login.html defines landingPath()");
const landingPath = new Function(`"use strict"; ${landingSource}; return landingPath;`)();

test("the successful sign-in uses the IAM role to choose the default console", () => {
  assert.match(login, /var admin = o\.role === "admin";[\s\S]*?window\.location\.replace\(landingPath\(readNext\(\), admin\)\);/);
  for (const search of ["", "?next=/", "?next=%2F", "?next=/?lang=zh"]) {
    assert.equal(landingPath(readNext(search), true), "/admin", search);
    assert.equal(landingPath(readNext(search), false), readNext(search), search);
  }
});

test("sign-in preserves deep links allowed for the role", () => {
  for (const path of ["/admin", "/admin/iam", "/admin/iam?tab=users", "/admin#section"]) {
    assert.equal(landingPath(path, true), path);
    assert.equal(landingPath(path, false), "/");
  }
  for (const path of ["/administrator", "/jobs/admin", "/terminal?worker=w&job=j"]) {
    assert.equal(landingPath(path, true), path);
    assert.equal(landingPath(path, false), path);
  }
});

test("a request without a session is redirected to the portal, carrying its address", () => {
  assert.match(nginx, /location \/ \{\s+auth_request \/_iam_page;\s+error_page 401 = @iam_login;/);
  assert.match(nginx, /location @iam_login \{\s+return 302 \/login\?next=\$request_uri;\s+\}/);
  assert.match(nginx, /^\s+absolute_redirect off;$/m);
  // A redirect in the body of a 302 without Location keeps the address bar on the page that was refused.
  assert.doesNotMatch(nginx, /error_page \d+ =302 \/login/);
});

test("the console shell is always confirmed with the server", () => {
  assert.match(
    nginx,
    /location = \/index\.html \{\s+auth_request \/_iam_page;\s+error_page 401 = @iam_login;\s+add_header Cache-Control "no-cache";/,
  );
});

test("page and API authorization use separate internal IAM endpoints", () => {
  assert.match(nginx, /location = \/login \{[\s\S]*?add_header Set-Cookie "astribot_session=; Path=\/; Max-Age=0; HttpOnly; SameSite=Lax" always;[\s\S]*?try_files \/login\.html =404;/);
  assert.match(nginx, /location \/api\/ \{\s+auth_request \/_iam_api;/);
  assert.match(nginx, /location = \/_iam_page \{\s+internal;\s+proxy_pass \$\{ASTRIBOT_IAM_URL\}\/internal\/admit;[\s\S]*?proxy_set_header X-Original-URI \$request_uri;/);
  assert.match(nginx, /location = \/_iam_api \{\s+internal;\s+proxy_pass \$\{ASTRIBOT_IAM_URL\}\/internal\/authorize;[\s\S]*?proxy_set_header X-Original-URI \$request_uri;/);
});

test("the proxy takes the workload submitter assertion from IAM", () => {
  assert.match(nginx, /auth_request_set \$iam_submitter \$upstream_http_x_rlark_submitter;/);
  assert.match(nginx, /proxy_set_header X-RLark-Submitter \$iam_submitter;/);
});

test("the portal page is always fetched, so that it can end the browser's session", () => {
  // A copy replayed from the browser's cache carries no Set-Cookie.
  assert.match(nginx, /location = \/login \{[\s\S]*?add_header Set-Cookie "astribot_session=;[^"]*" always;\s+(?:#[^\n]*\n\s*)?add_header Cache-Control "no-store" always;/);
});
