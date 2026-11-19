const SAFARI_USER_AGENT = /^((?!chrome|chromium|android).)*safari/i;

export function isSafariUserAgent(userAgent: string): boolean {
  return SAFARI_USER_AGENT.test(userAgent);
}

export type TerminalKeyEvent = Pick<
  KeyboardEvent,
  "altKey" | "ctrlKey" | "isComposing" | "key" | "metaKey" | "type"
>;

/**
 * Returns terminal input that must bypass xterm's hidden textarea on Safari.
 * Safari can skip xterm's normal data event for printable keys and Escape,
 * which makes full-screen programs such as Vim appear frozen.
 */
export function getSafariTerminalInput(event: TerminalKeyEvent): string | null {
  if (
    event.type !== "keydown" ||
    event.isComposing ||
    event.key === "Process" ||
    event.key === "Dead" ||
    event.metaKey ||
    event.ctrlKey ||
    event.altKey
  ) {
    return null;
  }

  switch (event.key) {
    case "Escape":
      return "\x1b";
    case "Enter":
      return "\r";
    case "Backspace":
      return "\x7f";
    case "Tab":
      return "\t";
    default:
      return Array.from(event.key).length === 1 ? event.key : null;
  }
}

const LEGACY_PROXY_CLOSE_MESSAGE =
  /connection closed: websocket: close 1006 \(abnormal closure\): unexpected EOF\r?\n?/gi;

export function stripLegacyProxyCloseMessage(data: string): {
  output: string;
  legacyClose: boolean;
} {
  LEGACY_PROXY_CLOSE_MESSAGE.lastIndex = 0;
  const legacyClose = LEGACY_PROXY_CLOSE_MESSAGE.test(data);
  LEGACY_PROXY_CLOSE_MESSAGE.lastIndex = 0;
  return {
    output: legacyClose ? data.replace(LEGACY_PROXY_CLOSE_MESSAGE, "") : data,
    legacyClose,
  };
}

const TERMINAL_PROCESS_EXIT_MESSAGE =
  /^exec error: command terminated with exit code (\d+)$/i;

export function isTerminalProcessExitMessage(message: string): boolean {
  return getTerminalProcessExitCode(message) !== null;
}

export function getTerminalProcessExitCode(message: string): number | null {
  const match = TERMINAL_PROCESS_EXIT_MESSAGE.exec(message.trim());
  if (!match) return null;
  const exitCode = Number.parseInt(match[1], 10);
  return Number.isFinite(exitCode) ? exitCode : null;
}

export function getTerminalCloseExitCode(reason: string): number | null {
  const match = /^terminal exited with code (\d+)$/i.exec(reason.trim());
  if (!match) return null;
  const exitCode = Number.parseInt(match[1], 10);
  return Number.isFinite(exitCode) ? exitCode : null;
}
