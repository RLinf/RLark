import { clearAuthSession, storeAuthSession, UNAUTHORIZED_EVENT } from "../../api.js";

const tokenKey = "rlark-auth-token";
const sessionPath = "/api/iam/session";
const logoutPath = "/api/iam/logout";
const signOutLabel = /退出|sign\s*out|log\s*out/i;
const requestTimeoutMs = 5000;

interface PortalSession {
  token?: unknown;
  role?: unknown;
  user?: { username?: unknown };
}

/** An event heard before the console's own handlers, and kept from them on demand. */
export interface InterceptedEvent {
  target: unknown;
  preventDefault(): void;
  stopPropagation(): void;
  stopImmediatePropagation(): void;
}

/** What the portal needs from the page; iamPortal() binds the console's own. */
export interface PortalHost {
  location: { pathname: string; search: string; replace(url: string): void };
  fetch(input: string, init: RequestInit): Promise<Response>;
  intercept(type: string, listener: (event: InterceptedEvent) => void): void;
  /** Stops showing the console: what it renders while it leaves must not be seen. */
  hide(): void;
  /** Calls back once the native sign-in page has been brought up by anything else. */
  onNativeSignIn(callback: () => void): void;
}

export type IamPortal = ReturnType<typeof createIamPortal>;

/** The portal's sign-in page, returning to `path` afterwards. */
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

/**
 * Behind Astribot IAM the console has no sign-in of its own: the session is
 * IAM's cookie, the native sign-in page has no credentials that work, and
 * every way out of the console leads to the portal (/login).
 */
export function createIamPortal(host: PortalHost) {
  const parked = new Promise<void>(() => {});

  function leave(path: string): Promise<void> {
    host.hide();
    host.location.replace(portalPath(path));
    return parked;
  }

  // The cookie outlives the tab's sessionStorage: a new tab or a bookmark
  // starts with the session but without the token the console calls the API with.
  async function restore(): Promise<void> {
    let session: PortalSession | null = null;
    try {
      const response = await host.fetch(sessionPath, {
        credentials: "same-origin",
        cache: "no-store",
        headers: { Accept: "application/json" },
        signal: AbortSignal.timeout(requestTimeoutMs),
      });
      if (response.ok) session = (await response.json()) as PortalSession;
    } catch {
      // No session, or IAM out of reach: the portal says which.
    }
    const token = session?.token;
    if (typeof token === "string" && token && typeof session?.user?.username === "string") {
      storeSession({ token, role: session?.role, user: session?.user });
      return;
    }
    await leave(consolePath(host));
  }

  async function signOut(path: string): Promise<void> {
    host.hide();
    const token = sessionStorage.getItem(tokenKey);
    try {
      await host.fetch(logoutPath, {
        method: "POST",
        credentials: "same-origin",
        cache: "no-store",
        headers: token ? { Authorization: `Bearer ${token}` } : {},
        signal: AbortSignal.timeout(requestTimeoutMs),
      });
    } catch {
      // The session may be gone already, or IAM out of reach: the person
      // leaves for the portal either way.
    }
    clearAuthSession();
    host.location.replace(portalPath(path));
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
      clearAuthSession();
      void leave(consolePath(host));
    });
    host.onNativeSignIn(() => {
      // A session of the other role asked for this console: it goes to the one it is for.
      if (sessionStorage.getItem(tokenKey)) {
        host.hide();
        host.location.replace("/");
        return;
      }
      void leave(consolePath(host));
    });
  }

  return {
    signOut,
    /** Resolves once the console may render as the signed-in session; never resolves when it leaves for the portal. */
    enter(): Promise<void> {
      keepNativeSignInOut();
      return sessionStorage.getItem(tokenKey) ? Promise.resolve() : restore();
    },
  };
}

function browserHost(): PortalHost {
  return {
    location: window.location,
    fetch: (input, init) => fetch(input, init),
    intercept: (type, listener) =>
      window.addEventListener(type, listener as EventListener, true),
    hide: () => {
      const root = document.getElementById("root");
      if (root) root.style.display = "none";
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
  };
}

const buildEnv: Record<string, string | undefined> = import.meta.env ?? {};
let portal: IamPortal | undefined;

/** True of the console built for the portal (astribot/release/rlark-ui.Dockerfile). */
export function iamPortalEnabled(): boolean {
  return buildEnv.VITE_IAM_PORTAL === "1";
}

function iamPortal(): IamPortal {
  portal ??= createIamPortal(browserHost());
  return portal;
}

/**
 * Called before the console's first render. Resolves once the console may
 * render as the signed-in session; never resolves when it leaves for the portal.
 */
export function enterIamPortal(): Promise<void> {
  return iamPortalEnabled() ? iamPortal().enter() : Promise.resolve();
}

/** Signs the person out of IAM and leaves for the portal, which returns to `path` after the next sign-in. */
export function signOutOfConsole(path: string): Promise<void> {
  return iamPortalEnabled() ? iamPortal().signOut(path) : Promise.resolve();
}
