import { useEffect, useMemo, useState, type FormEvent } from "react";
import {
  ArrowRight,
  CircleDot,
  CloudCog,
  Languages,
  Moon,
  Settings,
  Shield,
} from "lucide-react";
import { type Copy, type Lang, type Theme, copy } from "../i18n";
import { adminNavItems } from "../constants";
import { ApiPage } from "../pages/Api";
import { DomainsPage } from "../pages/Domains";
import { ClusterManagementPage } from "../pages/ClusterManagement";
import { StorageClassesPage, StorageClassCreatePage } from "../pages/Storage";
import { CreateClusterPage } from "./CreateCluster";
import { AddonsPage } from "./Addons";
import { AdminPage } from "./AdminPage";
import { AdminDashboard } from "./AdminDashboard";
import { Header, Logo, PlatformFooter } from "../components/shared";
import { useBackendMode, usePersistentState } from "../hooks";
import { SSHKeysPage } from "../pages/SSHKeys";
import {
  ImageRegistriesPage,
  ImageRegistryCreatePage,
} from "../pages/ImageRegistries";
import { SystemConfigPage } from "../pages/SystemConfig";
import { JobsPage } from "../pages/Jobs";

export function AdminLogin({
  copy: c,
  lang,
  onLangChange,
  theme,
  onThemeChange,
  onLogin,
}: {
  copy: Copy;
  lang: Lang;
  onLangChange: (l: Lang) => void;
  theme: Theme;
  onThemeChange: (t: Theme) => void;
  onLogin: (username: string) => void;
}) {
  const zh = lang === "zh";
  const [username, setUsername] = useState("admin");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password.trim()) {
      setError(zh ? "请输入账号和密码" : "Please enter username and password");
      return;
    }
    setLoading(true);
    setError("");
    fetch("/api/v1/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: username.trim(), password }),
    })
      .then((resp) =>
        resp.ok
          ? resp.json()
          : Promise.reject(
              new Error(
                resp.status === 401
                  ? zh
                    ? "账号或密码错误"
                    : "Invalid credentials"
                  : `HTTP ${resp.status}`,
              ),
            ),
      )
      .then(() => {
        sessionStorage.setItem("rlark-admin-auth", "1");
        sessionStorage.setItem("rlark-admin-user-name", username.trim());
        onLogin(username.trim());
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  };

  return (
    <div className={"admin-login-page theme-" + theme}>
      <div className="admin-login-topbar">
        <div className="admin-brand">
          <img
            src={`/rlark-logo-${lang}-light.png`}
            alt="RLark"
            className="brand-logo brand-logo-light"
          />
          <img
            src={`/rlark-logo-${lang}-dark.png`}
            alt="RLark"
            className="brand-logo brand-logo-dark"
          />
          <div className="admin-brand-text">
            <strong>RLark</strong>
            <small>ADMIN</small>
          </div>
        </div>
        <div className="topbar-actions">
          <div className="segmented-control">
            <button
              className={lang === "zh" ? "active" : ""}
              onClick={() => onLangChange("zh")}
            >
              <Languages size={14} />中
            </button>
            <button
              className={lang === "en" ? "active" : ""}
              onClick={() => onLangChange("en")}
            >
              EN
            </button>
          </div>
          <div className="segmented-control theme-control">
            <button
              className={theme === "light" ? "active" : ""}
              onClick={() => onThemeChange("light")}
            >
              {c.common.light}
            </button>
            <button
              className={theme === "dark" ? "active" : ""}
              onClick={() => onThemeChange("dark")}
            >
              <Moon size={14} />
              {c.common.dark}
            </button>
          </div>
        </div>
      </div>
      <div className="admin-login-body">
        <form className="admin-login-card" onSubmit={handleSubmit}>
          <div className="admin-login-logo">
            <Shield size={32} />
          </div>
          <h2>{zh ? "管理后台登录" : "Admin Login"}</h2>
          <p className="muted">
            {zh ? "请输入管理员账号和密码" : "Enter your admin credentials"}
          </p>
          <div className="admin-login-field">
            <label>{zh ? "账号" : "Username"}</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="admin"
              autoComplete="username"
            />
          </div>
          <div className="admin-login-field">
            <label>{zh ? "密码" : "Password"}</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
            />
          </div>
          {error && (
            <div className="cert-error" style={{ marginBottom: 12 }}>
              {error}
            </div>
          )}
          <button
            type="submit"
            className="primary-button admin-login-btn"
            disabled={loading}
          >
            {loading
              ? zh
                ? "登录中…"
                : "Signing in…"
              : zh
                ? "登录"
                : "Sign In"}
          </button>
          <a className="admin-login-back" href="/">
            <ArrowRight size={13} />
            {zh ? "返回前台" : "Back to Console"}
          </a>
        </form>
      </div>
    </div>
  );
}

