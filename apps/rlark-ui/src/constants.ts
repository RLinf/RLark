import {
  GitBranch,
  Braces,
  Globe2,
  HardDrive,
  Image,
  Network,
  Package,
  Server,
  Settings,
  CirclePlus,
  ListChecks,
  LayoutDashboard,
  Terminal,
} from "lucide-react";
import type { NavParent, AdminNavItem } from "./types";

export const navItems: NavParent[] = [
  { id: "overview", icon: LayoutDashboard },
  { id: "clusters-management", icon: Network },
  { id: "clusters-nodes", icon: Server },
  { id: "jobs", icon: ListChecks },
  { id: "workflows", icon: GitBranch },
  { id: "storageClass", icon: HardDrive },
  { id: "ssh-keys", icon: Terminal },
];

export const adminNavItems: AdminNavItem[] = [
  {
    id: "dashboard",
    icon: LayoutDashboard,
    zh: "管理工作台",
    en: "Dashboard",
  },
  {
    id: "clusters",
    icon: Network,
    zh: "集群管理",
    en: "Clusters",
    children: [
      {
        id: "create-cluster",
        icon: CirclePlus,
        zh: "创建集群",
        en: "Create Cluster",
      },
      {
        id: "clusters-list",
        icon: Network,
        zh: "集群列表",
        en: "Cluster List",
      },
      { id: "clusters-nodes", icon: Server, zh: "节点管理", en: "Nodes" },
      { id: "addons", icon: Package, zh: "组件市场", en: "Addons" },
    ],
  },
  { id: "jobs", icon: ListChecks, zh: "任务管理", en: "Jobs" },
  { id: "domains", icon: Globe2, zh: "网络域", en: "Domains" },
  { id: "api", icon: Braces, zh: "接口参考", en: "API Reference" },
  { id: "config", icon: Settings, zh: "系统配置", en: "Config" },
  { id: "storageClass", icon: HardDrive, zh: "存储管理", en: "Storage" },
  { id: "ssh-keys", icon: Terminal, zh: "SSH 公钥", en: "SSH Keys" },
  {
    id: "image-registries",
    icon: Image,
    zh: "镜像管理",
    en: "Image Registries",
  },
];
