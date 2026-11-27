import assert from "node:assert/strict";
import test from "node:test";
import {
  pullCommand,
  registryCommands,
  shellQuote,
} from "../dist/test/astribot/lib/registryCommands.js";

test("registry commands quote user controlled values and contain no password", () => {
  assert.equal(shellQuote("a'b"), "'a'\\''b'");
  const commands = registryCommands("192.168.31.2:5443", "name'unsafe");
  assert.equal(
    commands[0],
    "docker login '192.168.31.2:5443' -u 'name'\\''unsafe'",
  );
  assert.equal(
    commands[2],
    "docker push '192.168.31.2:5443/example/image:version'",
  );
  assert.equal(
    commands.some((command) => command.includes("password")),
    false,
  );
  assert.equal(
    pullCommand("host/repo@sha256:abc"),
    "docker pull 'host/repo@sha256:abc'",
  );
});
