import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const nginxConfig = await readFile(
  new URL("../nginx.conf", import.meta.url),
  "utf8",
);

test("preserves the public port for WebSocket origin checks", () => {
  assert.match(nginxConfig, /proxy_set_header\s+Host\s+\$http_host\s*;/);
  assert.doesNotMatch(nginxConfig, /proxy_set_header\s+Host\s+\$host\s*;/);
});
