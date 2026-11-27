import type { AuditEvent, IAMRole, IAMSession, IAMUser } from "./mockData";

export interface IAMUserDto {
  id: string;
  username: string;
  displayName: string;
  role: IAMRole;
  disabled: boolean;
  createdAt: string;
  updatedAt: string;
  lastLoginAt: string | null;
}

export interface IAMSessionDto {
  id: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  userAgent: string;
  ip: string;
  current: boolean;
  revokedAt: string | null;
}

export type IAMSessionView = IAMSession & Partial<Pick<IAMSessionDto, "lastSeenAt" | "expiresAt" | "userAgent" | "ip" | "current">>;

export interface IAMAuditEventDto {
  id: number;
  time: string;
  actor: string;
  actorId?: string;
  action: string;
  target: string;
  result: "success" | "failure" | "denied";
  ip: string;
  detail?: Record<string, unknown>;
}

export interface IAMLoginResponse {
  ok: boolean;
  token: string;
  role: IAMRole;
  expiresAt: string;
  user: IAMUserDto;
}

export interface IAMMeResponse {
  user: IAMUserDto;
}

export interface IAMUsersResponse {
  users: IAMUserDto[];
}

export interface IAMSessionsResponse {
  sessions: IAMSessionDto[];
}

export interface IAMAuditResponse {
  events: IAMAuditEventDto[];
}

export class IAMApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
    this.name = "IAMApiError";
  }
}

const tokenKey = "rlark-auth-token";

async function request<T>(path: string, init: Omit<RequestInit, "body"> & { body?: unknown } = {}): Promise<T> {
  const method = (init.method ?? "GET").toUpperCase();
  const headers = new Headers(init.headers);
  // Admin read endpoints also require a bearer token in the IAM server.
  const token = sessionStorage.getItem(tokenKey);
  if (token) headers.set("Authorization", `Bearer ${token}`);

  let body = init.body;
  if (body !== undefined && body !== null && typeof body !== "string" && !(body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
    body = JSON.stringify(body);
  }

  const response = await fetch(path, { ...init, method, headers, body: body as BodyInit | null | undefined, credentials: "include" });
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const errorBody = await response.json() as { message?: unknown; error?: unknown };
      if (typeof errorBody.message === "string") message = errorBody.message;
      else if (typeof errorBody.error === "string") message = errorBody.error;
    } catch {
      // Keep the status message when the error response has no JSON body.
    }
    throw new IAMApiError(response.status, message);
  }
  if (response.status === 204) return undefined as T;
  return response.json() as Promise<T>;
}

function userFromDto(dto: IAMUserDto): IAMUser {
  return {
    id: dto.id,
    username: dto.username,
    displayName: dto.displayName,
    role: dto.role,
    status: dto.disabled ? "disabled" : "active",
    createdAt: dto.createdAt,
    lastLoginAt: dto.lastLoginAt,
    password: "",
    isDefaultAccount: false,
  };
}

function sessionsFromResponse(response: IAMSessionsResponse, userId: string): IAMSessionView[] {
  return response.sessions.map((dto) => ({
    id: dto.id,
    userId,
    createdAt: dto.createdAt,
    revokedAt: dto.revokedAt,
    lastSeenAt: dto.lastSeenAt,
    expiresAt: dto.expiresAt,
    userAgent: dto.userAgent,
    ip: dto.ip,
    current: dto.current,
  }));
}

const auditActionMap: Record<string, string> = {
  "user-create": "create",
  "user-update": "edit",
  "user-disable": "disable",
  "user-enable": "enable",
  "user-delete": "delete",
  "password-reset": "reset",
  "password-change": "password",
  "revoke-sessions": "revokeSessions",
};

function eventFromDto(dto: IAMAuditEventDto): AuditEvent {
  return {
    id: String(dto.id),
    time: dto.time,
    actor: dto.actor,
    action: auditActionMap[dto.action] ?? dto.action,
    target: dto.target,
    result: dto.result === "success" ? "success" : "failure",
    ip: dto.ip,
    detail: dto.detail ? JSON.stringify(dto.detail) : "",
  };
}

export const iamApi = {
  login(username: string, password: string) {
    return request<IAMLoginResponse>("/api/iam/login", { method: "POST", body: { username, password } });
  },
  me() {
    return request<IAMMeResponse>("/api/iam/me");
  },
  logout() {
    return request<void>("/api/iam/logout", { method: "POST" });
  },
  async sessions(userId: string) {
    return sessionsFromResponse(await request<IAMSessionsResponse>("/api/iam/sessions"), userId);
  },
  revokeOtherSessions() {
    return request<void>("/api/iam/sessions/revoke-others", { method: "POST" });
  },
  async users() {
    const response = await request<IAMUsersResponse>("/api/iam/users");
    return response.users.map(userFromDto);
  },
  async createUser(input: { username: string; displayName: string; role: IAMRole; password: string }) {
    const response = await request<{ user: IAMUserDto }>("/api/iam/users", { method: "POST", body: input });
    return userFromDto(response.user);
  },
  async updateUser(id: string, input: { displayName: string; role: IAMRole; disabled?: boolean }) {
    const response = await request<{ user: IAMUserDto }>(`/api/iam/users/${encodeURIComponent(id)}`, { method: "PUT", body: input });
    return userFromDto(response.user);
  },
  removeUser(id: string) {
    return request<void>(`/api/iam/users/${encodeURIComponent(id)}`, { method: "DELETE" });
  },
  resetPassword(id: string, newPassword: string) {
    return request<void>(`/api/iam/users/${encodeURIComponent(id)}/reset-password`, { method: "POST", body: { newPassword } });
  },
  async audit() {
    const response = await request<IAMAuditResponse>("/api/iam/audit");
    return response.events.map(eventFromDto);
  },
  changePassword(input: { currentPassword: string; newPassword: string }) {
    return request<void>("/api/iam/password", { method: "POST", body: input });
  },
  mapUser: userFromDto,
};
