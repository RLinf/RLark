export type IAMRole = "admin" | "operator" | "viewer";
export type IAMStatus = "active" | "disabled";
export type AuditResult = "success" | "failure";

export interface IAMUser {
  id: string;
  username: string;
  displayName: string;
  role: IAMRole;
  status: IAMStatus;
  createdAt: string;
  lastLoginAt: string | null;
  password: string;
  isDefaultAccount: boolean;
}

export interface IAMSession {
  id: string;
  userId: string;
  createdAt: string;
  revokedAt: string | null;
}

export interface AuditEvent {
  id: string;
  time: string;
  actor: string;
  action: string;
  target: string;
  result: AuditResult;
  ip: string;
  detail: string;
}

const hoursAgo = (hours: number) =>
  new Date(Date.now() - hours * 60 * 60 * 1000).toISOString();

export const initialUsers: IAMUser[] = [
  { id: "u_admin", username: "admin", displayName: "Administrator", role: "admin", status: "active", createdAt: hoursAgo(24 * 60), lastLoginAt: hoursAgo(2), password: "astribot", isDefaultAccount: true },
  { id: "u_user", username: "user", displayName: "Operator", role: "operator", status: "active", createdAt: hoursAgo(24 * 60), lastLoginAt: hoursAgo(3), password: "123", isDefaultAccount: true },
  { id: "u_lin", username: "lin.ops", displayName: "Lin", role: "operator", status: "active", createdAt: hoursAgo(24 * 20), lastLoginAt: hoursAgo(26), password: "mock-only-ops", isDefaultAccount: false },
  { id: "u_mira", username: "mira.view", displayName: "Mira", role: "viewer", status: "active", createdAt: hoursAgo(24 * 10), lastLoginAt: null, password: "mock-only-view", isDefaultAccount: false },
  { id: "u_old", username: "old.ops", displayName: "Former operator", role: "operator", status: "disabled", createdAt: hoursAgo(24 * 30), lastLoginAt: null, password: "mock-only-old", isDefaultAccount: false },
];

export const initialSessions: IAMSession[] = [
  { id: "session-admin-1", userId: "u_admin", createdAt: hoursAgo(2), revokedAt: null },
  { id: "session-user-1", userId: "u_user", createdAt: hoursAgo(3), revokedAt: null },
  { id: "session-lin-1", userId: "u_lin", createdAt: hoursAgo(26), revokedAt: null },
];

export const initialAuditEvents: AuditEvent[] = [
  { id: "evt-1", time: hoursAgo(1), actor: "user", action: "login", target: "user", result: "failure", ip: "192.0.2.32", detail: "Invalid credentials or disabled account." },
  { id: "evt-2", time: hoursAgo(2), actor: "admin", action: "login", target: "admin", result: "success", ip: "192.0.2.10", detail: "Sign-in completed." },
  { id: "evt-3", time: hoursAgo(3), actor: "user", action: "login", target: "user", result: "success", ip: "192.0.2.32", detail: "Sign-in completed." },
  { id: "evt-4", time: hoursAgo(20), actor: "admin", action: "disable", target: "old.ops", result: "success", ip: "192.0.2.10", detail: "User account disabled." },
  { id: "evt-5", time: hoursAgo(24 * 10), actor: "admin", action: "create", target: "mira.view", result: "success", ip: "192.0.2.10", detail: "Viewer account created." },
];
