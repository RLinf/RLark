import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const login = await readFile(new URL("../../../astribot/release/login.html", import.meta.url), "utf8");
const nginx = await readFile(new URL("../../../astribot/release/rlark-ui.nginx.conf", import.meta.url), "utf8");

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

test("the admin console is not the way back for another role", () => {
  const rule = login.match(/if \(!admin && (\/.*)\.test\(next\)\) next = "\/";/)?.[1];
  assert.ok(rule, "login.html sends another role away from the admin console");
  const adminConsole = new Function(`return ${rule}`)();
  assert.equal(adminConsole.test("/admin"), true);
  assert.equal(adminConsole.test("/admin/iam"), true);
  assert.equal(adminConsole.test("/admin/iam?tab=users"), true);
  assert.equal(adminConsole.test("/administrator"), false);
  assert.equal(adminConsole.test("/jobs/admin"), false);
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

test("the portal page is always fetched, so that it can end the browser's session", () => {
  // A copy replayed from the browser's cache carries no Set-Cookie.
  assert.match(nginx, /location = \/login \{[\s\S]*?add_header Set-Cookie "astribot_session=;[^"]*" always;\s+(?:#[^\n]*\n\s*)?add_header Cache-Control "no-store" always;/);
});
