import { Fragment, useEffect, useMemo, useRef, useState, type FormEvent } from "react";
import { Activity, Box, Bot, Check, ChevronDown, CircleUserRound, ClipboardList, Database, Eye, EyeOff, Fingerprint, HardDrive, KeyRound, LockKeyhole, Network, Search, Server, Settings, ShieldCheck, Terminal, UserRound, UserRoundPlus, UsersRound, Workflow, X } from "lucide-react";
import { isMockMode } from "../../config";
import { clearAuthSession, storeAuthSession } from "../../api";
import { iamText, type IAMMessage } from "./i18n";
import { initialAuditEvents, initialSessions, initialUsers, type AuditEvent, type IAMRole, type IAMSession, type IAMUser } from "./mockData";
import { IAMApiError, iamApi, type IAMSessionView } from "./api";
import { iamPortalEnabled, returnToPortal, signOutOfConsole } from "./portal";
import "./iam.css";
import astribotDark from "./assets/astribot-logo-dark.png";
import astribotLight from "./assets/astribot-logo-light.png";

type Tab = "users" | "roles" | "audit" | "profile";
type Scenario = "normal" | "empty" | "loading" | "error";
type DialogKind = "create" | "edit" | "reset" | "disable" | "enable" | "delete";
type DialogState = { kind: DialogKind; userId?: string } | null;
type UserForm = { username: string; displayName: string; role: IAMRole; password: string };

const roleOrder: IAMRole[] = ["admin", "operator", "viewer"];
const platformModules: IAMMessage[] = ["jobs", "workflows", "tasks", "pods", "nodes", "clusters", "storage", "images", "robots", "iamUsers", "iamAudit", "settings"];
const iamModules: IAMMessage[] = ["iamUsers", "iamAudit", "settings"];
const userWriteModules = new Set<IAMMessage>(["jobs", "workflows", "tasks", "pods", "storage"]);
const roleDescriptions: Record<IAMRole, IAMMessage> = { admin: "roleCardAdmin", operator: "roleCardOperator", viewer: "roleCardViewer" };
const workloads: IAMMessage[] = ["jobs", "workflows", "tasks", "pods"];
const infrastructure: IAMMessage[] = ["nodes", "clusters", "storage", "images", "robots"];
const platform: IAMMessage[] = ["iamUsers", "iamAudit", "settings"];
const roleIcons = { admin: ShieldCheck, operator: Bot, viewer: Eye };
const moduleIcons: Partial<Record<IAMMessage, typeof Server>> = { jobs: ClipboardList, workflows: Workflow, tasks: Activity, pods: Box, nodes: Server, clusters: Network, storage: HardDrive, images: Database, robots: Bot, iamUsers: UsersRound, iamAudit: Fingerprint, settings: Settings };
const labels: Record<string, IAMMessage> = {
  login: "login", create: "createUser", edit: "edit", reset: "reset", disable: "disable", enable: "enable", delete: "delete", password: "changePassword",
};
const confirmationCopy: Record<Exclude<DialogKind, "create" | "edit" | "reset">, IAMMessage> = {
  disable: "disablePrompt", enable: "enablePrompt", delete: "deletePrompt",
};
const eventLabels: Record<string, IAMMessage> = {
  login: "login", logout: "logout", create: "createUser", edit: "edit", reset: "reset", disable: "disable", enable: "enable", delete: "delete", password: "changePassword", revokeSessions: "revokeSessions",
};

const text = (zh: boolean, key: IAMMessage) => iamText(zh, key);
const cloneUsers = () => initialUsers.map((user) => ({ ...user }));
const cloneSessions = () => initialSessions.map((session) => ({ ...session }));
const cloneEvents = () => initialAuditEvents.map((event) => ({ ...event }));
const roleLabel = (zh: boolean, role: IAMRole) => text(zh, role);
const dateLabel = (value: string | null, zh: boolean) => value
  && !Number.isNaN(Date.parse(value))
  ? new Intl.DateTimeFormat(zh ? "zh-CN" : "en-GB", { dateStyle: "medium", timeStyle: "short" }).format(new Date(value))
  : "—";
const createId = (prefix: string) => `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`;
const initials = (name: string) => name.trim().split(/\s+/).slice(0, 2).map((part) => part[0]?.toUpperCase() ?? "").join("");

function permissionLevel(role: IAMRole, module: IAMMessage): "full" | "write" | "read" | "none" {
  if (role === "admin") return "full";
  if (iamModules.includes(module)) return "none";
  if (role === "viewer") return "read";
  if (userWriteModules.has(module) || module === "robots") return "write";
  return "read";
}

