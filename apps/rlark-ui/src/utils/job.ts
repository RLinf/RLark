import type { JobTag, JobType } from "../data";
import type { RoleResource } from "../types";

// 任务名称限制：1-64 个字符，支持中英文、数字、中划线（-）、下划线（_）和英文句号（.）
export const JOB_DISPLAY_NAME_MAX_LENGTH = 64;
export const JOB_DISPLAY_NAME_PATTERN = /^[A-Za-z0-9\u4e00-\u9fa5._-]{1,64}$/;

export function isValidJobDisplayName(name: string): boolean {
  return JOB_DISPLAY_NAME_PATTERN.test(name);
}

// 角色名称与任务名称使用相同的字符与长度限制
export const ROLE_NAME_MAX_LENGTH = JOB_DISPLAY_NAME_MAX_LENGTH;
export const ROLE_NAME_PATTERN = JOB_DISPLAY_NAME_PATTERN;

export function isValidRoleName(name: string): boolean {
  return ROLE_NAME_PATTERN.test(name);
}

const TASK_ROLE_MAP: Record<string, "Actor" | "Rollout" | "Env"> = {
  Actor: "Actor",
  Learner: "Actor",
  Evaluator: "Actor",
  "Scenario Runner": "Actor",
  "Metrics Aggregator": "Actor",
  Collector: "Actor",
  "Calibration Driver": "Actor",
  Rollout: "Rollout",
  "Rollout Worker": "Rollout",
  "Robot Operator": "Rollout",
  "Camera Worker": "Rollout",
  Environment: "Env",
  "Env Worker": "Env",
  Uploader: "Env",
  "Quality Checker": "Env",
  "Robot Worker": "Env",
  "Metrics Worker": "Env",
};

const ROLE_TEMPLATES: Record<JobType, string[]> = {
  RL: ["Actor", "Rollout", "Environment"],
  DataCollection: ["Environment"],
  Evaluation: ["Rollout", "Environment"],
  Custom: [],
};

export { TASK_ROLE_MAP, ROLE_TEMPLATES };

export function mapTaskRole(role: string): "Actor" | "Rollout" | "Env" {
  if (TASK_ROLE_MAP[role]) return TASK_ROLE_MAP[role];
  const lower = role.toLowerCase();
  if (
    lower.includes("env") ||
    lower.includes("uploader") ||
    lower.includes("quality") ||
    lower.includes("metrics worker")
  )
    return "Env";
  if (
    lower.includes("rollout") ||
    lower.includes("camera") ||
    lower.includes("robot operator")
  )
    return "Rollout";
  if (lower.includes("robot")) return "Env";
  if (lower.includes("collect") || lower.includes("eval")) return "Actor";
  return "Actor";
}

export function parseNodeSelector(s: string): Record<string, string> {
  const result: Record<string, string> = {};
  let lastKey = "";
  for (const part of s.split(",")) {
    const eqIdx = part.indexOf("=");
    if (eqIdx >= 0) {
      const k = part.slice(0, eqIdx).trim();
      const v = part.slice(eqIdx + 1).trim();
      if (k) {
        result[k] = v;
        lastKey = k;
      }
    } else if (lastKey) {
      result[lastKey] += "," + part.trim();
    }
  }
  return result;
}

export function parseNodeSelectorStr(s: string): Record<string, string> {
  const result: Record<string, string> = {};
  let lastKey = "";
  for (const part of s.split(",")) {
    const eqIdx = part.indexOf("=");
    if (eqIdx >= 0) {
      const k = part.slice(0, eqIdx).trim();
      const v = part.slice(eqIdx + 1).trim();
      if (k) {
        result[k] = v;
        lastKey = k;
      }
    } else if (lastKey) {
      result[lastKey] += "," + part.trim();
    }
  }
  return result;
}

export function selectorToStr(sel: Record<string, string>): string {
  return Object.entries(sel)
    .map(([k, v]) => `${k}=${v}`)
    .join(",");
}

export function generateJobResourceName(): string {
  const bytes = new Uint8Array(8);
  crypto.getRandomValues(bytes);
  return `jo-${Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("")}`;
}

export function toResourceName(value: string): string {
  return Array.from(value.toLowerCase())
    .map((char) =>
      /^[a-z0-9.-]$/.test(char)
        ? char
        : `-${char.codePointAt(0)?.toString(16)}-`,
    )
    .join("")
    .replace(/-+/g, "-")
    .replace(/^[^a-z0-9]+|[^a-z0-9]+$/g, "");
}

// shortHash 返回输入字符串的短哈希（FNV-1a，6 位十六进制），用于让名字唯一。
function shortHash(s: string): string {
  let h = 0x811c9dc5;
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193);
  }
  return (h >>> 0).toString(16).padStart(8, "0").slice(0, 6);
}

// toVolumeName 把 mount path 转成合法的 k8s volume 名（DNS-1123 label：
// 小写字母/数字/`-`，最长 63）。非法字符替换为 `-`；如果发生过替换
// （说明不同 path 可能清洗成同一个名字），追加原 path 的短哈希保证唯一。
export function toVolumeName(mountPath: string): string {
  const hadIllegal = /[^a-z0-9/-]/.test(mountPath);
  let name =
    mountPath
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-+|-+$/g, "") || "vol";
  if (hadIllegal) {
    name = `${name}-${shortHash(mountPath)}`;
  }
  if (name.length > 63) {
    const suffix = shortHash(mountPath);
    name = `${name.slice(0, 56).replace(/-+$/g, "")}-${suffix}`;
  }
  return name;
}

