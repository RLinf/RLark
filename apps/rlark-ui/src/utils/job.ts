import type { JobType } from "../data";
import type { RoleResource } from "../types";

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

export function computePvcStorageMap(
  role: string,
  mounts: Array<{
    type: "host" | "storage";
    objectStorage: string;
    mountPath: string;
    hostPath: string;
  }>,
  jobName?: string,
): Record<string, string> | undefined {
  const roleSlug = role.toLowerCase().replace(/\s+/g, "-");
  const storageMounts = mounts.filter((m) => m.type === "storage");
  if (storageMounts.length === 0) return undefined;
  const map: Record<string, string> = {};
  const jobSlug = jobName ? jobName.toLowerCase().replace(/\s+/g, "-") : "";
  storageMounts.forEach((m) => {
    const volName =
      m.mountPath.replace(/\//g, "-").replace(/^-|-$/g, "") || "vol";
    const claimName = jobSlug
      ? `pvc-${jobSlug}-${roleSlug}-${volName}`
      : `pvc-${roleSlug}-${volName}`;
    map[claimName] = m.objectStorage ?? "";
  });
  return map;
}

export function generateJobCRD(opts: {
  name: string;
  type: JobType;
  headerRole: string;
  roles: string[];
  roleResources: Record<string, RoleResource>;
  runScript: string;
  domain: string;
  tensorBoardDir?: string;
  sshPublicKey?: string;
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
      const taskName = role.toLowerCase().replace(/\s+/g, "-");
      const jobSlug = opts.name.toLowerCase().replace(/\s+/g, "-");

      const hostMounts = roleMounts.filter((m) => m.type === "host");
      const storageMounts = roleMounts.filter((m) => m.type === "storage");

      const containerVolumes = hostMounts.map((m) => ({
        name: m.mountPath.replace(/\//g, "-").replace(/^-|-$/g, "") || "vol",
        hostPath: { path: m.hostPath || m.objectStorage },
      }));

      const storageVolumes = storageMounts.map((m) => {
        const volName =
          m.mountPath.replace(/\//g, "-").replace(/^-|-$/g, "") || "vol";
        const claimName = `pvc-${jobSlug}-${taskName}-${volName}`;
        return {
          name: volName,
          persistentVolumeClaim: {
            claimName,
          },
        };
      });

      const pvcStorageMap =
        res?.pvcStorageMap ?? computePvcStorageMap(role, roleMounts, opts.name);

      const allVolumeMounts = roleMounts.map((m) => ({
        name: m.mountPath.replace(/\//g, "-").replace(/^-|-$/g, "") || "vol",
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
            ...(pvcStorageMap ? { pvcStorageMap } : {}),
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
    metadata: { name: opts.name },
    spec: {
      tasks,
      ...(opts.domain ? { domain: opts.domain } : {}),
      ...(opts.sshPublicKey ? { sshPublicKey: opts.sshPublicKey } : {}),
    },
  };
}
