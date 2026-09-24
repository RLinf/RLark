import { useEffect, useState } from "react";
import type { Page } from "../types";

export type AdminRoute = { page: string; sub: string };

const adminPages = new Set([
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
  "files",
  "image-registries",
  "ssh-keys",
]);

export function isAdminPath(pathname: string) {
  return pathname === "/admin" || pathname.startsWith("/admin/");
}

export function parseAdminRoute(
  pathname = window.location.pathname,
): AdminRoute {
  const parts = pathname
    .replace(/^\/admin\/?/, "")
    .replace(/\/+$/, "")
    .split("/")
    .filter(Boolean);
  const page = adminPages.has(parts[0])
    ? parts[0]
    : parts.length > 0
      ? "clusters-nodes"
      : "dashboard";
  const subParts = adminPages.has(parts[0]) ? parts.slice(1) : parts;
  return {
    page,
    sub: subParts.length > 0 ? decodeURIComponent(subParts.join("/")) : "",
  };
}

export function filesPath(
  cluster: string,
  storageClass: string,
  admin = false,
) {
  const prefix = admin ? "/admin/files" : "/files";
  return `${prefix}/${encodeURIComponent(cluster)}/${encodeURIComponent(storageClass)}`;
}

export function hasTerminalSession(storage: Pick<Storage, "getItem">) {
  return Boolean(storage.getItem("rlark-auth-token"));
}

export function useIsAdminPath() {
  const [isAdmin, setIsAdmin] = useState(() => {
    if (typeof window === "undefined") return false;
    return isAdminPath(window.location.pathname);
  });
  useEffect(() => {
    const onPop = () => {
      setIsAdmin(isAdminPath(window.location.pathname));
    };
    window.addEventListener("popstate", onPop);
    return () => window.removeEventListener("popstate", onPop);
  }, []);
  return isAdmin;
}

export function parseRoute() {
  const parts = window.location.pathname
    .replace(/^\/+/, "")
    .replace(/\/+$/, "")
    .split("/")
    .filter(Boolean);
  const valid: Page[] = [
    "overview",
    "clusters-management",
    "clusters-nodes",
    "jobs",
    "workflows",
    "domains",
    "storageClass",
    "files",
    "ssh-keys",
  ];
  const top = (parts[0] as Page) ?? "overview";
  if ((top as string) === "nodes") {
    const nodeName = parts.slice(1).join("/");
    return {
      page: "clusters-nodes" as Page,
      sub: decodeURIComponent(nodeName),
    };
  }
  if ((top as string) === "clusters") {
    const sub = parts[1] ?? "overview";
    if (sub === "nodes") {
      const nodeName = parts.slice(2).join("/");
      return {
        page: "clusters-nodes" as Page,
        sub: decodeURIComponent(nodeName),
      };
    }
    if (sub === "manage") {
      const clusterID = parts.slice(2).join("/");
      return {
        page: "clusters-management" as Page,
        sub: decodeURIComponent(clusterID),
      };
    }
    const clusterID = parts.slice(1).join("/");
    return {
      page: "clusters-management" as Page,
      sub: decodeURIComponent(clusterID),
    };
  }
  if (!valid.includes(top)) return { page: "overview" as Page, sub: "" };
  const sub = parts.slice(1).join("/");
  return { page: top, sub: decodeURIComponent(sub) };
}