export function IAMPage({ zh }: { zh: boolean }) {
  const [users, setUsers] = useState<IAMUser[]>(cloneUsers);
  const [sessions, setSessions] = useState<IAMSessionView[]>(cloneSessions);
  const [events, setEvents] = useState<AuditEvent[]>(cloneEvents);
  const [currentUserId, setCurrentUserId] = useState<string | null>(null);
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null);
  const [tab, setTab] = useState<Tab>("users");
  const [role, setRole] = useState<IAMRole>("admin");
  const [scenario, setScenario] = useState<Scenario>("normal");
  const [loginName, setLoginName] = useState("");
  const [loginPassword, setLoginPassword] = useState("");
  const [loginError, setLoginError] = useState("");
  const [loginLoading, setLoginLoading] = useState(false);
  const [apiLoading, setApiLoading] = useState(false);
  const [sessionChecked, setSessionChecked] = useState(false);
  const [apiError, setApiError] = useState("");
  const [userQuery, setUserQuery] = useState("");
  const [userRole, setUserRole] = useState<"" | IAMRole>("");
  const [userStatus, setUserStatus] = useState<"" | IAMUser["status"]>("");
  const [auditQuery, setAuditQuery] = useState("");
  const [auditActor, setAuditActor] = useState("");
  const [auditAction, setAuditAction] = useState("");
  const [auditResult, setAuditResult] = useState<"" | AuditEvent["result"]>("");
  const [auditTime, setAuditTime] = useState("");
  const [selectedUserId, setSelectedUserId] = useState<string | null>(null);
  const [selectedEventId, setSelectedEventId] = useState<string | null>(null);
  const [dialog, setDialog] = useState<DialogState>(null);
  const [form, setForm] = useState<UserForm>({ username: "", displayName: "", role: "operator", password: "" });
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [modalError, setModalError] = useState("");
  const [profileCurrent, setProfileCurrent] = useState("");
  const [profileNew, setProfileNew] = useState("");
  const [profileConfirm, setProfileConfirm] = useState("");
  const [profileError, setProfileError] = useState("");
  const [toast, setToast] = useState("");
  const [showLoginPassword, setShowLoginPassword] = useState(false);
  const [showProfilePasswords, setShowProfilePasswords] = useState(false);
  const [actionMenuUserId, setActionMenuUserId] = useState<string | null>(null);
  const [actionMenuOpen, setActionMenuOpen] = useState(false);
  const dialogRef = useRef<HTMLDivElement>(null);
  const drawerRef = useRef<HTMLDivElement>(null);
  const passwordFormRef = useRef<HTMLFormElement>(null);
  const drawerTriggerRef = useRef<HTMLElement | null>(null);
  const dialogTriggerRef = useRef<HTMLElement | null>(null);
  const toastTimerRef = useRef<number | undefined>(undefined);
  const loginTimerRef = useRef<number | undefined>(undefined);

  const handleApiError = (error: unknown) => {
    if (error instanceof IAMApiError && error.status === 401) {
      if (iamPortalEnabled()) {
        void returnToPortal(window.location.pathname + window.location.search);
        return;
      }
      const hadSession = Boolean(sessionStorage.getItem("rlark-auth-token"));
      clearAuthSession();
      setCurrentUserId(null);
      setCurrentSessionId(null);
      if (hadSession) setLoginError(text(zh, "loginExpired"));
      return;
    }
    setApiError(error instanceof Error ? error.message : text(zh, "requestFailed"));
  };

  useEffect(() => {
    if (isMockMode) return;
    let cancelled = false;
    setApiLoading(true);
    setApiError("");
    void (async () => {
      try {
        const me = await iamApi.me();
        if (cancelled) return;
        const user = iamApi.mapUser(me.user);
        setUsers([user]);
        setCurrentUserId(user.id);
        const requests: Promise<unknown>[] = [iamApi.sessions(user.id)];
        if (user.role === "admin") requests.push(iamApi.users(), iamApi.audit());
        const [loadedSessions, loadedUsers, loadedEvents] = await Promise.all(requests);
        if (cancelled) return;
        const userSessions = loadedSessions as IAMSessionView[];
        setSessions(userSessions);
        setCurrentSessionId(userSessions.find((session) => session.current)?.id ?? null);
        if (user.role === "admin") {
          setUsers(loadedUsers as IAMUser[]);
          setEvents(loadedEvents as AuditEvent[]);
        }
      } catch (error) {
        if (!cancelled) handleApiError(error);
      } finally {
        if (!cancelled) setApiLoading(false);
        if (!cancelled) setSessionChecked(true);
      }
    })();
    return () => { cancelled = true; };
  }, []);

  const currentUser = users.find((user) => user.id === currentUserId) ?? null;
  const isAdmin = currentUser?.role === "admin";
  const targetUser = dialog?.userId ? users.find((user) => user.id === dialog.userId) ?? null : null;
  const drawerUser = selectedUserId ? users.find((user) => user.id === selectedUserId) ?? null : null;
  const drawerEvent = selectedEventId ? events.find((event) => event.id === selectedEventId) ?? null : null;

  useEffect(() => () => {
    window.clearTimeout(toastTimerRef.current);
    window.clearTimeout(loginTimerRef.current);
  }, []);

  useEffect(() => {
    if (!toast) return;
    toastTimerRef.current = window.setTimeout(() => setToast(""), 4000);
    return () => window.clearTimeout(toastTimerRef.current);
  }, [toast]);

  useEffect(() => {
    if (!dialog) return;
    const dialogNode = dialogRef.current;
    const focusable = () => [...(dialogNode?.querySelectorAll<HTMLElement>("button:not(:disabled), input:not(:disabled), select:not(:disabled), [tabindex='0']") ?? [])]
      .filter((node) => !node.hasAttribute("hidden"));
    focusable()[0]?.focus();
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        closeDialog();
      }
      if (event.key !== "Tab") return;
      const nodes = focusable();
      if (nodes.length === 0) return;
      const first = nodes[0];
      const last = nodes[nodes.length - 1];
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [dialog]);

  useEffect(() => {
    if (dialog) return;
    dialogTriggerRef.current?.focus();
    dialogTriggerRef.current = null;
  }, [dialog]);

  useEffect(() => {
    if (!drawerUser && !drawerEvent) return;
    const drawer = drawerRef.current;
    const focusable = () => [...(drawer?.querySelectorAll<HTMLElement>("button:not(:disabled), [tabindex='0']") ?? [])];
    focusable()[0]?.focus();
    const onKeyDown = (event: globalThis.KeyboardEvent) => {
      if (event.key === "Escape") {
        event.preventDefault();
        setSelectedUserId(null);
        setSelectedEventId(null);
      }
      if (event.key !== "Tab") return;
      const nodes = focusable();
      if (!nodes.length) return;
      if (event.shiftKey && document.activeElement === nodes[0]) {
        event.preventDefault(); nodes[nodes.length - 1].focus();
      } else if (!event.shiftKey && document.activeElement === nodes[nodes.length - 1]) {
        event.preventDefault(); nodes[0].focus();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [drawerUser, drawerEvent]);

  const notify = (message: string) => setToast(message);
  const appendEvent = (action: string, target: string, result: AuditEvent["result"] = "success", detail?: string) => {
    const newEvent: AuditEvent = {
      id: createId("evt"), time: new Date().toISOString(), actor: currentUser?.username ?? "anonymous",
      action, target, result, ip: "192.0.2.20", detail: detail ?? `${action} ${target}`,
    };
    setEvents((previous) => [newEvent, ...previous]);
  };
  const revokeUserSessions = (userId: string) => {
    const now = new Date().toISOString();
    setSessions((previous) => previous.map((session) => session.userId === userId && !session.revokedAt ? { ...session, revokedAt: now } : session));
  };
  const openDialog = (kind: DialogKind, user?: IAMUser, trigger?: HTMLElement) => {
    dialogTriggerRef.current = trigger ?? document.activeElement as HTMLElement;
    setModalError("");
    setNewPassword("");
    setConfirmPassword("");
    setForm(user
      ? { username: user.username, displayName: user.displayName, role: user.role, password: "" }
      : { username: "", displayName: "", role: "operator", password: "" });
    setDialog({ kind, userId: user?.id });
  };
  const closeDialog = () => setDialog(null);

  const signIn = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setLoginError("");
    if (!loginName.trim() || !loginPassword) {
      setLoginError(text(zh, "required"));
      return;
    }
    setLoginLoading(true);
    if (!isMockMode) {
      let user: IAMUser;
      try {
        const result = await iamApi.login(loginName.trim(), loginPassword);
        if (!result.ok || !result.token || !["admin", "operator", "viewer"].includes(result.role)) throw new Error(text(zh, "invalidLoginResponse"));
        const nativeRole = result.role === "admin" ? "admin" : "user";
        clearAuthSession();
        storeAuthSession(result.token, nativeRole);
        sessionStorage.setItem(nativeRole === "admin" ? "rlark-admin-auth" : "rlark-user-auth", "1");
        sessionStorage.setItem(nativeRole === "admin" ? "rlark-admin-user-name" : "rlark-user-name", result.user.username);
        user = iamApi.mapUser(result.user);
        setUsers([user]);
        setCurrentUserId(user.id);
        setLoginPassword("");
        setTab(user.role === "admin" ? "users" : "profile");
        notify(text(zh, "signedIn"));
      } catch (error) {
        if (error instanceof IAMApiError && error.status === 403 && error.message === "account disabled") setLoginError(text(zh, "accountDisabled"));
        else if (error instanceof IAMApiError && error.status === 401) setLoginError(text(zh, "loginError"));
        else setLoginError(error instanceof Error ? error.message : text(zh, "requestFailed"));
        setLoginLoading(false);
        return;
      }
      try {
        const dataRequests: Promise<unknown>[] = [iamApi.sessions(user.id)];
        if (user.role === "admin") dataRequests.push(iamApi.users(), iamApi.audit());
        const [loadedSessions, loadedUsers, loadedEvents] = await Promise.all(dataRequests);
        const userSessions = loadedSessions as IAMSessionView[];
        setSessions(userSessions);
        setCurrentSessionId(userSessions.find((session) => session.current)?.id ?? null);
        if (user.role === "admin") {
          setUsers(loadedUsers as IAMUser[]);
          setEvents(loadedEvents as AuditEvent[]);
        }
      } catch (error) {
        handleApiError(error);
      } finally {
        setLoginLoading(false);
      }
      return;
    }
    loginTimerRef.current = window.setTimeout(() => {
      const user = users.find((entry) => entry.username === loginName.trim() && entry.password === loginPassword && entry.status === "active");
      if (!user) {
        const eventRow: AuditEvent = { id: createId("evt"), time: new Date().toISOString(), actor: loginName.trim(), action: "login", target: loginName.trim(), result: "failure", ip: "192.0.2.20", detail: "Invalid credentials or disabled account." };
        setEvents((previous) => [eventRow, ...previous]);
        setLoginError(text(zh, "loginError"));
        setLoginLoading(false);
        return;
      }
      const now = new Date().toISOString();
      setUsers((previous) => previous.map((entry) => entry.id === user.id ? { ...entry, lastLoginAt: now } : entry));
      const newSession: IAMSession = { id: createId("session"), userId: user.id, createdAt: now, revokedAt: null };
      setSessions((previous) => [...previous, newSession]);
      setCurrentSessionId(newSession.id);
      setEvents((previous) => [{ id: createId("evt"), time: now, actor: user.username, action: "login", target: user.username, result: "success", ip: "192.0.2.20", detail: "Sign-in completed." }, ...previous]);
      setCurrentUserId(user.id);
      setTab(user.role === "admin" ? "users" : "profile");
      setLoginPassword("");
      setLoginLoading(false);
      notify(text(zh, "signedIn"));
    }, 420);
  };

  const signOut = async () => {
    if (iamPortalEnabled()) {
      await signOutOfConsole(window.location.pathname);
      return;
    }
    if (!isMockMode) {
      setApiLoading(true);
      setApiError("");
      try {
        await iamApi.logout();
      } catch (error) {
        if (!(error instanceof IAMApiError && error.status === 401)) setApiError(error instanceof Error ? error.message : text(zh, "requestFailed"));
      } finally {
        clearAuthSession();
        setCurrentUserId(null);
        setCurrentSessionId(null);
        setTab("users");
        setLoginName("");
        setLoginPassword("");
        setSelectedUserId(null);
        setSelectedEventId(null);
        setApiLoading(false);
      }
      return;
    }
    if (currentUser) appendEvent("logout", currentUser.username);
    setCurrentUserId(null);
    setCurrentSessionId(null);
    setTab("users");
    setLoginName("");
    setLoginPassword("");
    setSelectedUserId(null);
    setSelectedEventId(null);
    notify(text(zh, "confirmSignout"));
  };

  // The native admin shell owns the avatar menu. On this page, its "Sign out"
  // must return to the Astribot IAM login instead of the native RLark login.
  const signOutRef = useRef(signOut);
  signOutRef.current = signOut;
  useEffect(() => {
    // Behind the portal the console itself keeps the native sign-out out (iam/portal.ts).
    if (iamPortalEnabled()) return;
    const onNativeSignOut = (event: MouseEvent) => {
      const target = event.target as HTMLElement | null;
      const button = target?.closest?.(".account-popover button");
      if (!button || !/退出登录|Sign out|Log out/i.test(button.textContent ?? "")) return;
      event.preventDefault();
      event.stopImmediatePropagation();
      // Close the native menu first; leaving it open lets a second click
      // reach the native handler after the IAM page has switched to its login.
      document.querySelector<HTMLButtonElement>(".topbar-menu .avatar")?.click();
      signOutRef.current();
    };
    document.addEventListener("click", onNativeSignOut, true);
    window.addEventListener("click", onNativeSignOut, true);
    return () => {
      document.removeEventListener("click", onNativeSignOut, true);
      window.removeEventListener("click", onNativeSignOut, true);
    };
  }, []);

  const saveUser = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!isAdmin || !dialog) return;
    const username = form.username.trim();
    const displayName = form.displayName.trim();
    if (!username || !displayName) { setModalError(text(zh, "required")); return; }
    if (dialog.kind === "create" && !/^[a-z0-9][a-z0-9_-]{2,31}$/.test(username)) { setModalError(text(zh, "badUsername")); return; }
    if (users.some((user) => user.username.toLowerCase() === username.toLowerCase() && user.id !== dialog.userId)) { setModalError(text(zh, "duplicate")); return; }
    if (dialog.kind === "create" && form.password.length < 8) { setModalError(text(zh, "minPassword")); return; }
    if (!isMockMode) {
      setApiLoading(true);
      setApiError("");
      setModalError("");
      try {
        if (dialog.kind === "create") {
          await iamApi.createUser({ username, displayName, role: form.role, password: form.password });
          setForm((previous) => ({ ...previous, password: "" }));
        } else if (dialog.kind === "edit" && targetUser) {
          await iamApi.updateUser(targetUser.id, { displayName, role: form.role });
        }
        const [freshUsers, freshEvents] = await Promise.all([iamApi.users(), iamApi.audit()]);
        setUsers(freshUsers);
        setEvents(freshEvents);
        notify(text(zh, dialog.kind === "create" ? "createSuccess" : "editSuccess"));
        closeDialog();
      } catch (error) {
        if (error instanceof IAMApiError && error.status !== 401) setModalError(error.message);
        handleApiError(error);
      } finally {
        setApiLoading(false);
      }
      return;
    }
    if (dialog.kind === "create") {
      const user: IAMUser = { id: createId("u"), username, displayName, role: form.role, status: "active", createdAt: new Date().toISOString(), lastLoginAt: null, password: form.password, isDefaultAccount: false };
      setUsers((previous) => [user, ...previous]);
      appendEvent("create", username);
      notify(text(zh, "createSuccess"));
    } else if (dialog.kind === "edit" && targetUser) {
      setUsers((previous) => previous.map((user) => user.id === targetUser.id ? { ...user, displayName, role: form.role } : user));
      appendEvent("edit", targetUser.username);
      notify(text(zh, "editSuccess"));
    }
    closeDialog();
  };

  const updateBackendUserStatus = async (user: IAMUser, status: IAMUser["status"]) => {
    setApiLoading(true);
    setApiError("");
    setModalError("");
    try {
      await iamApi.updateUser(user.id, { displayName: user.displayName, role: user.role, disabled: status === "disabled" });
      const [freshUsers, freshEvents] = await Promise.all([iamApi.users(), iamApi.audit()]);
      setUsers(freshUsers);
      setEvents(freshEvents);
      notify(text(zh, status === "active" ? "enableSuccess" : "disableSuccess"));
      closeDialog();
    } catch (error) {
      if (error instanceof IAMApiError && error.status !== 401) setModalError(error.message);
      handleApiError(error);
    } finally {
      setApiLoading(false);
    }
  };

  const confirmUserAction = async () => {
    if (!dialog || !targetUser || !isAdmin) return;
    if (dialog.kind === "reset") {
      if (newPassword.length < 8) { setModalError(text(zh, "minPassword")); return; }
      if (newPassword !== confirmPassword) { setModalError(text(zh, "mismatch")); return; }
      if (!isMockMode) {
        setApiLoading(true);
        setApiError("");
        setModalError("");
        try {
          await iamApi.resetPassword(targetUser.id, newPassword);
          setNewPassword("");
          setConfirmPassword("");
          const [freshUsers, freshSessions, freshEvents] = await Promise.all([iamApi.users(), iamApi.sessions(currentUser?.id ?? ""), iamApi.audit()]);
          setUsers(freshUsers);
          setSessions(freshSessions);
          setCurrentSessionId(freshSessions.find((session) => session.current)?.id ?? null);
          setEvents(freshEvents);
          notify(text(zh, "resetSuccess"));
          closeDialog();
        } catch (error) {
          if (error instanceof IAMApiError && error.status !== 401) setModalError(error.message);
          handleApiError(error);
        } finally {
          setApiLoading(false);
        }
        return;
      }
      setUsers((previous) => previous.map((user) => user.id === targetUser.id ? { ...user, password: newPassword, isDefaultAccount: false } : user));
      revokeUserSessions(targetUser.id);
      appendEvent("reset", targetUser.username, "success", "Administrator reset password and revoked sessions.");
      notify(text(zh, "resetSuccess"));
    } else if (dialog.kind === "disable") {
      if (!isMockMode) {
        await updateBackendUserStatus(targetUser, "disabled");
        return;
      }
      setUsers((previous) => previous.map((user) => user.id === targetUser.id ? { ...user, status: "disabled" } : user));
      revokeUserSessions(targetUser.id);
      appendEvent("disable", targetUser.username);
      notify(text(zh, "disableSuccess"));
    } else if (dialog.kind === "enable") {
      if (!isMockMode) {
        await updateBackendUserStatus(targetUser, "active");
        return;
      }
      setUsers((previous) => previous.map((user) => user.id === targetUser.id ? { ...user, status: "active" } : user));
      appendEvent("enable", targetUser.username);
      notify(text(zh, "enableSuccess"));
    } else if (dialog.kind === "delete") {
      if (!isMockMode) {
        setApiLoading(true);
        setApiError("");
        setModalError("");
        try {
          await iamApi.removeUser(targetUser.id);
          const [freshUsers, freshEvents] = await Promise.all([iamApi.users(), iamApi.audit()]);
          setUsers(freshUsers);
          setEvents(freshEvents);
          setSelectedUserId((current) => current === targetUser.id ? null : current);
          notify(text(zh, "deleteSuccess"));
          closeDialog();
        } catch (error) {
          if (error instanceof IAMApiError && error.status !== 401) setModalError(error.message);
          handleApiError(error);
        } finally {
          setApiLoading(false);
        }
        return;
      }
      setUsers((previous) => previous.filter((user) => user.id !== targetUser.id));
      revokeUserSessions(targetUser.id);
      appendEvent("delete", targetUser.username);
      setSelectedUserId((current) => current === targetUser.id ? null : current);
      notify(text(zh, "deleteSuccess"));
    }
    closeDialog();
  };

  const saveProfilePassword = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!currentUser) return;
    setProfileError("");
    if (!profileCurrent || !profileNew || !profileConfirm) { setProfileError(text(zh, "required")); return; }
    if (isMockMode && currentUser.password !== profileCurrent) { setProfileError(text(zh, "wrongCurrent")); return; }
    if (profileNew.length < 8) { setProfileError(text(zh, "minPassword")); return; }
    if (profileNew !== profileConfirm) { setProfileError(text(zh, "mismatch")); return; }
    if (profileNew === profileCurrent) { setProfileError(text(zh, "samePassword")); return; }
    if (!isMockMode) {
      setApiLoading(true);
      setApiError("");
      try {
        await iamApi.changePassword({ currentPassword: profileCurrent, newPassword: profileNew });
        setProfileCurrent(""); setProfileNew(""); setProfileConfirm("");
        if (iamPortalEnabled()) {
          void signOutOfConsole("/admin/iam");
          return;
        }
        notify(text(zh, "passwordUpdated"));
        setLoginName(currentUser.username);
        clearAuthSession();
        setCurrentUserId(null);
        setCurrentSessionId(null);
      } catch (error) {
        setProfileError(error instanceof Error ? error.message : text(zh, "requestFailed"));
        handleApiError(error);
      } finally {
        setApiLoading(false);
      }
      return;
    }
    setUsers((previous) => previous.map((user) => user.id === currentUser.id ? { ...user, password: profileNew, isDefaultAccount: false } : user));
    appendEvent("password", currentUser.username);
    setProfileCurrent(""); setProfileNew(""); setProfileConfirm("");
    notify(text(zh, "passwordUpdated"));
  };

  const revokeOtherSessions = async () => {
    if (!currentUser) return;
    const now = new Date().toISOString();
    const others = sessions.filter((session) => session.userId === currentUser.id && !session.revokedAt && session.id !== currentSessionId);
    if (!others.length) return;
    if (!isMockMode) {
      setApiLoading(true);
      setApiError("");
      try {
        await iamApi.revokeOtherSessions();
        const refreshedSessions = await iamApi.sessions(currentUser.id);
        setSessions(refreshedSessions);
        setCurrentSessionId(refreshedSessions.find((session) => session.current)?.id ?? null);
        notify(zh ? "其他会话已撤销。" : "Other sessions revoked.");
      } catch (error) {
        handleApiError(error);
      } finally {
        setApiLoading(false);
      }
      return;
    }
    setSessions((previous) => previous.map((session) => session.userId === currentUser.id && session.id !== currentSessionId && !session.revokedAt ? { ...session, revokedAt: now } : session));
    notify(zh ? "其他会话已撤销。" : "Other sessions revoked.");
  };

  const filteredUsers = useMemo(() => users.filter((user) => {
    const query = userQuery.trim().toLowerCase();
    return (!userRole || user.role === userRole) && (!userStatus || user.status === userStatus) &&
      (!query || user.username.toLowerCase().includes(query) || user.displayName.toLowerCase().includes(query));
  }), [users, userQuery, userRole, userStatus]);

  const actors = useMemo(() => [...new Set(events.map((event) => event.actor))].sort(), [events]);
  const filteredEvents = useMemo(() => events.filter((event) => {
    const query = auditQuery.trim().toLowerCase();
    const withinTime = !auditTime || Date.parse(event.time) >= Date.now() - Number(auditTime) * 60 * 60 * 1000;
    const queryMatch = !query || [event.actor, event.target, event.ip, event.detail].some((value) => value.toLowerCase().includes(query));
    return queryMatch && (!auditActor || event.actor === auditActor) && (!auditAction || event.action === auditAction) &&
      (!auditResult || event.result === auditResult) && withinTime;
  }), [events, auditQuery, auditActor, auditAction, auditResult, auditTime]);

  const isScenario = (value: "empty" | "loading" | "error") => scenario === value;
  const activeSessions = (userId: string) => sessions.filter((session) => session.userId === userId && !session.revokedAt).length;

  const closeDrawer = () => {
    setSelectedUserId(null);
    setSelectedEventId(null);
    drawerTriggerRef.current?.focus();
    drawerTriggerRef.current = null;
  };
  const openUserAction = (kind: DialogKind, user: IAMUser, trigger: HTMLElement) => {
    setActionMenuOpen(false);
    openDialog(kind, user, trigger);
  };

  const renderStats = (items: Array<{ label: string; value: number | string; icon: typeof UsersRound; tone?: string }>) => (
    <div className="iam-stats">{items.map(({ label, value, icon: Icon, tone }) => <div className="iam-stat" key={label}>
      <span className={`iam-stat-icon ${tone ?? ""}`}><Icon size={17} /></span><span className="iam-stat-copy"><small>{label}</small><strong>{value}</strong></span>
    </div>)}</div>
  );

  const renderLogin = () => (
    <section className="iam-login-screen" aria-label={text(zh, "loginTitle")}>
      <div className="iam-login-brand-panel">
        <div className="iam-brand-top">
          <div className="iam-login-brand">
            <img src={astribotLight} alt="Astribot" />
            <img src={zh ? "/rlark-logo-zh-light.png" : "/rlark-logo-en-light.png"} alt="RLark" />
          </div>
          {isMockMode && <span className="iam-mock-badge"><i />{text(zh, "mockBadge")}</span>}
        </div>
        <div className="iam-brand-foot">
          <div className="iam-brand-links">
            <a href="https://www.astribot.com/">{zh ? "Astribot 官网" : "Astribot website"} ↗</a>
            <a href="https://github.com/RLinf/RLark">{zh ? "RLark 开源" : "RLark on GitHub"} ↗</a>
          </div>
        </div>
      </div>
      <div className="iam-login-form-panel"><div className="iam-login-card">
        <div className="iam-login-heading"><span className="iam-eyebrow">{text(zh, "signInEyebrow")}</span><h2>{text(zh, "loginTitle")}</h2><p>{text(zh, "loginHelp")}</p></div>
        <form id="login-form" className="iam-form" onSubmit={signIn}>
          <label className="iam-field"><span>{text(zh, "username")}</span><span className="iam-login-input"><UserRound size={17} /><input autoComplete="username" value={loginName} onChange={(event) => setLoginName(event.target.value)} /></span></label>
          <label className="iam-field"><span>{text(zh, "password")}</span><span className="iam-login-input"><LockKeyhole size={17} /><input type={showLoginPassword ? "text" : "password"} autoComplete="current-password" value={loginPassword} onChange={(event) => setLoginPassword(event.target.value)} /><button type="button" className="iam-visibility-button" aria-label={showLoginPassword ? text(zh, "hidePassword") : text(zh, "showPassword")} onClick={() => setShowLoginPassword((value) => !value)}>{showLoginPassword ? <EyeOff size={17} /> : <Eye size={17} />}</button></span></label>
          {loginError && <p className="iam-error iam-login-error" role="alert">{loginError}</p>}
          <button className="iam-primary iam-login-submit" type="submit" disabled={loginLoading}>{loginLoading ? <><span className="iam-spinner" />{text(zh, "loggingIn")}</> : <>{text(zh, "login")}<span aria-hidden="true">↗</span></>}</button>
        </form>
        {isMockMode && <div className="iam-demo-accounts"><div className="iam-demo-title"><h3>{text(zh, "defaultAccounts")}</h3><span className="iam-demo-badge">{text(zh, "demoBadge")}</span></div>
          <div className="iam-demo-row"><code><span>ADMIN</span>admin / astribot</code><button type="button" onClick={() => { setLoginName("admin"); setLoginPassword("astribot"); }}>{text(zh, "fillAdmin")}</button></div>
          <div className="iam-demo-row"><code><span>USER</span>user / 123</code><button type="button" onClick={() => { setLoginName("user"); setLoginPassword("123"); }}>{text(zh, "fillUser")}</button></div>
          <p className="iam-warning" role="note"><LockKeyhole size={15} />{text(zh, "weakPassword")}</p>
        </div>}
        <div className="iam-form-cobrand">
          <img src={astribotLight} alt="Astribot" className="iam-cobrand-light" />
          <img src={astribotDark} alt="Astribot" className="iam-cobrand-dark" />
          <span>for RLark</span>
        </div>
      </div></div>
    </section>
  );

  const renderUsers = () => {
    const scenarioBody = isScenario("loading")
      ? <div className="iam-state" aria-busy="true"><span className="iam-spinner" />{text(zh, "loading")}</div>
      : isScenario("error")
        ? <div className="iam-state"><p>{text(zh, "loadError")}</p><button type="button" onClick={() => setScenario("normal")}>{text(zh, "retry")}</button></div>
        : isScenario("empty")
          ? <div className="iam-state"><UsersRound size={30} /><strong>{text(zh, "noUsers")}</strong><p>{text(zh, "emptyUsers")}</p></div>
          : filteredUsers.length === 0
            ? <div className="iam-state"><Search size={26} /><strong>{text(zh, "noMatch")}</strong><button type="button" onClick={() => { setUserQuery(""); setUserRole(""); setUserStatus(""); }}>{text(zh, "cancel")}</button></div>
          : <div className="iam-table-wrap"><table className="iam-table iam-users-table"><thead><tr>{(["userColumn", "role", "status", "lastLogin", "created", "actions"] as const).map((key) => <th key={key} scope="col">{text(zh, key)}</th>)}</tr></thead><tbody>
              {filteredUsers.map((user) => <tr key={user.id} className="iam-clickable-row" tabIndex={0} aria-label={`${text(zh, "userDetails")}: ${user.username}`} onClick={(event) => { drawerTriggerRef.current = event.currentTarget; setActionMenuOpen(false); setSelectedUserId(user.id); }} onKeyDown={(event) => { if (event.key === "Enter") { drawerTriggerRef.current = event.currentTarget; setSelectedUserId(user.id); } }}>
                <td data-label={text(zh, "userColumn")}><span className={`iam-user-cell ${user.role === "admin" ? "is-admin" : ""}`}><span className="iam-avatar">{initials(user.displayName || user.username)}</span><span><strong>{user.username}</strong><small>{user.displayName}</small></span></span></td>
                <td data-label={text(zh, "role")}><span className={`iam-pill ${user.role === "admin" ? "is-admin" : ""}`}>{roleLabel(zh, user.role)}</span></td>
                <td data-label={text(zh, "status")}><span className={`iam-status ${user.status}`}><i />{text(zh, user.status)}</span></td>
                <td data-label={text(zh, "lastLogin")}>{dateLabel(user.lastLoginAt, zh)}</td>
                <td data-label={text(zh, "created")}>{dateLabel(user.createdAt, zh)}</td>
                <td data-label={text(zh, "actions")} onClick={(event) => event.stopPropagation()}>
                  {user.id === currentUserId ? <span className="iam-self-pill">{text(zh, "you")}</span> : <div className="iam-action-menu">
                    <button type="button" aria-expanded={actionMenuOpen && actionMenuUserId === user.id} aria-haspopup="menu" onClick={(event) => { event.stopPropagation(); setActionMenuUserId(user.id); setActionMenuOpen((open) => !open || actionMenuUserId !== user.id); }}>{text(zh, "actions")} <ChevronDown size={14} /></button>
                    {actionMenuOpen && actionMenuUserId === user.id && <div className="iam-action-popover" role="menu">
                      <button type="button" role="menuitem" onClick={(event) => openUserAction("edit", user, event.currentTarget.closest(".iam-action-menu")?.querySelector("button") ?? event.currentTarget)}>{text(zh, "edit")}</button>
                      <button type="button" role="menuitem" onClick={(event) => openUserAction("reset", user, event.currentTarget.closest(".iam-action-menu")?.querySelector("button") ?? event.currentTarget)}>{text(zh, "reset")}</button>
                      <button type="button" role="menuitem" onClick={(event) => openUserAction(user.status === "active" ? "disable" : "enable", user, event.currentTarget.closest(".iam-action-menu")?.querySelector("button") ?? event.currentTarget)}>{text(zh, user.status === "active" ? "disable" : "enable")}</button>
                      <button type="button" role="menuitem" className="is-danger" onClick={(event) => openUserAction("delete", user, event.currentTarget.closest(".iam-action-menu")?.querySelector("button") ?? event.currentTarget)}>{text(zh, "delete")}</button>
                    </div>}
                  </div>}
                </td>
              </tr>)}
            </tbody></table></div>;
    return <div className="iam-content">
      <div className="iam-page-heading"><div><span className="iam-eyebrow">{text(zh, "managementEyebrow")}</span><h1>{text(zh, "users")}</h1><p>{text(zh, "userDescription")}</p></div><button className="iam-primary" type="button" onClick={(event) => openDialog("create", undefined, event.currentTarget)}><UserRoundPlus size={16} />{text(zh, "createUser")}</button></div>
      {renderStats([
        { label: text(zh, "totalUsers"), value: users.length, icon: UsersRound },
        { label: text(zh, "enabledUsers"), value: users.filter((user) => user.status === "active").length, icon: Check, tone: "success" },
        { label: text(zh, "adminCount"), value: users.filter((user) => user.role === "admin").length, icon: ShieldCheck, tone: "accent" },
        { label: text(zh, "disabledUsers"), value: users.filter((user) => user.status === "disabled").length, icon: X, tone: "muted" },
      ])}
      <div className="iam-toolbar">
        <label className="iam-search"><Search size={17} /><span className="iam-visually-hidden">{text(zh, "searchUsers")}</span><input value={userQuery} onChange={(event) => setUserQuery(event.target.value)} placeholder={text(zh, "searchUsers")} type="search" /></label>
        <label className="iam-filter"><span>{text(zh, "role")}</span><select value={userRole} onChange={(event) => setUserRole(event.target.value as "" | IAMRole)}><option value="">{text(zh, "allRoles")}</option>{roleOrder.map((entry) => <option key={entry} value={entry}>{roleLabel(zh, entry)}</option>)}</select></label>
        <label className="iam-filter"><span>{text(zh, "status")}</span><select value={userStatus} onChange={(event) => setUserStatus(event.target.value as "" | IAMUser["status"])}><option value="">{text(zh, "allStatuses")}</option><option value="active">{text(zh, "active")}</option><option value="disabled">{text(zh, "disabled")}</option></select></label>
      </div>
      <section className="iam-panel iam-users-panel" aria-live="polite"><div className="iam-panel-heading"><div><strong>{text(zh, "users")}</strong><small>{text(zh, "rowClickHint")}</small></div><span>{filteredUsers.length} / {users.length}</span></div>{scenarioBody}<div className="iam-table-count">{filteredUsers.length} {text(zh, "count")}</div></section>
    </div>;
  };

  const renderRoles = () => (
    <div className="iam-content">
      <div className="iam-page-heading"><div><span className="iam-eyebrow">{text(zh, "managementEyebrow")}</span><h1>{text(zh, "roles")}</h1><p>{text(zh, "roleDescription")}</p></div></div>
      <div className="iam-role-workspace">
        <div className="iam-role-rail" aria-label={text(zh, "role")}>
          {roleOrder.map((entry) => {
            const Icon = roleIcons[entry];
            const count = platformModules.filter((module) => permissionLevel(entry, module) !== "none").length;
            return <button type="button" key={entry} className={`iam-role-card ${role === entry ? "is-selected" : ""}`} aria-pressed={role === entry} onClick={() => setRole(entry)}>
              <span className="iam-role-icon"><Icon size={18} /></span><span className="iam-role-copy"><strong>{roleLabel(zh, entry)}</strong><small>{text(zh, roleDescriptions[entry])}</small><em>{count} {text(zh, "permissionCount")}</em></span>
            </button>;
          })}
        </div>
        <section className="iam-panel iam-compare-panel">
          <div className="iam-panel-heading"><div><strong>{text(zh, "roles")}</strong><small>{text(zh, "fullAccess")} · {text(zh, "readWrite")} · {text(zh, "readOnly")} · {text(zh, "noAccess")}</small></div><span className="iam-active-role"><ShieldCheck size={15} />{roleLabel(zh, role)}</span></div>
          <div className="iam-matrix-scroll"><table className="iam-compare-table"><thead><tr><th scope="col">{text(zh, "module")}</th>{roleOrder.map((entry) => <th scope="col" key={entry} className={entry === role ? "is-selected" : ""}>{roleLabel(zh, entry)}</th>)}</tr></thead><tbody>
            {([["workloads", workloads], ["infrastructure", infrastructure], ["platform", platform]] as const).map(([group, modules]) => <Fragment key={group}>
              <tr className="iam-group-row"><th colSpan={4}>{text(zh, group)}</th></tr>
              {modules.map((module) => { const Icon = moduleIcons[module] ?? Box; return <tr key={module}>
                <th scope="row"><span className="iam-module-name"><Icon size={15} />{text(zh, module)}</span></th>
                {roleOrder.map((entry) => { const level = permissionLevel(entry, module); return <td key={entry} className={entry === role ? "is-selected" : ""}><span className={`iam-access-chip ${level}`}><i />{text(zh, level === "full" ? "fullAccess" : level === "write" ? "readWrite" : level === "read" ? "readOnly" : "noAccess")}</span></td>; })}
              </tr>; })}
            </Fragment>)}
          </tbody></table></div>
          <p className="iam-note"><span><Bot size={15} /></span>{text(zh, "robotPermissions")} {text(zh, "terminalsAllowed")}</p>
        </section>
        <section className="iam-panel iam-iam-permissions"><div className="iam-iam-card-icon"><KeyRound size={18} /></div><div><h2>{text(zh, "iamPermissions")}</h2><p>{text(zh, "iamAdminOnly")}</p><div className="iam-iam-tags">{iamModules.map((module) => <span key={module}>{text(zh, module)}</span>)}</div></div></section>
      </div>
    </div>
  );

  const renderAudit = () => {
    const today = new Date().toDateString();
    const todayEvents = events.filter((entry) => new Date(entry.time).toDateString() === today);
    const recentActors = new Set(events.filter((entry) => Date.now() - Date.parse(entry.time) < 7 * 24 * 60 * 60 * 1000).map((entry) => entry.actor)).size;
    const auditScenario = scenario === "loading"
      ? <div className="iam-state" aria-busy="true"><span className="iam-spinner" />{text(zh, "loading")}</div>
      : scenario === "error"
        ? <div className="iam-state"><p>{text(zh, "loadError")}</p><button type="button" onClick={() => setScenario("normal")}>{text(zh, "retry")}</button></div>
        : scenario === "empty"
          ? <div className="iam-state"><ClipboardList size={28} /><strong>{text(zh, "noAudit")}</strong></div>
          : filteredEvents.length === 0
            ? <div className="iam-state"><Search size={26} /><strong>{text(zh, "noMatch")}</strong></div>
            : <div className="iam-table-wrap"><table className="iam-table iam-audit-table"><thead><tr>{(["time", "actor", "action", "target", "result", "ip"] as const).map((key) => <th key={key} scope="col">{text(zh, key)}</th>)}</tr></thead><tbody>{filteredEvents.map((entry) => <tr key={entry.id} className="iam-clickable-row" tabIndex={0} aria-label={`${text(zh, "eventDetails")}: ${entry.action} · ${entry.target}`} onClick={(event) => { drawerTriggerRef.current = event.currentTarget; setSelectedEventId(entry.id); }} onKeyDown={(event) => { if (event.key === "Enter") { drawerTriggerRef.current = event.currentTarget; setSelectedEventId(entry.id); } }}>
                <td data-label={text(zh, "time")}>{dateLabel(entry.time, zh)}</td><td data-label={text(zh, "actor")}><span className="iam-actor-cell"><span className="iam-avatar is-small">{initials(entry.actor)}</span>{entry.actor}</span></td>
                <td data-label={text(zh, "action")}>{text(zh, eventLabels[entry.action] ?? "action")}</td><td data-label={text(zh, "target")}>{entry.target}</td>
                <td data-label={text(zh, "result")}><span className={`iam-pill ${entry.result === "success" ? "is-active" : "is-disabled"}`}>{text(zh, entry.result)}</span></td>
                <td data-label={text(zh, "ip")}><code>{entry.ip}</code></td>
              </tr>)}</tbody></table></div>;
    return <div className="iam-content"><div className="iam-page-heading"><div><span className="iam-eyebrow">{text(zh, "managementEyebrow")}</span><h1>{text(zh, "audit")}</h1><p>{text(zh, "auditDescription")}</p></div></div>
      {renderStats([
        { label: text(zh, "todayEvents"), value: todayEvents.length, icon: Activity },
        { label: text(zh, "failedEvents"), value: events.filter((entry) => entry.result === "failure").length, icon: X, tone: "danger" },
        { label: text(zh, "activeActors"), value: recentActors, icon: UsersRound, tone: "accent" },
        { label: text(zh, "retentionDays"), value: 90, icon: ClipboardList },
      ])}
      <div className="iam-toolbar iam-audit-toolbar">
        <label className="iam-search"><Search size={17} /><span className="iam-visually-hidden">{text(zh, "searchAudit")}</span><input type="search" value={auditQuery} onChange={(event) => setAuditQuery(event.target.value)} placeholder={text(zh, "searchAudit")} /></label>
        <label className="iam-filter"><span>{text(zh, "actor")}</span><select value={auditActor} onChange={(event) => setAuditActor(event.target.value)}><option value="">{text(zh, "allActors")}</option>{actors.map((actor) => <option key={actor}>{actor}</option>)}</select></label>
        <label className="iam-filter"><span>{text(zh, "action")}</span><select value={auditAction} onChange={(event) => setAuditAction(event.target.value)}><option value="">{text(zh, "allActions")}</option>{Object.keys(eventLabels).map((action) => <option key={action} value={action}>{text(zh, eventLabels[action])}</option>)}</select></label>
        <label className="iam-filter"><span>{text(zh, "result")}</span><select value={auditResult} onChange={(event) => setAuditResult(event.target.value as "" | AuditEvent["result"])}><option value="">{text(zh, "allResults")}</option><option value="success">{text(zh, "success")}</option><option value="failure">{text(zh, "failure")}</option></select></label>
        <label className="iam-filter"><span>{text(zh, "time")}</span><select value={auditTime} onChange={(event) => setAuditTime(event.target.value)}><option value="">{text(zh, "anyTime")}</option><option value="24">{text(zh, "day")}</option><option value="168">{text(zh, "week")}</option></select></label>
      </div><section className="iam-panel iam-audit-panel" aria-live="polite"><div className="iam-panel-heading"><div><strong>{text(zh, "audit")}</strong><small>{text(zh, "eventDrawerHint")}</small></div><span>{filteredEvents.length}</span></div>{auditScenario}<div className="iam-table-count">{filteredEvents.length} {text(zh, "eventCountSuffix")}</div></section>
    </div>;
  };

  const renderProfile = () => currentUser && <div className="iam-content iam-profile-content">
    <div className="iam-page-heading"><div><span className="iam-eyebrow">{text(zh, "businessEyebrow")}</span><h1>{text(zh, "profile")}</h1><p>{text(zh, "profileDescription")}</p></div></div>
    <section className="iam-profile-hero" aria-label={text(zh, "accountInfo")}>
      <div className="iam-profile-hero-main"><span className="iam-profile-hero-avatar" aria-hidden="true">{initials(currentUser.displayName || currentUser.username)}</span><div className="iam-profile-hero-copy">
        <h2>{currentUser.displayName}</h2><span className="iam-profile-username">@{currentUser.username}</span>
        <div className="iam-profile-badges"><span className={`iam-profile-role ${currentUser.role}`}>{roleLabel(zh, currentUser.role)}</span><span className={`iam-profile-status ${currentUser.status}`}><i />{text(zh, currentUser.status)}</span></div>
        {currentUser.isDefaultAccount && <div className="iam-hero-password-note"><span><LockKeyhole size={15} />{text(zh, "defaultPassword")}</span><button type="button" onClick={() => { const input = passwordFormRef.current?.querySelector<HTMLInputElement>("input[autocomplete='current-password']"); input?.scrollIntoView({ block: "center" }); input?.focus({ preventScroll: true }); }}>{zh ? "立即修改" : "Change now"}</button></div>}
      </div></div>
      <dl className="iam-profile-hero-facts"><div><dt>{text(zh, "created")}</dt><dd>{dateLabel(currentUser.createdAt, zh)}</dd></div><div><dt>{text(zh, "lastLogin")}</dt><dd>{dateLabel(currentUser.lastLoginAt, zh)}</dd></div><div><dt>{text(zh, "activeSessionCount")}</dt><dd className="iam-profile-fact-number">{activeSessions(currentUser.id)}</dd></div><div><dt>{text(zh, "status")}</dt><dd><span className={`iam-profile-status ${currentUser.status}`}><i />{text(zh, currentUser.status)}</span></dd></div></dl>
    </section>
    {currentUser.role !== "admin" && <p className="iam-profile-guidance">{text(zh, "ordinaryUserNote")}</p>}
    <div className="iam-profile-settings">
      <section className="iam-panel iam-profile-card iam-password-card">
        <div className="iam-card-heading"><span className="iam-card-icon"><KeyRound size={18} /></span><div><h2>{text(zh, "changePassword")}</h2><p>{text(zh, "passwordStrength")}</p></div></div>
        <form className="iam-form iam-password-form" ref={passwordFormRef} onSubmit={saveProfilePassword}>
          <label className="iam-field"><span>{text(zh, "currentPassword")}</span><input type={showProfilePasswords ? "text" : "password"} autoComplete="current-password" value={profileCurrent} onChange={(event) => setProfileCurrent(event.target.value)} /></label>
          <div className="iam-password-pair">
            <label className="iam-field"><span>{text(zh, "newPassword")}</span><input type={showProfilePasswords ? "text" : "password"} autoComplete="new-password" value={profileNew} onChange={(event) => setProfileNew(event.target.value)} /></label>
            <label className="iam-field"><span>{text(zh, "confirmPassword")}</span><input type={showProfilePasswords ? "text" : "password"} autoComplete="new-password" value={profileConfirm} onChange={(event) => setProfileConfirm(event.target.value)} /></label>
          </div>
          <ul className="iam-password-rules" aria-label={zh ? "密码要求" : "Password requirements"}>
            <li className={profileNew.length >= 8 ? "is-met" : ""}><Check size={14} />{zh ? "至少 8 个字符" : "At least 8 characters"}</li>
            <li className={profileConfirm && profileNew === profileConfirm ? "is-met" : ""}><Check size={14} />{zh ? "两次输入一致" : "Passwords match"}</li>
            <li className={profileNew && profileNew !== profileCurrent ? "is-met" : ""}><Check size={14} />{zh ? "与当前密码不同" : "Different from current password"}</li>
          </ul>
          <label className="iam-check"><input type="checkbox" checked={showProfilePasswords} onChange={(event) => setShowProfilePasswords(event.target.checked)} />{zh ? "显示密码" : "Show passwords"}</label>
          {profileError && <p className="iam-error" role="alert">{profileError}</p>}<button className="iam-primary iam-profile-save" type="submit" disabled={apiLoading}>{text(zh, "changePassword")}</button>
        </form>
      </section>
      <section className="iam-panel iam-profile-card iam-security-card">
        <div className="iam-card-heading"><span className="iam-card-icon is-success"><ShieldCheck size={18} /></span><div><h2>{zh ? "安全与会话" : "Security & sessions"}</h2></div></div>
        <div className="iam-security-status-row"><span className={`iam-profile-status ${currentUser.status}`}><i />{text(zh, currentUser.status === "active" ? "protectedAccount" : "disabled")}</span><span>{activeSessions(currentUser.id)} {text(zh, "activeSession")}</span></div>
        <div className="iam-session-heading"><strong>{text(zh, "sessionList")}</strong><button type="button" disabled={apiLoading || activeSessions(currentUser.id) <= (currentSessionId ? 1 : 0)} onClick={revokeOtherSessions}>{zh ? "撤销其他会话" : "Revoke other sessions"}</button></div>
        <div className="iam-session-list iam-profile-session-list">{sessions.filter((session) => session.userId === currentUser.id).length === 0 ? <p>{text(zh, "noSessions")}</p> : sessions.filter((session) => session.userId === currentUser.id).map((session) => {
          const current = session.id === currentSessionId;
          const mockIp = session.id === "session-admin-1" ? "192.0.2.10" : session.id === "session-user-1" ? "192.0.2.32" : session.id === "session-lin-1" ? "192.0.2.51" : "192.0.2.20";
          const ip = session.ip ?? (isMockMode ? mockIp : "—");
          return <div className={`iam-profile-session-row ${current ? "is-current" : ""}`} key={session.id}>
            <span className={`iam-session-dot ${session.revokedAt ? "is-revoked" : ""}`} />
            <span className="iam-profile-session-copy"><strong>{session.userAgent || (zh ? "浏览器会话" : "Browser session")}{current && <em>{zh ? "当前" : "Current"}</em>}</strong><small>{text(zh, "lastSeen")} · {dateLabel(session.lastSeenAt ?? session.createdAt, zh)}</small><small>IP · {ip}</small></span>
            <span className={`iam-session-state ${session.revokedAt ? "is-revoked" : ""}`}>{text(zh, session.revokedAt ? "revokedSession" : "activeSession")}</span>
          </div>;
        })}</div>
      </section>
      <section className="iam-panel iam-profile-card iam-profile-activity" aria-labelledby="iam-profile-activity-title">
        <div className="iam-profile-activity-heading"><h2 id="iam-profile-activity-title">{zh ? "最近活动" : "Recent activity"}</h2><button type="button" onClick={() => setTab("audit")}>{zh ? "查看全部审计" : "View all audit"}<span aria-hidden="true">↗</span></button></div>
        {(() => {
          const recentEvents = events.filter((event) => event.actor === currentUser.username || event.target === currentUser.username).slice(0, 6);
          return recentEvents.length ? <ul className="iam-profile-activity-list">{recentEvents.map((event) => <li className="iam-profile-activity-row" key={event.id}>
            <span className="iam-profile-activity-action">{text(zh, eventLabels[event.action] ?? "action")}</span>
            <span className="iam-profile-activity-target">{event.target}</span>
            <time className="iam-profile-activity-time" dateTime={event.time}>{dateLabel(event.time, zh)}</time>
            <span className="iam-profile-activity-ip">{event.ip}</span>
            <span className={`iam-profile-result ${event.result}`}>{text(zh, event.result)}</span>
          </li>)}</ul> : <p className="iam-profile-activity-empty">{zh ? "暂无与此账户相关的近期活动。" : "No recent activity for this account."}</p>;
        })()}
      </section>
    </div>
  </div>;

  const renderDrawer = () => {
    if (!drawerUser && !drawerEvent) return null;
    const close = closeDrawer;
    return <div className="iam-drawer-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) close(); }}>
      <aside className="iam-drawer" ref={drawerRef} role="dialog" aria-modal="true" aria-labelledby="iam-drawer-title">
        <div className="iam-drawer-head"><div><span className="iam-eyebrow">{text(zh, drawerUser ? "accountInfo" : "eventDetails")}</span><h2 id="iam-drawer-title">{drawerUser ? text(zh, "userDetails") : text(zh, "eventDetails")}</h2></div><button className="iam-icon-button" type="button" aria-label={text(zh, "close")} onClick={close}><X size={18} /></button></div>
        {drawerUser && <>
          <div className="iam-drawer-person"><span className="iam-avatar is-large">{initials(drawerUser.displayName || drawerUser.username)}</span><div><strong>{drawerUser.displayName}</strong><span>@{drawerUser.username}</span></div><span className={`iam-status ${drawerUser.status}`}><i />{text(zh, drawerUser.status)}</span></div>
          <section className="iam-drawer-section"><h3>{text(zh, "accountFacts")}</h3><dl className="iam-facts"><div><dt>{text(zh, "role")}</dt><dd><span className={`iam-pill ${drawerUser.role === "admin" ? "is-admin" : ""}`}>{roleLabel(zh, drawerUser.role)}</span></dd></div><div><dt>{text(zh, "status")}</dt><dd>{text(zh, drawerUser.status)}</dd></div><div><dt>{text(zh, "created")}</dt><dd>{dateLabel(drawerUser.createdAt, zh)}</dd></div><div><dt>{text(zh, "lastLogin")}</dt><dd>{dateLabel(drawerUser.lastLoginAt, zh)}</dd></div></dl></section>
          <section className="iam-drawer-section"><div className="iam-session-heading"><h3>{text(zh, "sessionList")}</h3><span>{activeSessions(drawerUser.id)} {text(zh, "activeSession")}</span></div><div className="iam-session-list">{sessions.filter((session) => session.userId === drawerUser.id).length === 0 ? <p>{text(zh, "noSessions")}</p> : sessions.filter((session) => session.userId === drawerUser.id).map((session) => <div className="iam-session-row" key={session.id}><span className={`iam-session-dot ${session.revokedAt ? "is-revoked" : ""}`} /><span><strong>{session.userAgent || (zh ? "控制台会话" : "Console session")}</strong><small>{dateLabel(session.lastSeenAt ?? session.createdAt, zh)} · {session.ip ?? "—"}</small></span><em>{text(zh, session.revokedAt ? "revokedSession" : "activeSession")}</em></div>)}</div></section>
          <div className="iam-drawer-actions">{drawerUser.id === currentUserId ? <span className="iam-self-pill">{text(zh, "you")}</span> : <>
            <button type="button" onClick={(event) => openDialog("edit", drawerUser, event.currentTarget)}>{text(zh, "edit")}</button>
            <button type="button" onClick={(event) => openDialog("reset", drawerUser, event.currentTarget)}>{text(zh, "reset")}</button>
            <button type="button" onClick={(event) => openDialog(drawerUser.status === "active" ? "disable" : "enable", drawerUser, event.currentTarget)}>{text(zh, drawerUser.status === "active" ? "disable" : "enable")}</button>
            <button type="button" className="iam-danger" onClick={(event) => openDialog("delete", drawerUser, event.currentTarget)}>{text(zh, "delete")}</button>
          </>}</div>
        </>}
        {drawerEvent && <>
          <div className="iam-event-summary"><span className={`iam-pill ${drawerEvent.result === "success" ? "is-active" : "is-disabled"}`}>{text(zh, drawerEvent.result)}</span><strong>{text(zh, eventLabels[drawerEvent.action] ?? "action")}</strong><small>{dateLabel(drawerEvent.time, zh)}</small></div>
          <section className="iam-drawer-section"><h3>{text(zh, "eventDetails")}</h3><dl className="iam-facts"><div><dt>{text(zh, "actor")}</dt><dd><span className="iam-actor-cell"><span className="iam-avatar is-small">{initials(drawerEvent.actor)}</span>{drawerEvent.actor}</span></dd></div><div><dt>{text(zh, "target")}</dt><dd>{drawerEvent.target}</dd></div><div><dt>{text(zh, "action")}</dt><dd>{text(zh, eventLabels[drawerEvent.action] ?? "action")}</dd></div><div><dt>{text(zh, "time")}</dt><dd>{dateLabel(drawerEvent.time, zh)}</dd></div><div><dt>{text(zh, "ip")}</dt><dd><code>{drawerEvent.ip}</code></dd></div><div><dt>ID</dt><dd><code>{drawerEvent.id}</code></dd></div></dl></section>
          <section className="iam-drawer-section"><h3>{text(zh, "eventDrawerHint")}</h3><p>{drawerEvent.detail}</p></section>
        </>}
      </aside>
    </div>;
  };

  const renderDialog = () => {
    if (!dialog) return null;
    const editing = dialog.kind === "edit";
    const isForm = dialog.kind === "create" || editing;
    const isReset = dialog.kind === "reset";
    const title = text(zh, labels[dialog.kind]);
    const confirmMessage = dialog.kind === "delete"
      ? `${targetUser?.username ?? ""} · ${text(zh, "deletePrompt")}`
      : dialog.kind === "disable" || dialog.kind === "enable"
        ? text(zh, confirmationCopy[dialog.kind])
        : "";
    return <div className="iam-modal-backdrop" onMouseDown={(event) => { if (event.target === event.currentTarget) closeDialog(); }}>
      <div className="iam-modal" ref={dialogRef} role="dialog" aria-modal="true" aria-labelledby="iam-dialog-title">
        <div className="iam-modal-heading"><h2 id="iam-dialog-title">{title}</h2><button type="button" className="iam-icon-button" aria-label={text(zh, "close")} onClick={closeDialog}><X size={18} /></button></div>
        {isForm && <form className="iam-form" onSubmit={saveUser}>
          <label className="iam-field"><span>{text(zh, "username")}</span><input autoComplete="username" value={form.username} readOnly={editing} onChange={(event) => setForm((previous) => ({ ...previous, username: event.target.value }))} /></label>
          <label className="iam-field"><span>{text(zh, "displayName")}</span><input value={form.displayName} onChange={(event) => setForm((previous) => ({ ...previous, displayName: event.target.value }))} /></label>
          <label className="iam-field"><span>{text(zh, "role")}</span><select value={form.role} onChange={(event) => setForm((previous) => ({ ...previous, role: event.target.value as IAMRole }))}>{roleOrder.map((entry) => <option key={entry} value={entry}>{roleLabel(zh, entry)}</option>)}</select></label>
          {!editing && <label className="iam-field"><span>{text(zh, "initialPassword")}</span><input type="password" autoComplete="new-password" value={form.password} onChange={(event) => setForm((previous) => ({ ...previous, password: event.target.value }))} /></label>}
          {modalError && <p className="iam-error" role="alert">{modalError}</p>}<div className="iam-modal-actions"><button className="iam-primary" type="submit" disabled={apiLoading}>{text(zh, editing ? "save" : "createUser")}</button><button type="button" onClick={closeDialog}>{text(zh, "cancel")}</button></div>
        </form>}
        {isReset && <form className="iam-form" onSubmit={(event) => { event.preventDefault(); confirmUserAction(); }}><p>{text(zh, "resetPrompt")}</p>
          <label className="iam-field"><span>{text(zh, "newPassword")}</span><input type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} /></label>
          <label className="iam-field"><span>{text(zh, "confirmPassword")}</span><input type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} /></label>
          {modalError && <p className="iam-error" role="alert">{modalError}</p>}<div className="iam-modal-actions"><button className="iam-primary" type="submit" disabled={apiLoading}>{text(zh, "reset")}</button><button type="button" onClick={closeDialog}>{text(zh, "cancel")}</button></div>
        </form>}
        {!isForm && !isReset && <div className="iam-form"><p>{confirmMessage}</p>{modalError && <p className="iam-error" role="alert">{modalError}</p>}<div className="iam-modal-actions"><button className={dialog.kind === "delete" ? "iam-danger" : "iam-primary"} type="button" disabled={apiLoading} onClick={confirmUserAction}>{text(zh, dialog.kind)}</button><button type="button" onClick={closeDialog}>{text(zh, "cancel")}</button></div></div>}
      </div>
    </div>;
  };

  if (!currentUser) return <div className="iam-page">{!isMockMode && !sessionChecked ? <div className="iam-state" role="status">{text(zh, "checkingSession")}</div> : renderLogin()}{toast && <div className="iam-toast" role="status" aria-live="polite">{toast}</div>}</div>;

  const pageContent = tab === "users" ? renderUsers() : tab === "roles" ? renderRoles() : tab === "audit" ? renderAudit() : renderProfile();
  return <div className="iam-page">
    <div className="iam-tabs" role="tablist" aria-label={text(zh, "title")}>
      {(isAdmin ? (["users", "roles", "audit", "profile"] as Tab[]) : (["profile"] as Tab[])).map((entry) => <button key={entry} type="button" role="tab" aria-selected={tab === entry} className={tab === entry ? "is-active" : ""} onClick={() => { setTab(entry); setActionMenuOpen(false); }}>{text(zh, entry)}</button>)}
      <span className="iam-tabs-spacer" />
      <span className="iam-console-cobrand"><i aria-hidden="true" />{zh ? "Astribot 定制版" : "Astribot edition"}</span>
      <span className="iam-current-user">{currentUser.username} · {roleLabel(zh, currentUser.role)}</span>
      <button type="button" className="iam-signout" onClick={signOut}>{text(zh, "signOut")}</button>
    </div>
    {apiLoading && <div className="iam-visually-hidden" role="status">{text(zh, "loading")}</div>}
    {apiError && <p className="iam-error" role="alert">{apiError}</p>}
    {!isAdmin && tab !== "profile" ? <div className="iam-content"><div className="iam-state"><p>{text(zh, "ordinaryUserNote")}</p><button type="button" onClick={() => setTab("profile")}>{text(zh, "profile")}</button></div></div> : pageContent}
    {renderDrawer()}
    {renderDialog()}
    {toast && <div className="iam-toast" role="status" aria-live="polite">{toast}</div>}
  </div>;
}
