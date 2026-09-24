export const AUTH_TOKEN_KEY = "rlark-auth-token";
export const AUTH_ROLE_KEY = "rlark-auth-role";
export const UNAUTHORIZED_EVENT = "rlark:unauthorized";

export type AuthRole = "admin" | "user";

export interface LoginResponse {
  ok: boolean;
  role: AuthRole;
  token: string;
  expiresAt: string;
}

export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
    public body?: unknown,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

export function storeAuthSession(token: string, role: AuthRole) {
  sessionStorage.setItem(AUTH_TOKEN_KEY, token);
  sessionStorage.setItem(AUTH_ROLE_KEY, role);
}

export function clearAuthSession() {
  sessionStorage.removeItem(AUTH_TOKEN_KEY);
  sessionStorage.removeItem(AUTH_ROLE_KEY);
  sessionStorage.removeItem("rlark-user-auth");
  sessionStorage.removeItem("rlark-user-name");
  sessionStorage.removeItem("rlark-admin-auth");
  sessionStorage.removeItem("rlark-admin-user-name");
}

export function hasAuthSession(role?: AuthRole) {
  const token = sessionStorage.getItem(AUTH_TOKEN_KEY);
  const storedRole = sessionStorage.getItem(AUTH_ROLE_KEY);
  return Boolean(token && (!role || storedRole === role));
}

async function authenticatedFetch(
  input: RequestInfo | URL,
  init: RequestInit = {},
) {
  const headers = new Headers(init.headers);
  const token = sessionStorage.getItem(AUTH_TOKEN_KEY);
  if (token && !headers.has("Authorization")) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const response = await fetch(input, { ...init, headers });
  if (response.status === 401) {
    clearAuthSession();
    window.dispatchEvent(new Event(UNAUTHORIZED_EVENT));
  }
  return response;
}

async function responseError(response: Response) {
  const text = await response.text();
  let body: unknown = text;
  try {
    body = text ? JSON.parse(text) : undefined;
  } catch {
    // Keep non-JSON error bodies as text.
  }
  const detail =
    body && typeof body === "object" && "error" in body
      ? String((body as { error: unknown }).error)
      : text;
  return new ApiError(
    response.status,
    detail ? `HTTP ${response.status}: ${detail}` : `HTTP ${response.status}`,
    body,
  );
}

export async function request(
  input: RequestInfo | URL,
  init: Omit<RequestInit, "body"> & { body?: BodyInit | object | null } = {},
) {
  const headers = new Headers(init.headers);
  let body = init.body;
  if (
    body != null &&
    typeof body === "object" &&
    !(body instanceof Blob) &&
    !(body instanceof FormData) &&
    !(body instanceof URLSearchParams) &&
    !(body instanceof ArrayBuffer) &&
    !ArrayBuffer.isView(body)
  ) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }
  const response = await authenticatedFetch(input, {
    ...init,
    headers,
    body: body as BodyInit | null | undefined,
  });
  if (!response.ok) throw await responseError(response);
  return response;
}

export async function requestJson<T>(
  input: RequestInfo | URL,
  init: Omit<RequestInit, "body"> & { body?: BodyInit | object | null } = {},
) {
  const response = await request(input, init);
  return (await response.json()) as T;
}

export function withQuery(
  path: string,
  values: Record<string, string | number | boolean | undefined>,
) {
  const query = new URLSearchParams();
  Object.entries(values).forEach(([key, value]) => {
    if (value !== undefined && value !== "") query.set(key, String(value));
  });
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}
