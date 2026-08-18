import assert from "node:assert/strict";
import test from "node:test";

import {
  getSafariTerminalInput,
  getTerminalCloseExitCode,
  getTerminalProcessExitCode,
  isTerminalProcessExitMessage,
  isSafariUserAgent,
  stripLegacyProxyCloseMessage,
} from "../dist/test/utils/terminalKeyboard.js";

const safariMac =
  "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) " +
  "AppleWebKit/605.1.15 (KHTML, like Gecko) Version/18.6 Safari/605.1.15";

function keyEvent(key, overrides = {}) {
  return {
    altKey: false,
    ctrlKey: false,
    isComposing: false,
    key,
    metaKey: false,
    type: "keydown",
    ...overrides,
  };
}

test("detects macOS Safari without matching Chromium browsers", () => {
  assert.equal(isSafariUserAgent(safariMac), true);
  assert.equal(
    isSafariUserAgent(
      "Mozilla/5.0 (Macintosh) AppleWebKit/537.36 Chrome/140.0 Safari/537.36",
    ),
    false,
  );
});

test("forwards the Vim insert, command, and quit key sequence", () => {
  const sequence = ["i", "Escape", ":", "q", "Enter"];
  assert.equal(
    sequence.map((key) => getSafariTerminalInput(keyEvent(key))).join(""),
    "i\x1b:q\r",
  );
});

test("forwards editing control keys", () => {
  assert.equal(getSafariTerminalInput(keyEvent("Backspace")), "\x7f");
  assert.equal(getSafariTerminalInput(keyEvent("Tab")), "\t");
});

test("leaves shortcuts, IME composition, and non-keydown events to xterm", () => {
  assert.equal(getSafariTerminalInput(keyEvent("c", { metaKey: true })), null);
  assert.equal(
    getSafariTerminalInput(keyEvent("i", { isComposing: true })),
    null,
  );
  assert.equal(getSafariTerminalInput(keyEvent("Process")), null);
  assert.equal(getSafariTerminalInput(keyEvent("i", { type: "keyup" })), null);
});

test("removes duplicate legacy proxy close messages", () => {
  const legacy =
    "connection closed: websocket: close 1006 (abnormal closure): unexpected EOF\r\n";
  const result = stripLegacyProxyCloseMessage(`output\r\n${legacy}${legacy}`);
  assert.equal(result.legacyClose, true);
  assert.equal(result.output, "output\r\n");
});

test("keeps regular terminal output unchanged", () => {
  const result = stripLegacyProxyCloseMessage("command output\r\n");
  assert.equal(result.legacyClose, false);
  assert.equal(result.output, "command output\r\n");
});

test("recognizes Kubernetes exec process exit messages", () => {
  assert.equal(
    isTerminalProcessExitMessage(
      "exec error: command terminated with exit code 127",
    ),
    true,
  );
  assert.equal(
    isTerminalProcessExitMessage("exec error: websocket transport failed"),
    false,
  );
  assert.equal(
    getTerminalProcessExitCode(
      "exec error: command terminated with exit code 127",
    ),
    127,
  );
  assert.equal(getTerminalCloseExitCode("terminal exited with code 42"), 42);
  assert.equal(getTerminalCloseExitCode("terminal session ended"), null);
});
