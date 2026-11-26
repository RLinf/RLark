import { clearAuthSession, storeAuthSession, UNAUTHORIZED_EVENT } from "../../api.js";

const tokenKey = "rlark-auth-token";
const sessionPath = "/api/iam/session";
const logoutPath = "/api/iam/logout";
const signOutLabel = /退出|sign\s*out|log\s*out/i;
const requestTimeoutMs = 5000;
const broadcastName = "astribot-iam";

interface PortalSession {
  token?: unknown;
  role?: unknown;
  user?: { username?: unknown };
}

export interface InterceptedEvent {
  target: unknown;
  persisted?: boolean;
  preventDefault(): void;
  stopPropagation(): void;
  stopImmediatePropagation(): void;
}

export interface PortalHost {
  location: { pathname: string; search: string; replace(url: string): void };
  fetch(input: string, init: RequestInit): Promise<Response>;
  intercept(type: string, listener: (event: InterceptedEvent) => void): void;
  cover(message: string): void;
  onNativeSignIn(callback: () => void): void;
  onPageShow(callback: (persisted: boolean) => void): void;
  onLogoutBroadcast(callback: () => void): void;
  broadcastLogout(): void;
}

export type IamPortal = ReturnType<typeof createIamPortal>;

export function portalPath(path: string): string {
  return `/login?next=${encodeURIComponent(path)}`;
}

function consolePath(host: PortalHost): string {
  return host.location.pathname + host.location.search;
}

function isSignOutButton(target: unknown): boolean {
  const button = (
    target as { closest?: (selector: string) => { textContent?: string } | null } | null
  )?.closest?.(".account-popover button");
  return Boolean(button && signOutLabel.test(button.textContent ?? ""));
}

function storeSession(session: PortalSession): void {
  const admin = session.role === "admin";
  const role = admin ? "admin" : "user";
  clearAuthSession();
  storeAuthSession(session.token as string, role);
  sessionStorage.setItem(admin ? "rlark-admin-auth" : "rlark-user-auth", "1");
  sessionStorage.setItem(admin ? "rlark-admin-user-name" : "rlark-user-name", session.user?.username as string);
}

function requestSignal(): { signal: AbortSignal; dispose(): void } {
  if (typeof AbortSignal.timeout === "function") {
    return { signal: AbortSignal.timeout(requestTimeoutMs), dispose: () => {} };
  }
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), requestTimeoutMs);
  return { signal: controller.signal, dispose: () => clearTimeout(timer) };
}

function leaveMessage(destination: "portal" | "console" | "sign-out"): string {
  const language = typeof navigator === "undefined" ? "en" : navigator.language;
  const zh = language.toLowerCase().startsWith("zh");
  if (destination === "sign-out") return zh ? "正在退出登录…" : "Signing out…";
  if (destination === "console") return zh ? "正在前往控制台…" : "Opening the console…";
  return zh ? "正在前往登录页…" : "Redirecting to sign in…";
}