export function AdminApp() {
  const [lang, setLang] = usePersistentState<Lang>("rlark-language", "zh");
  const [theme, setTheme] = usePersistentState<Theme>("rlark-theme", "light");
  const [loggedIn, setLoggedIn] = useState(
    () =>
      import.meta.env.DEV ||
      (typeof sessionStorage !== "undefined" &&
        sessionStorage.getItem("rlark-admin-auth") === "1"),
  );
  const [userName, setUserName] = useState(
    () => sessionStorage.getItem("rlark-admin-user-name") || "admin",
  );
  const { isMockMode } = useBackendMode();
  const [collapsed, setCollapsed] = usePersistentState(
    "rlark-sidebar-collapsed",
    false,
  );
  const [storageRefreshKey, setStorageRefreshKey] = useState(0);
  const [adminPage, setAdminPage] = useState(() => {
    const p = window.location.pathname
      .replace(/^\/admin\/?/, "")
      .replace(/\/+$/, "");
    const parts = p.split("/").filter(Boolean);
    const valid = [
      "dashboard",
      "clusters-list",
      "create-cluster",
      "clusters-nodes",
      "addons",
      "jobs",
      "domains",
      "api",
      "config",
      "storageClass",
      "image-registries",
      "ssh-keys",
    ];
    if (valid.includes(parts[0])) return parts[0];
    return parts.length > 0 ? "clusters-nodes" : "dashboard";
  });
  const [adminSub, setAdminSub] = useState(() => {
    const p = window.location.pathname
      .replace(/^\/admin\/?/, "")
      .replace(/\/+$/, "");
    const parts = p.split("/").filter(Boolean);
    const explicitPages = [
      "dashboard",
      "clusters-list",
      "create-cluster",
      "clusters-nodes",
      "addons",
      "jobs",
      "domains",
      "api",
      "config",
      "storageClass",
      "image-registries",
      "ssh-keys",
    ];
    const subParts = explicitPages.includes(parts[0]) ? parts.slice(1) : parts;
    return subParts.length > 0 ? decodeURIComponent(subParts.join("/")) : "";
  });
  const c = copy[lang];
  const zh = lang === "zh";

  const adminPageTitle = useMemo(() => {
    const item = adminNavItems
      .flatMap((navItem) => [navItem, ...(navItem.children ?? [])])
      .find((navItem) => navItem.id === adminPage);
    return item ? (zh ? item.zh : item.en) : "";
  }, [adminPage, zh]);

  const navigate = (id: string, sub?: string) => {
    setAdminPage(id);
    setAdminSub(sub ?? "");
    let path = id === "dashboard" && !sub ? "/admin" : `/admin/${id}`;
    if (sub) path += "/" + encodeURIComponent(sub);
    window.history.pushState({}, "", path);
  };

  useEffect(() => {
    const onPop = () => {
      const p = window.location.pathname
        .replace(/^\/admin\/?/, "")
        .replace(/\/+$/, "");
      const parts = p.split("/").filter(Boolean);
      const valid = [
        "dashboard",
        "clusters-list",
        "create-cluster",
        "clusters-nodes",
        "addons",
        "jobs",
        "domains",
        "api",
        "config",
        "storageClass",
        "image-registries",
        "access-control",
        "ssh-keys",
      ];
      setAdminPage(
        valid.includes(parts[0])
          ? parts[0]
          : parts.length > 0
            ? "clusters-nodes"
            : "dashboard",
      );
      const subParts = valid.includes(parts[0]) ? parts.slice(1) : parts;
      setAdminSub(
        subParts.length > 0 ? decodeURIComponent(subParts.join("/")) : "",
      );
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  if (!loggedIn) {
    return (
      <AdminLogin
        copy={c}
        lang={lang}
        onLangChange={setLang}
        theme={theme}
        onThemeChange={setTheme}
        onLogin={(name) => {
          setUserName(name);
          setLoggedIn(true);
        }}
      />
    );
  }

  return (
    <div
      className={
        "app-shell theme-" + theme + (collapsed ? " sidebar-collapsed" : "")
      }
    >
      <aside className="sidebar">
        <Logo lang={lang} />
        <nav>
          <span className="nav-label">{zh ? "管理后台" : "Admin"}</span>
          {adminNavItems.map((item) => {
            const Icon = item.icon;
            const itemLabel = zh ? item.zh : item.en;
            const isParent = item.children && item.children.length > 0;
            const isActive = adminPage === item.id;
            const isChildActive = isParent
              ? item.children!.some((ch) => ch.id === adminPage)
              : false;
            const expanded = isParent && (isChildActive || isActive);
            return (
              <div key={item.id} className={isParent ? "nav-parent" : ""}>
                <button
                  aria-label={itemLabel}
                  title={itemLabel}
                  className={
                    (isParent
                      ? isChildActive
                        ? "nav-parent-expanded"
                        : isActive
                          ? "active"
                          : ""
                      : isActive
                        ? "active"
                        : "") + (isParent ? " nav-parent-btn" : "")
                  }
                  onClick={() =>
                    isParent
                      ? navigate(
                          item.id === "clusters"
                            ? "clusters-list"
                            : item.children![0].id,
                        )
                      : navigate(item.id)
                  }
                >
                  <Icon size={18} />
                  <span>{itemLabel}</span>
                </button>
                {isParent && expanded && (
                  <div className="nav-children">
                    {item.children!.map((ch) => {
                      const ChIcon = ch.icon;
                      const childLabel = zh ? ch.zh : ch.en;
                      return (
                        <button
                          key={ch.id}
                          aria-label={childLabel}
                          title={childLabel}
                          className={adminPage === ch.id ? "active" : ""}
                          onClick={() => navigate(ch.id)}
                        >
                          <ChIcon size={16} />
                          <span>{childLabel}</span>
                        </button>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </nav>
        <div className="sidebar-bottom">
          {isMockMode && (
            <div className="environment-card">
              <span>
                <CloudCog size={16} />
              </span>
              <div>
                <small>{c.common.env}</small>
                <strong>{zh ? "Mock 环境" : "Mock environment"}</strong>
                <b className="env-meta">ADMIN · Mock</b>
              </div>
              <i />
            </div>
          )}
          <button onClick={() => setCollapsed(!collapsed)}>
            <CircleDot size={17} />
            <span>{c.common.collapse}</span>
          </button>
        </div>
      </aside>
      <main className="main-area">
        <Header
          title={adminPageTitle}
          lang={lang}
          theme={theme}
          copy={c}
          onLangChange={setLang}
          onThemeChange={setTheme}
          showMockEnvironment={isMockMode}
          userName={userName}
          onCreate={() => navigate("create-cluster")}
          onLogout={() => {
            sessionStorage.removeItem("rlark-admin-auth");
            sessionStorage.removeItem("rlark-admin-user-name");
            setLoggedIn(false);
          }}
          createLabel={zh ? "创建集群" : "Create Cluster"}
        />
        {adminPage === "dashboard" && (
          <AdminDashboard copy={c} onNavigate={navigate} />
        )}
        {adminPage === "clusters-nodes" && (
          <AdminPage
            copy={c}
            selectedNode={adminSub}
            onNavigate={(sub?: string) => navigate("clusters-nodes", sub)}
          />
        )}
        {adminPage === "clusters-list" && (
          <ClusterManagementPage
            copy={c}
            selectedClusterID={adminSub}
            onSelectCluster={(id?: string) => navigate("clusters-list", id)}
            onSelectNode={(name: string) => navigate("clusters-nodes", name)}
          />
        )}
        {adminPage === "jobs" && (
          <JobsPage
            copy={c}
            isMockMode={isMockMode}
            selectedName={adminSub}
            onSelect={(name?: string) => navigate("jobs", name)}
            adminMode
          />
        )}
        {adminPage === "domains" && (
          <DomainsPage
            copy={c}
            selectedName={adminSub}
            onSelect={(name?: string) => navigate("domains", name)}
          />
        )}
        {adminPage === "api" && <ApiPage copy={c} />}
        {adminPage === "create-cluster" && (
          <CreateClusterPage copy={c} lang={lang} />
        )}
        {adminPage === "addons" && <AddonsPage copy={c} lang={lang} />}
        {adminPage === "config" && <SystemConfigPage copy={c} />}
        {adminPage === "storageClass" && adminSub === "create" && (
          <StorageClassCreatePage
            copy={c}
            onCreated={() => setStorageRefreshKey((key) => key + 1)}
            onBack={() => navigate("storageClass")}
          />
        )}
        {adminPage === "storageClass" && adminSub !== "create" && (
          <StorageClassesPage
            copy={c}
            selectedName={adminSub}
            onSelect={(name?: string) => navigate("storageClass", name)}
            onCreate={() => navigate("storageClass", "create")}
            refreshKey={storageRefreshKey}
          />
        )}
        {adminPage === "image-registries" && adminSub === "create" && (
          <ImageRegistryCreatePage
            copy={c}
            onBack={() => navigate("image-registries")}
            onCreated={() => setStorageRefreshKey((key) => key + 1)}
          />
        )}
        {adminPage === "image-registries" && adminSub !== "create" && (
          <ImageRegistriesPage
            copy={c}
            selectedName={adminSub || undefined}
            onSelect={(name?: string) =>
              navigate("image-registries", name ?? "")
            }
            onCreate={() => navigate("image-registries", "create")}
          />
        )}
        {adminPage === "ssh-keys" && <SSHKeysPage copy={c} />}
        <PlatformFooter />
      </main>
    </div>
  );
}