export function automaticNetworkDomain(domains: Array<{ name: string }>) {
  return (
    [...domains].sort((left, right) => left.name.localeCompare(right.name))[0]
      ?.name ?? ""
  );
}

export function generateJobCRD(opts: {
  name: string;
  displayName?: string;
  type: JobType;
  headerRole: string;
  roles: string[];
  roleResources: Record<string, RoleResource>;
  runScript: string;
  domain: string;
  tensorBoardDir?: string;
  sshPublicKey?: string;
  tags?: JobTag[];
}) {
  const tasks = opts.roles
    .map((role) => {
      const res = opts.roleResources[role];
      if (!res?.image) return null;
      const isHead = role === opts.headerRole;
      const roleEnvs = res?.envs ?? [];
      const roleMounts = res?.mounts ?? [];
      const envVars = [
        ...roleEnvs
          .filter((e) => e.key !== "RLARK_TASK_ROLE")
          .map((e) => ({ name: e.key, value: e.value })),
        { name: "RLARK_TASK_ROLE", value: role },
      ];
      const taskName = toResourceName(role);
      const hostMounts = roleMounts.filter((m) => m.type === "host");
      const storageMounts = roleMounts.filter((m) => m.type === "storage");

      const containerVolumes = hostMounts.map((m) => ({
        name: toVolumeName(m.mountPath),
        hostPath: { path: m.hostPath || m.objectStorage },
      }));

      const storageVolumes = storageMounts.map((m) => {
        const volName = toVolumeName(m.mountPath);
        const pvcSizeGb =
          m.pvcSizeGb === "" ? 10 : Math.min(200, Math.max(1, m.pvcSizeGb));
        return {
          name: volName,
          ephemeral: {
            volumeClaimTemplate: {
              spec: {
                accessModes: ["ReadWriteOnce"],
                ...(m.objectStorage
                  ? { storageClassName: m.objectStorage }
                  : {}),
                resources: {
                  requests: { storage: `${pvcSizeGb}Gi` },
                },
              },
            },
          },
        };
      });

      const allVolumeMounts = roleMounts.map((m) => ({
        name: toVolumeName(m.mountPath),
        mountPath: m.mountPath,
      }));

      return {
        name: taskName,
        head: isHead,
        agentType: "Kubernetes",
        role: mapTaskRole(role),
        nodeSelector: res ? parseNodeSelector(res.nodeSelector) : {},
        prepareScript: res?.prepareScript ?? "",
        ...(isHead ? { runScript: opts.runScript } : {}),
        ...(isHead && opts.tensorBoardDir
          ? { tensorBoardDir: opts.tensorBoardDir }
          : {}),
        kubernetes: {
          workload: {
            kind: "StatefulSet",
            replicas: res ? Number(res.replicas) : 1,
            template: {
              spec: {
                containers: [
                  {
                    name: "main",
                    image: res?.image ?? "",
                    env: envVars.length > 0 ? envVars : undefined,
                    volumeMounts:
                      allVolumeMounts.length > 0 ? allVolumeMounts : undefined,
                    resources: res
                      ? {
                          requests: {
                            ...(res.cpu ? { cpu: res.cpu } : {}),
                            ...(res.memory ? { memory: res.memory } : {}),
                            ...(res.gpu && res.gpu !== "0"
                              ? { "nvidia.com/gpu": res.gpu }
                              : {}),
                            ...Object.fromEntries(
                              (res.devices ?? [])
                                .filter(
                                  (d) =>
                                    d.name && d.quantity && d.quantity !== "0",
                                )
                                .map((d) => [d.name, d.quantity]),
                            ),
                          },
                          limits: {
                            ...(res.gpu && res.gpu !== "0"
                              ? { "nvidia.com/gpu": res.gpu }
                              : {}),
                            ...Object.fromEntries(
                              (res.devices ?? [])
                                .filter(
                                  (d) =>
                                    d.name && d.quantity && d.quantity !== "0",
                                )
                                .map((d) => [d.name, d.quantity]),
                            ),
                          },
                        }
                      : undefined,
                  },
                ],
                volumes: [
                  ...(containerVolumes.length > 0 ? containerVolumes : []),
                  ...(storageVolumes.length > 0 ? storageVolumes : []),
                ],
              },
            },
          },
        },
      };
    })
    .filter(Boolean);

  return {
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Job",
    metadata: {
      name: opts.name,
      ...(opts.displayName
        ? { annotations: { "rlark.io/display-name": opts.displayName } }
        : {}),
    },
    spec: {
      tasks,
      ...(opts.domain ? { domain: opts.domain } : {}),
      ...(opts.sshPublicKey ? { sshPublicKey: opts.sshPublicKey } : {}),
      ...(opts.tags && opts.tags.length > 0
        ? {
            tags: [
              ...new Map(
                opts.tags
                  .filter((t) => t.key.trim() && t.value.trim())
                  .map((t) => [
                    t.key.trim(),
                    {
                      key: t.key.trim(),
                      values: opts
                        .tags!.filter(
                          (item) => item.key.trim() === t.key.trim(),
                        )
                        .map((item) => item.value.trim())
                        .filter(
                          (value, index, values) =>
                            value && values.indexOf(value) === index,
                        ),
                    },
                  ]),
              ).values(),
            ],
          }
        : {}),
    },
  };
}