export function createIamPortal(host: PortalHost) {
  const parked = new Promise<void>(() => {});
  let leaving = false;

  function beginDeparture(message: string): boolean {
    if (leaving) return false;
    leaving = true;
    host.cover(message);
    return true;
  }

  function leave(path: string, broadcast = false): Promise<void> {
    if (!beginDeparture(leaveMessage("portal"))) return parked;
    clearAuthSession();
    if (broadcast) host.broadcastLogout();
    host.location.replace(portalPath(path));
    return parked;
  }

  async function syncSession(): Promise<void> {
    const existingToken = sessionStorage.getItem(tokenKey);
    const timeout = requestSignal();
    let response: Response;
    try {
      response = await host.fetch(sessionPath, {
        credentials: "same-origin",
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: timeout.signal,
      });
    } catch {
      if (leaving) return;
      if (!existingToken) await leave(consolePath(host));
      return;
    } finally {
      timeout.dispose();
    }

    if (leaving) return;
    if (response.status === 401) {
      await leave(consolePath(host));
      return;
    }
    if (!response.ok) {
      if (!existingToken) await leave(consolePath(host));
      return;
    }

    let session: PortalSession;
    try {
      session = (await response.json()) as PortalSession;
    } catch {
      if (!existingToken) await leave(consolePath(host));
      return;
    }
    if (typeof session.token === "string" && session.token && typeof session.user?.username === "string") {
      storeSession(session);
      return;
    }
    if (!existingToken) await leave(consolePath(host));
  }

  async function signOut(path: string): Promise<void> {
    if (!beginDeparture(leaveMessage("sign-out"))) return;
    const token = sessionStorage.getItem(tokenKey);
    const timeout = requestSignal();
    try {
      await host.fetch(logoutPath, {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal: timeout.signal,
      });
    } catch {
      // Navigation still clears the browser session when IAM cannot confirm logout.
    } finally {
      timeout.dispose();
      clearAuthSession();
      host.broadcastLogout();
      host.location.replace(portalPath(path));
    }
  }

  function keepNativeSignInOut(): void {
    host.intercept("click", (event) => {
      if (!isSignOutButton(event.target)) return;
      event.preventDefault();
      event.stopPropagation();
      event.stopImmediatePropagation();
      void signOut(consolePath(host));
    });
    host.intercept(UNAUTHORIZED_EVENT, (event) => {
      event.stopImmediatePropagation();
      void leave(consolePath(host));
    });
    host.onNativeSignIn(() => {
      if (sessionStorage.getItem(tokenKey)) {
        if (!beginDeparture(leaveMessage("console"))) return;
        host.location.replace("/");
        return;
      }
      void leave(consolePath(host));
    });
    host.onPageShow((persisted) => {
      if (persisted && !leaving) void syncSession();
    });
    host.onLogoutBroadcast(() => void leave(consolePath(host)));
  }

  return {
    signOut,
    returnToPortal(path: string): Promise<void> {
      return leave(path);
    },
    enter(): Promise<void> {
      keepNativeSignInOut();
      return syncSession();
    },
  };
}

function browserHost(): PortalHost {
  let channel: BroadcastChannel | undefined;
  try {
    if (typeof BroadcastChannel !== "undefined") channel = new BroadcastChannel(broadcastName);
  } catch {
    channel = undefined;
  }
  return {
    location: window.location,
    fetch: (input, init) => fetch(input, init),
    intercept: (type, listener) => window.addEventListener(type, listener as EventListener, true),
    cover: (message) => {
      let cover = document.getElementById("iam-portal-leaving");
      if (!cover) {
        cover = document.createElement("div");
        cover.id = "iam-portal-leaving";
        cover.setAttribute("role", "status");
        Object.assign(cover.style, {
          position: "fixed",
          inset: "0",
          zIndex: "2147483647",
          display: "grid",
          placeItems: "center",
          background: "#f6f4fc",
          color: "#403773",
          font: "500 15px/1.5 system-ui, sans-serif",
          pointerEvents: "auto",
        });
        document.body.append(cover);
      }
      cover.textContent = message;
    },
    onNativeSignIn: (callback) => {
      const root = document.getElementById("root");
      if (!root || typeof MutationObserver === "undefined") return;
      new MutationObserver((records) => {
        for (const record of records) {
          for (const node of record.addedNodes) {
            if (!(node instanceof Element)) continue;
            if (!node.matches(".admin-login-page") && !node.querySelector(".admin-login-page")) continue;
            callback();
            return;
          }
        }
      }).observe(root, { childList: true, subtree: true });
    },
    onPageShow: (callback) => window.addEventListener("pageshow", (event) => callback((event as PageTransitionEvent).persisted)),
    onLogoutBroadcast: (callback) => {
      if (channel) channel.onmessage = (event) => {
        if (event.data === "logout") callback();
      };
    },
    broadcastLogout: () => channel?.postMessage("logout"),
  };
}

const buildEnv: Record<string, string | undefined> = import.meta.env ?? {};
let portal: IamPortal | undefined;

export function iamPortalEnabled(): boolean {
  return buildEnv.VITE_IAM_PORTAL === "1";
}

function iamPortal(): IamPortal {
  portal ??= createIamPortal(browserHost());
  return portal;
}

export function enterIamPortal(): Promise<void> {
  return iamPortalEnabled() ? iamPortal().enter() : Promise.resolve();
}

export function signOutOfConsole(path: string): Promise<void> {
  return iamPortalEnabled() ? iamPortal().signOut(path) : Promise.resolve();
}

export function returnToPortal(path: string): Promise<void> {
  return iamPortalEnabled() ? iamPortal().returnToPortal(path) : Promise.resolve();
}
