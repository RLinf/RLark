import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { runInNewContext } from "node:vm";

const login = await readFile(new URL("../../../astribot/release/login.html", import.meta.url), "utf8");
const nginx = await readFile(new URL("../../../astribot/release/rlark-ui.nginx.conf", import.meta.url), "utf8");
const appSource = await readFile(new URL("../src/App.tsx", import.meta.url), "utf8");
const adminAppSource = await readFile(new URL("../src/admin/AdminApp.tsx", import.meta.url), "utf8");
const userLoginSource = await readFile(new URL("../src/pages/Login.tsx", import.meta.url), "utf8");
const iamPageSource = await readFile(new URL("../src/astribot/iam/IAMPage.tsx", import.meta.url), "utf8");

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
    /location = \/index\.html \{\s+auth_request \/_iam_page;\s+error_page 401 = @iam_login;\s+error_page 403 = @iam_home;\s+add_header Cache-Control "no-cache";/,
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

test("every startup state is answered before a native form can be rendered", () => {
  assert.match(
    appSource,
    /import \{\s+addressPath,\s+portalHomeRedirect,\s+resolvePortalView,[\s\S]*?\} from "\.\/astribot\/iam\/portal";/,
  );
  assert.match(appSource, /const portalView = portalState\s+\? resolvePortalView\(portalState, window\.location\.pathname\)/);
  // The three states that must not reach a form each return their own screen.
  assert.match(appSource, /if \(portalView\.kind === "error"\)\s+return \(\s+<PortalError/);
  assert.match(appSource, /if \(portalView\.kind === "pending"\)\s+return \(\s+<PortalStatus/);
  assert.match(appSource, /if \(portalView\.kind === "home"\)\s+return \(\s+<PortalStatus/);
  // A role without admin access is sent home once, never to AdminLogin.
  assert.match(appSource, /const target = portalView \? portalHomeRedirect\(portalView\) : undefined;/);
  assert.match(appSource, /window\.location\.replace\(target\)/);
  assert.match(appSource, /<AdminApp\s+portalRole=\{portalView\?\.kind === "admin" \? "admin" : undefined\}/);
});

test("the native user form is mounted only outside portal mode", () => {
  assert.equal(appSource.match(/<UserLogin/g)?.length, 1);
  assert.match(appSource, /return portalView \? \(\s*<PortalStatus[\s\S]*?\)\s*:\s*\(\s*<UserLogin/);
});

test("the admin console never mounts its native form behind the portal", () => {
  assert.equal(adminAppSource.match(/<AdminLogin/g)?.length, 1);
  assert.match(adminAppSource, /const portal = iamPortalEnabled\(\) \|\| portalRole !== undefined;/);
  assert.match(adminAppSource, /if \(!loggedIn\) \{\s+if \(portal\) \{\s+return \(/);
  assert.match(adminAppSource, /if \(portal && !loggedIn\) void returnToPortal\(addressPath\(window\.location\)\)/);
  assert.match(adminAppSource, /if \(portal\) \{\s+void signOutOfConsole\(addressPath\(window\.location\)\)/);
});

test("both native forms switch themselves off in the portal build", () => {
  assert.match(userLoginSource, /if \(iamPortalEnabled\(\)\) return null;/);
  assert.match(adminAppSource, /if \(iamPortalEnabled\(\)\) return null;/);
});

test("the IAM page never mounts its own credentials form behind the portal", () => {
  // A 403, a 5xx or an unreachable IAM renders the retry screen instead of the form.
  assert.match(iamPageSource, /if \(iamPortalEnabled\(\)\) return renderPortalSession\(\);/);
  assert.match(iamPageSource, /iamPortalEnabled\(\) \? renderPortalSession\(\) : renderLogin\(\)/);
  assert.equal(iamPageSource.match(/<section className="iam-login-screen"/g)?.length, 1);
});

test("the release proxy refuses the native credentials endpoint", () => {
  assert.match(
    nginx,
    /location = \/api\/v1\/auth\/login \{[\s\S]{0,160}?return 403 '\{"(?:code|error)":"native_login_disabled"/,
  );
});
