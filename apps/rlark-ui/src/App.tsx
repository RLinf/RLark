import { useEffect, useMemo, useState } from "react";
import { CircleDot, CloudCog } from "lucide-react";
import type { Job } from "./data";
import { copy, type Lang, type Theme } from "./i18n";
import { navItems } from "./constants";
import type { Page } from "./types";
import { useIsAdminPath, parseRoute } from "./utils/route";
import { Logo, Header, PlatformFooter } from "./components/shared";
import { TerminalPage } from "./components/terminal";
import { Overview } from "./pages/Overview";
import { ClustersPage } from "./pages/Clusters";
import { ClusterManagementPage } from "./pages/ClusterManagement";
import { JobsPage } from "./pages/Jobs";
import { WorkflowsPage, CreateWorkflowModal } from "./pages/Workflows";
import {
  StorageClassesPage,
  StorageClassCreatePage,
  StorageClassFilesPage,
} from "./pages/Storage";
import { CreateJobModal } from "./pages/CreateJob";
import { UserLogin } from "./pages/Login";
import { SSHKeysPage } from "./pages/SSHKeys";
import { AdminApp } from "./admin/AdminApp";
import { useBackendMode, usePersistentState } from "./hooks";

type NavigateOptions = {
  replace?: boolean;
  query?: Record<string, string | undefined>;
};

export default function App() {
  const isAdmin = useIsAdminPath();
  const isTerminal = window.location.pathname === "/terminal";
  const [{ page, sub }, setRoute] = useState(parseRoute);
  const [collapsed, setCollapsed] = usePersistentState(
    "rlark-sidebar-collapsed",
    false,
  );
  const [createOpen, setCreateOpen] = useState(false);
  const [createWfOpen, setCreateWfOpen] = useState(false);
  const [storageCreateOpen, setStorageCreateOpen] = useState(() => {
    const route = parseRoute();
    return route.page === "storageClass" && route.sub === "create";
  });
  const [storageRefreshKey, setStorageRefreshKey] = useState(0);
  const [cloneJob, setCloneJob] = useState<Job | null>(null);
  const [editJob, setEditJob] = useState<Job | null>(null);
  const [lang, setLang] = usePersistentState<Lang>("rlark-language", "zh");
  const [theme, setTheme] = usePersistentState<Theme>("rlark-theme", "light");
  const [userLoggedIn, setUserLoggedIn] = useState(
    () =>
      import.meta.env.DEV ||
      (typeof sessionStorage !== "undefined" &&
        sessionStorage.getItem("rlark-user-auth") === "1"),
  );
  const [userName, setUserName] = useState(
    () => sessionStorage.getItem("rlark-user-name") || "user",
  );
  const { isMockMode } = useBackendMode();
  const c = copy[lang];
  const pageTitle = useMemo(() => c.nav[page], [c, page]);

  useEffect(() => {
    document.title = isTerminal
      ? "WebTerminal · RLark"
      : isAdmin
        ? "RLark 管理后台"
        : "RLark具身智能云原生纳管平台";
    const favicon = document.querySelector<HTMLLinkElement>("#app-favicon");
    if (favicon) {
      favicon.href = "/favicon.png";
    }
  }, [isAdmin, isTerminal]);

  const navigate = (
    next: Page,
    name?: string,
    options: NavigateOptions = {},
  ) => {
    const sub = name ?? "";
    const encodedSub = name ? encodeURIComponent(name) : "";
    setRoute({ page: next, sub });
    let path: string;
    if (next === "overview" && !sub) {
      path = "/";
    } else if (next === "clusters-management") {
      path = encodedSub ? `/clusters/${encodedSub}` : "/clusters";
    } else if (next === "clusters-nodes") {
      path = encodedSub ? `/nodes/${encodedSub}` : "/nodes";
    } else {
      path = `/${next}${encodedSub ? "/" + encodedSub : ""}`;
    }
    const params = new URLSearchParams();
    Object.entries(options.query ?? {}).forEach(([key, value]) => {
      if (value) params.set(key, value);
    });
    if (params.size) path += `?${params.toString()}`;
    window.history[options.replace ? "replaceState" : "pushState"](
      {},
      "",
      path,
    );
  };

  useEffect(() => {
    const onPop = () => setRoute(parseRoute());
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);

  if (isAdmin) return <AdminApp />;
  if (!userLoggedIn) {
    return (
      <UserLogin
        onLogin={(name) => {
          setUserName(name);
          setUserLoggedIn(true);
        }}
      />
    );
  }

  if (isTerminal) {
    const params = new URLSearchParams(window.location.search);
    const workerCRName = params.get("worker") ?? "";
    const jobName = params.get("job") ?? "";
    const workerStatus = params.get("status") ?? "";
    if (!workerCRName) {
      return (
        <div className="terminal-page-error">
          缺少 Worker 参数，无法连接终端。
        </div>
      );
    }
    return (
      <TerminalPage
        workerCRName={workerCRName}
        workerName={workerCRName}
        jobName={jobName}
        workerStatus={workerStatus}
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
          <span className="nav-label">{c.nav.workspace}</span>
          {navItems.map((item) => {
            const Icon = item.icon;
            const isParent = item.children && item.children.length > 0;
            const itemLabel = isParent ? c.nav.clustersParent : c.nav[item.id];
            const isActive = page === item.id;
            const isChildActive = isParent
              ? item.children!.some((ch) => ch.id === page)
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
                      ? navigate(item.children![0].id)
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
                      const childLabel = c.nav[ch.id];
                      return (
                        <button
                          key={ch.id}
                          aria-label={childLabel}
                          title={childLabel}
                          className={page === ch.id ? "active" : ""}
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
                <strong>
                  {lang === "zh" ? "Mock 环境" : "Mock environment"}
                </strong>
                <b className="env-meta">{c.common.envMeta}</b>
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
          title={pageTitle}
          lang={lang}
          theme={theme}
          copy={c}
          onLangChange={setLang}
          onThemeChange={setTheme}
          showMockEnvironment={isMockMode}
          userName={userName}
          onCreate={() => setCreateOpen(true)}
          onLogout={() => {
            sessionStorage.removeItem("rlark-user-auth");
            sessionStorage.removeItem("rlark-user-name");
            setUserLoggedIn(false);
          }}
        />
        {page === "overview" && (
          <Overview navigate={navigate} copy={c} isMockMode={isMockMode} />
        )}{" "}
        {page === "clusters-management" && (
          <ClusterManagementPage
            copy={c}
            workerOnly
            selectedClusterID={sub}
            onSelectCluster={(id?: string) =>
              navigate("clusters-management", id, { replace: !id })
            }
            onSelectNode={(name: string) => navigate("clusters-nodes", name)}
          />
        )}{" "}
        {page === "clusters-nodes" && (
          <ClustersPage
            copy={c}
            initialView="nodes"
            selectedNodeName={sub}
            onNavigate={(name?: string) =>
              navigate("clusters-nodes", name, { replace: !name })
            }
            onTaskNavigate={(name: string) => navigate("jobs", name)}
          />
        )}{" "}
        {page === "jobs" && (
          <JobsPage
            copy={c}
            isMockMode={isMockMode}
            selectedName={sub}
            onSelect={(name?: string) =>
              navigate("jobs", name, { replace: !name })
            }
            onCreate={() => {
              setCloneJob(null);
              setEditJob(null);
              setCreateOpen(true);
            }}
            onClone={(job) => {
              setCloneJob(job);
              setEditJob(null);
              setCreateOpen(true);
            }}
            onEdit={(job) => {
              setEditJob(job);
              setCloneJob(null);
              setCreateOpen(true);
            }}
          />
        )}{" "}
        {page === "storageClass" && (
          <StorageClassesPage
            copy={c}
            selectedName={sub === "create" ? undefined : sub}
            onSelect={(name?: string) =>
              navigate("storageClass", name, { replace: !name })
            }
            onCreate={() => setStorageCreateOpen(true)}
            refreshKey={storageRefreshKey}
          />
        )}
        {page === "files" && (
          <StorageClassFilesPage
            copy={c}
            sub={sub}
            onBack={() =>
              navigate("storageClass", undefined, { replace: true })
            }
          />
        )}
        {page === "workflows" && (
          <WorkflowsPage
            copy={c}
            selectedName={sub}
            onSelect={(name?: string) =>
              navigate("workflows", name, { replace: !name })
            }
            onCreate={() => setCreateWfOpen(true)}
            onJobClick={(name) => navigate("jobs", name)}
          />
        )}
        {page === "ssh-keys" && <SSHKeysPage copy={c} />}
        <PlatformFooter />
      </main>
      {createOpen && (
        <CreateJobModal
          onClose={() => {
            setCreateOpen(false);
            setCloneJob(null);
            setEditJob(null);
          }}
          copy={c}
          cloneJob={cloneJob}
          editJob={editJob}
        />
      )}
      {createWfOpen && (
        <CreateWorkflowModal onClose={() => setCreateWfOpen(false)} copy={c} />
      )}
      {storageCreateOpen && (
        <StorageClassCreatePage
          copy={c}
          onCreated={() => setStorageRefreshKey((key) => key + 1)}
          onBack={() => {
            setStorageCreateOpen(false);
            if (sub === "create") navigate("storageClass");
          }}
        />
      )}
    </div>
  );
}
