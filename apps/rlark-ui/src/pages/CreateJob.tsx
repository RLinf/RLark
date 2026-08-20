import { useEffect, useRef, useState } from "react";
import { Check, ChevronDown, Plus, Trash2, X } from "lucide-react";
import { type Cluster, clusters, type Job, type JobType } from "../data";
import type { Copy } from "../i18n";
import type { RoleResource } from "../types";
import {
  ROLE_TEMPLATES,
  computePvcStorageMap,
  generateJobCRD,
  parseNodeSelectorStr,
} from "../utils/job";
import { toYaml } from "../utils/yaml";
import { useNodeLabels } from "../utils/nodes";
import { imageReferenceHasWhitespace } from "../utils/imageReference";
import { RoleNameInput } from "../components/create";
import { CodeEditorField } from "../components/CodeEditor";
import { ResourcePlacementPicker } from "../components/ResourcePlacementPicker";

function ClusterSelect({
  clusters,
  value,
  zh,
  onChange,
}: {
  clusters: Cluster[];
  value: string;
  zh: boolean;
  onChange: (value: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selected = clusters.find(
    (cluster) => cluster.id === value || cluster.name === value,
  );

  useEffect(() => {
    const closeOnOutsideClick = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", closeOnOutsideClick);
    return () => document.removeEventListener("mousedown", closeOnOutsideClick);
  }, []);

  const typeLabel = (cluster: Cluster) =>
    cluster.type === "Embodied"
      ? zh
        ? "具身设备集群"
        : "Embodied"
      : zh
        ? "云算力集群"
        : "Cloud compute";
  const statusLabel = (cluster: Cluster) =>
    cluster.phase === "Online"
      ? zh
        ? "运行正常"
        : "Online"
      : cluster.phase === "Offline"
        ? zh
          ? "离线"
          : "Offline"
        : zh
          ? "部分异常"
          : "Degraded";

  const optionContent = (cluster: Cluster) => (
    <>
      <strong>{cluster.name}</strong>
      <span className="worker-cluster-type">{typeLabel(cluster)}</span>
      <span className={`worker-cluster-state ${cluster.phase.toLowerCase()}`}>
        <i aria-hidden="true" />
        {statusLabel(cluster)}
      </span>
    </>
  );

  return (
    <div
      className="worker-cluster-select"
      ref={rootRef}
      onKeyDown={(event) => {
        if (event.key === "Escape" && open) {
          event.stopPropagation();
          setOpen(false);
        }
      }}
    >
      <button
        type="button"
        className="worker-cluster-trigger"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen((current) => !current)}
      >
        {selected ? (
          optionContent(selected)
        ) : (
          <span className="worker-cluster-placeholder">
            {zh ? "请选择集群" : "Select a cluster"}
          </span>
        )}
        <ChevronDown size={15} className={open ? "open" : undefined} />
      </button>
      {open && (
        <div className="worker-cluster-options" role="listbox">
          {clusters.map((cluster) => {
            const active = cluster.id === value || cluster.name === value;
            return (
              <button
                type="button"
                role="option"
                aria-selected={active}
                className={active ? "active" : undefined}
                key={cluster.id}
                onClick={() => {
                  onChange(cluster.id);
                  setOpen(false);
                }}
              >
                {optionContent(cluster)}
                {active && <Check size={15} className="worker-cluster-check" />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

export function CreateJobModal({
  onClose,
  copy: c,
  cloneJob,
  editJob,
  restartAfterSave = false,
}: {
  onClose: () => void;
  copy: Copy;
  cloneJob?: Job | null;
  editJob?: Job | null;
  restartAfterSave?: boolean;
}) {
  const zh = c.nav.overview === "总览";
  const isEdit = !!editJob;
  const sourceJob = editJob ?? cloneJob;
  const [type, setType] = useState<JobType>(sourceJob?.type ?? "RL");
  const [step, setStep] = useState(1);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [errorNonce, setErrorNonce] = useState(0);
  const errorBannerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (error && errorBannerRef.current) {
      errorBannerRef.current.scrollIntoView({
        behavior: "smooth",
        block: "center",
      });
    }
  }, [error, errorNonce]);

  useEffect(() => {
    const handleEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !submitting) onClose();
    };
    document.addEventListener("keydown", handleEscape);
    return () => document.removeEventListener("keydown", handleEscape);
  }, [onClose, submitting]);

  const [roles, setRoles] = useState<string[]>(
    sourceJob?.defaultRoles ?? ROLE_TEMPLATES[type],
  );
  const [jobName, setJobName] = useState(
    sourceJob
      ? editJob
        ? sourceJob.name
        : sourceJob.name + "-copy"
      : "robot-policy-training",
  );
  const [headerRole, setHeaderRole] = useState(
    sourceJob?.headerRole ?? roles[0],
  );
  const effectiveHeader = roles.includes(headerRole) ? headerRole : roles[0];

  const [runScript, setRunScript] = useState(
    sourceJob?.command ??
      "python train.py --config /mnt/config/train.yaml --dataset /mnt/dataset --output /mnt/checkpoints",
  );
  const [domain, setDomain] = useState(sourceJob?.domain ?? "");
  const [tensorBoardDir, setTensorBoardDir] = useState(
    sourceJob?.tensorBoardDir ?? "",
  );
  const [sshPublicKey, setSSHPublicKey] = useState(
    sourceJob?.sshPublicKey ?? "",
  );
  const [sshKeys, setSShKeys] = useState<
    { index: number; user: string; public_key: string; added_at: string }[]
  >([]);
  const [sshKeysLoaded, setSShKeysLoaded] = useState(false);
  const [domains, setDomains] = useState<{ name: string; cidr: string }[]>([]);
  const {
    clusterDisplayNames,
    nodes: allNodes,
    loading: nodesLoading,
  } = useNodeLabels();
  const [availableClusters, setAvailableClusters] = useState<Cluster[]>(
    clusters.slice(0, 4),
  );
  const [storageClasses, setStorageClasses] = useState<
    { name: string; description: string; bucket: string }[]
  >([]);
  const [storageClassLoading, setStorageClassLoading] = useState(false);
  const [storageClassFetched, setStorageClassFetched] = useState(false);
  const [clustersLoaded, setClustersLoaded] = useState(false);
  const lastFetchedStorageClusterRef = useRef<string>("");
  const inferenceDoneRef = useRef(false);

  useEffect(() => {
    fetch("/api/v1/rlinf.io/v1alpha1/domains")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) =>
        setDomains(
          (data.items ?? []).map((d: any) => ({
            name: d.metadata?.name ?? "",
            cidr: d.spec?.cidr ?? "",
          })),
        ),
      )
      .catch(() => {});
  }, []);

  useEffect(() => {
    fetch("/api/v1/ssh-user-keys")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) => {
        setSShKeys(data ?? []);
        setSShKeysLoaded(true);
      })
      .catch(() => setSShKeysLoaded(true));
  }, []);

  useEffect(() => {
    fetch("/api/v1/clusters")
      .then((r) =>
        r.ok ? r.json() : Promise.reject(new Error(`HTTP ${r.status}`)),
      )
      .then((data) => {
        const list: Cluster[] = (data.data ?? []).map((c: any) => ({
          id: c.id ?? c.name ?? "",
          name: c.name ?? c.id ?? "",
          type: c.type === "Embodied" ? "Embodied" : "Cloud",
          region: c.region ?? "",
          location: c.location ?? "",
          phase: c.phase ?? "Online",
          cloudNodes: c.cloudNodes ?? 0,
          embodiedNodes: c.embodiedNodes ?? 0,
          robots: c.robots ?? 0,
          gpuModels: c.gpuModels ?? [],
          robotModels: c.robotModels ?? [],
          cpuUsage: c.cpuUsage ?? 0,
          gpuUsage: c.gpuUsage ?? 0,
          robotUsage: c.robotUsage ?? 0,
          runningJobs: c.runningJobs ?? 0,
          description: c.description ?? "",
        }));
        const realList = list.length > 0 ? list : clusters.slice(0, 4);
        setAvailableClusters(realList);
        setClustersLoaded(list.length > 0);
        if (list.length > 0) {
          setRoleResources((prev) =>
            Object.fromEntries(
              Object.entries(prev).map(([role, rr]) => {
                const current = rr.cluster;
                const match = list.find(
                  (c) => c.id === current || c.name === current,
                );
                return [
                  role,
                  { ...rr, cluster: match ? match.id : list[0].id },
                ];
              }),
            ),
          );
        }
      })
      .catch(() => {});
  }, []);

  const fetchStorageClasses = async (cluster?: string) => {
    if (storageClassLoading) return;
    const clusterKey = cluster ?? "";
    if (
      storageClassFetched &&
      lastFetchedStorageClusterRef.current === clusterKey
    )
      return;
    lastFetchedStorageClusterRef.current = clusterKey;
    setStorageClassLoading(true);
    setStorageClassFetched(false);
    try {
      const url = new URL(
        "/api/v1/storage/storageclass",
        window.location.origin,
      );
      if (cluster) {
        url.searchParams.set("clusters", cluster);
      }
      const resp = await fetch(url.pathname + url.search);
      if (!resp.ok) throw new Error(`HTTP ${resp.status}`);
      const data = await resp.json();
      const scData = data.data ?? {};
      const storageClassList = Object.values(scData).map((sc: any) => ({
        name: sc.name,
        description: sc.description || "",
        bucket: sc.bucket || "",
      }));
      setStorageClasses(storageClassList);
    } catch (e) {
      console.warn("Failed to fetch storage classes:", e);
    } finally {
      setStorageClassLoading(false);
      setStorageClassFetched(true);
    }
  };

  const cloneRR: Record<string, RoleResource> = {};
  if (sourceJob) {
    sourceJob.resources.forEach((res) => {
      cloneRR[res.role] = {
        role: res.role,
        cluster: res.cluster,
        nodeSelector: res.nodeSelector,
        replicas: res.replicas,
        cpu: "",
        memory: "",
        gpu: res.gpu,
        devices: res.devices?.map((d) => ({ ...d })) ?? [],
        image: res.image,
        prepareScript: res.prepareScript ?? "",
        envs: res.env.map((e) => ({ ...e })),
        mounts: res.mounts.map((m) => ({
          type: m.type ?? "host",
          objectStorage: m.objectStorage ?? "",
          mountPath: m.mountPath ?? "",
          hostPath: m.hostPath ?? m.objectStorage ?? "",
        })),
      };
    });
  }
  const defaultRoleResources: Record<string, RoleResource> = {};
  if (!sourceJob) {
    roles.forEach((role, index) => {
      defaultRoleResources[role] = {
        role,
        cluster: clusterDisplayNames[0] ?? "",
        nodeSelector: "",
        replicas: 0,
        cpu: "",
        memory: "",
        gpu: index === 0 ? "4" : "0",
        devices: [],
        image: "",
        prepareScript: "",
        envs: [],
        mounts: [],
      };
    });
  }
  const [roleResources, setRoleResources] = useState<
    Record<string, RoleResource>
  >(sourceJob ? cloneRR : defaultRoleResources);
  const [activeRoleTab, setActiveRoleTab] = useState<string>(roles[0] ?? "");

  useEffect(() => {
    if (availableClusters.length === 0) return;
    setRoleResources((prev) => {
      let changed = false;
      const next = Object.fromEntries(
        Object.entries(prev).map(([role, resource]) => {
          const matchedCluster = availableClusters.find(
            (cluster) =>
              cluster.id === resource.cluster ||
              cluster.name === resource.cluster,
          );
          const nextCluster = matchedCluster?.id ?? availableClusters[0].id;
          if (nextCluster !== resource.cluster) changed = true;
          return [role, { ...resource, cluster: nextCluster }];
        }),
      );
      return changed ? next : prev;
    });
  }, [availableClusters]);

  useEffect(() => {
    if (clusterDisplayNames.length === 0) return;
    setRoleResources((prev) => {
      let changed = false;
      const next: Record<string, RoleResource> = {};
      for (const [k, v] of Object.entries(prev)) {
        if (!v.cluster && clusterDisplayNames[0]) {
          next[k] = { ...v, cluster: clusterDisplayNames[0] };
          changed = true;
        } else {
          next[k] = v;
        }
      }
      return changed ? next : prev;
    });
  }, [clusterDisplayNames]);

  useEffect(() => {
    if (inferenceDoneRef.current || allNodes.length === 0 || !clustersLoaded)
      return;
    inferenceDoneRef.current = true;

    setRoleResources((prev) => {
      let changed = false;
      const next = Object.fromEntries(
        Object.entries(prev).map(([role, resource]) => {
          const selectorMap = parseNodeSelectorStr(resource.nodeSelector);
          if (Object.keys(selectorMap).length === 0) return [role, resource];

          const matched = allNodes.filter((n) => {
            const labels = n.metadata.labels ?? {};
            return Object.entries(selectorMap).every(([k, v]) => {
              const values = v.split(",");
              return labels[k] !== undefined && values.includes(labels[k]);
            });
          });

          const nsList = Array.from(
            new Set(
              matched
                .map((n) => n.metadata.namespace)
                .filter((v): v is string => !!v),
            ),
          );
          if (nsList.length === 0) return [role, resource];

          const inferred = nsList[0];
          if (inferred !== resource.cluster) {
            changed = true;
            return [role, { ...resource, cluster: inferred }];
          }
          return [role, resource];
        }),
      );
      return changed ? next : prev;
    });
  }, [allNodes, clustersLoaded]);

  useEffect(() => {
    if (!activeRoleTab) return;
    const rr = roleResources[activeRoleTab];
    if (!rr) return;
    const hasStorageMount = rr.mounts.some((m) => m.type === "storage");
    if (hasStorageMount && rr.cluster) {
      fetchStorageClasses(rr.cluster);
    }
  }, [activeRoleTab, roleResources]);

  const onTypeChange = (next: JobType) => {
    setType(next);
    const newRoles = ROLE_TEMPLATES[next];
    setRoles(newRoles);
    setHeaderRole(newRoles[0] ?? "");
    setActiveRoleTab(newRoles[0] ?? "");
    const newRR: Record<string, RoleResource> = {};
    newRoles.forEach((role, index) => {
      newRR[role] = roleResources[role] ?? {
        role,
        cluster: clusterDisplayNames[0] ?? "",
        nodeSelector: "",
        replicas: 0,
        cpu: "",
        memory: "",
        gpu: index === 0 ? "4" : "0",
        devices: [],
        image: "",
        prepareScript: "",
        envs: [],
        mounts: [],
      };
    });
    setRoleResources(newRR);
  };

  const addRole = () => {
    const name = zh ? "新角色" : "New Role";
    setRoles((prev) => [...prev, name]);
    setRoleResources((prev) => ({
      ...prev,
      [name]: {
        role: name,
        cluster: clusterDisplayNames[0] ?? "",
        nodeSelector: "",
        replicas: 0,
        cpu: "",
        memory: "",
        gpu: "0",
        devices: [],
        image: "",
        prepareScript: "",
        envs: [],
        mounts: [],
      },
    }));
  };
  const removeRole = (role: string) => {
    if (roles.length === 0) return;
    setRoles((prev) => prev.filter((r) => r !== role));
    setRoleResources((prev) => {
      const next = { ...prev };
      delete next[role];
      return next;
    });
    if (headerRole === role) setHeaderRole(roles[0]);
  };
  const renameRole = (oldName: string, newName: string) => {
    newName = newName.trim();
    if (!newName || oldName === newName) return;
    if (roles.includes(newName)) return;
    setRoles((prev) => prev.map((r) => (r === oldName ? newName : r)));
    setRoleResources((prev) => {
      const rr = prev[oldName];
      if (!rr) return prev;
      const next = { ...prev };
      delete next[oldName];
      next[newName] = {
        ...rr,
        role: newName,
        envs: rr.envs.map((e) =>
          e.key === "RLARK_TASK_ROLE" ? { ...e, value: newName } : e,
        ),
      };
      return next;
    });
    if (headerRole === oldName) setHeaderRole(newName);
  };

  const updateRR = (role: string, field: keyof RoleResource, v: any) => {
    setRoleResources((prev) => ({
      ...prev,
      [role]: { ...prev[role], [field]: v },
    }));
  };
  const updateRREnv = (
    role: string,
    i: number,
    field: "key" | "value",
    v: string,
  ) => {
    setRoleResources((prev) => {
      const rr = prev[role];
      const next = [...rr.envs];
      next[i] = { ...next[i], [field]: v };
      return { ...prev, [role]: { ...rr, envs: next } };
    });
  };
  const addRREnv = (role: string) => {
    setRoleResources((prev) => ({
      ...prev,
      [role]: {
        ...prev[role],
        envs: [...prev[role].envs, { key: "", value: "" }],
      },
    }));
  };
  const removeRREnv = (role: string, i: number) => {
    setRoleResources((prev) => ({
      ...prev,
      [role]: {
        ...prev[role],
        envs: prev[role].envs.filter((_, idx) => idx !== i),
      },
    }));
  };
  const updateRRMount = (
    role: string,
    i: number,
    field: "objectStorage" | "mountPath" | "type" | "hostPath",
    v: string,
  ) => {
    setRoleResources((prev) => {
      const rr = prev[role];
      const next = [...rr.mounts];
      next[i] = { ...next[i], [field]: v };
      const pvcStorageMap = computePvcStorageMap(role, next, jobName);
      return { ...prev, [role]: { ...rr, mounts: next, pvcStorageMap } };
    });
  };
  const addRRMount = (role: string) => {
    setRoleResources((prev) => {
      const rr = prev[role];
      const newMounts = [
        ...rr.mounts,
        {
          type: "host" as const,
          objectStorage: "",
          mountPath: "",
          hostPath: "",
        },
      ];
      const pvcStorageMap = computePvcStorageMap(role, newMounts, jobName);
      return { ...prev, [role]: { ...rr, mounts: newMounts, pvcStorageMap } };
    });
  };
  const removeRRMount = (role: string, i: number) => {
    setRoleResources((prev) => {
      const rr = prev[role];
      const newMounts = rr.mounts.filter((_, idx) => idx !== i);
      const pvcStorageMap = computePvcStorageMap(role, newMounts, jobName);
      return { ...prev, [role]: { ...rr, mounts: newMounts, pvcStorageMap } };
    });
  };

  const crd = generateJobCRD({
    name: jobName,
    type,
    headerRole: effectiveHeader,
    roles,
    roleResources,
    runScript,
    domain,
    tensorBoardDir,
    sshPublicKey,
  });
  const yaml = toYaml(crd);
  const steps = zh
    ? ["角色和资源", "Worker 配置", "公共配置", "YAML 预览"]
    : ["Roles & Resources", "Worker Config", "Common Config", "YAML Preview"];

  const validateStep = (targetStep: number) => {
    if (targetStep === 1) {
      const trimmedName = jobName.trim();
      if (!trimmedName) return zh ? "请输入任务名称。" : "Enter a job name.";
      if (
        trimmedName.length > 63 ||
        !/^[a-z0-9]([-a-z0-9]*[a-z0-9])?$/.test(trimmedName)
      )
        return zh
          ? "任务名称需为 1-63 位小写字母、数字或连字符，且不能以连字符开头或结尾。"
          : "Use 1-63 lowercase letters, numbers, or hyphens; do not start or end with a hyphen.";
      if (roles.length === 0)
        return zh ? "至少添加一个角色。" : "Add at least one role.";
      const normalizedRoles = roles.map((role) => role.trim().toLowerCase());
      if (normalizedRoles.some((role) => !role))
        return zh ? "角色名称不能为空。" : "Role names cannot be empty.";
      if (new Set(normalizedRoles).size !== normalizedRoles.length)
        return zh ? "角色名称不能重复。" : "Role names must be unique.";
      if (!effectiveHeader || !roles.includes(effectiveHeader))
        return zh ? "请选择 Header 角色。" : "Select a header role.";
    }

    if (targetStep === 2) {
      for (const role of roles) {
        const resource = roleResources[role];
        if (!resource?.cluster)
          return zh
            ? `请为 ${role} 选择集群。`
            : `Select a cluster for ${role}.`;
        if (!resource.image.trim())
          return zh ? `请为 ${role} 输入镜像。` : `Enter an image for ${role}.`;
        if (imageReferenceHasWhitespace(resource.image))
          return zh
            ? `${role} 的镜像地址不能包含空格或其他空白字符。`
            : `The image reference for ${role} cannot contain spaces or other whitespace.`;
        if (
          !Number.isFinite(Number(resource.replicas)) ||
          resource.replicas < 1
        )
          return zh
            ? `${role} 当前没有匹配到可用节点，请调整集群或节点选择条件。`
            : `${role} has no matched nodes. Adjust its cluster or node selector.`;
        const selector = parseNodeSelectorStr(resource.nodeSelector);
        const selectedNames = (selector["kubernetes.io/hostname"] ?? "")
          .split(",")
          .map((name) => name.trim())
          .filter(Boolean);
        const selectedNodes = allNodes.filter(
          (node) =>
            node.metadata.namespace === resource.cluster &&
            selectedNames.includes(node.metadata.name),
        );
        const resourceRequest =
          resource.gpu && resource.gpu !== "0"
            ? { key: "nvidia.com/gpu", amount: Number(resource.gpu) }
            : resource.devices[0]
              ? {
                  key: resource.devices[0].name,
                  amount: Number(resource.devices[0].quantity),
                }
              : null;
        if (resourceRequest && selectedNodes.length > 0) {
          const freeByNode = selectedNodes.map((node) => {
            const allocatable = Number(
              node.status?.allocatable?.[resourceRequest.key] ?? 0,
            );
            const used = Number(node.status?.used?.[resourceRequest.key] ?? 0);
            return Math.max(0, allocatable - used);
          });
          const available = freeByNode.reduce((sum, value) => sum + value, 0);
          const requested = resource.replicas * resourceRequest.amount;
          if (requested > available)
            return zh
              ? `${role} 申请 ${requested} 个资源，但所选资源池当前仅可用 ${available} 个。`
              : `${role} requests ${requested} resources, but only ${available} are free.`;
          if (freeByNode.every((free) => free < resourceRequest.amount))
            return zh
              ? `${role} 每个 Worker 申请 ${resourceRequest.amount} 个资源，但没有单个节点能够满足。`
              : `No node can satisfy ${role}'s per-worker request of ${resourceRequest.amount}.`;
        }
        for (const mount of resource.mounts) {
          if (!mount.mountPath.trim())
            return zh
              ? `${role} 的挂载目录不能为空。`
              : `${role} has a mount without a target path.`;
          if (mount.type === "host" && !mount.hostPath.trim())
            return zh
              ? `${role} 的主机目录不能为空。`
              : `${role} has an empty host path.`;
          if (mount.type === "storage" && !mount.objectStorage.trim())
            return zh
              ? `${role} 需要选择对象存储。`
              : `${role} needs an object storage class.`;
        }
      }
    }

    if (targetStep === 3 && !runScript.trim())
      return zh ? "请输入运行命令。" : "Enter a run command.";
    return "";
  };

  const goToStep = (nextStep: number) => {
    if (nextStep <= step) {
      setError("");
      setStep(nextStep);
      return;
    }
    for (let currentStep = 1; currentStep < nextStep; currentStep += 1) {
      const message = validateStep(currentStep);
      if (message) {
        setError(message);
        setErrorNonce((n) => n + 1);
        setStep(currentStep);
        return;
      }
    }
    setError("");
    setStep(nextStep);
  };

  const handleSubmit = async () => {
    for (let currentStep = 1; currentStep <= 3; currentStep += 1) {
      const message = validateStep(currentStep);
      if (message) {
        setError(message);
        setErrorNonce((n) => n + 1);
        setStep(currentStep);
        return;
      }
    }
    setSubmitting(true);
    setError("");
    try {
      const url = isEdit
        ? `/api/v1/rlinf.io/v1alpha1/jobs/${editJob!.name}`
        : "/api/v1/rlinf.io/v1alpha1/jobs";
      const method = isEdit ? "PUT" : "POST";
      const requestBody =
        isEdit && restartAfterSave
          ? {
              ...crd,
              metadata: {
                ...crd.metadata,
                annotations: {
                  "rlark.io/restarted-at": new Date().toISOString(),
                },
              },
              spec: { ...crd.spec, stopped: false },
            }
          : crd;
      const resp = await fetch(url, {
        method,
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(requestBody),
      });
      if (!resp.ok) {
        const body = await resp.text();
        throw new Error(`HTTP ${resp.status}: ${body}`);
      }
      onClose();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
      setErrorNonce((n) => n + 1);
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) =>
        e.target === e.currentTarget && !submitting && onClose()
      }
    >
      <div className="modal create-job-modal" role="dialog" aria-modal="true">
        <div className="modal-head">
          <div>
            <span className="eyebrow">
              {isEdit
                ? restartAfterSave
                  ? zh
                    ? "编辑后重启"
                    : "EDIT & RESTART"
                  : zh
                    ? "编辑任务"
                    : "EDIT JOB"
                : "NEW JOB"}
            </span>
            <h2>
              {isEdit
                ? restartAfterSave
                  ? zh
                    ? "编辑并重启任务"
                    : "Edit and restart job"
                  : zh
                    ? "编辑任务"
                    : "Edit Job"
                : c.jobs.createTitle}
            </h2>
          </div>
          <button
            className="icon-button"
            onClick={onClose}
            aria-label={zh ? "关闭创建任务" : "Close job creator"}
            disabled={submitting}
          >
            ×
          </button>
        </div>
        <div className="create-stepper">
          {steps.map((label, index) => (
            <button
              key={label}
              className={step >= index + 1 ? "active" : ""}
              onClick={() => goToStep(index + 1)}
            >
              <span>{index + 1}</span>
              {label}
            </button>
          ))}
        </div>
        <div className="modal-body create-job-body">
          {error && (
            <div
              className="form-error-banner"
              ref={errorBannerRef}
              role="alert"
            >
              {error}
            </div>
          )}
          {step === 1 && (
            <>
              <div className="form-row">
                <label>
                  {zh ? "任务名称" : "Job Name"}
                  <input
                    value={jobName}
                    onChange={(e) => setJobName(e.target.value)}
                    disabled={isEdit}
                  />
                </label>
                <label>
                  {zh ? "任务类型" : "Job Type"}
                  <select
                    value={type}
                    onChange={(e) => onTypeChange(e.target.value as JobType)}
                  >
                    <option value="RL">{c.jobType.RL}</option>
                    <option value="DataCollection">
                      {c.jobType.DataCollection}
                    </option>
                    <option value="Evaluation">{c.jobType.Evaluation}</option>
                    <option value="Custom">{c.jobType.Custom}</option>
                  </select>
                </label>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <strong>{zh ? "角色列表" : "Roles"}</strong>
                  <small>
                    {zh
                      ? "点击选择 Header 角色，可编辑角色名称、增删角色。"
                      : "Click to select header role. Roles can be renamed, added, or removed."}
                  </small>
                </div>
                <div className="role-edit-list">
                  {roles.map((role) => (
                    <div
                      key={role}
                      className={`role-edit-row ${effectiveHeader === role ? "active" : ""}`}
                      onClick={() => setHeaderRole(role)}
                    >
                      <Check size={14} />
                      <RoleNameInput role={role} onRename={renameRole} />
                      <small>
                        {effectiveHeader === role
                          ? zh
                            ? "Header"
                            : "Header"
                          : zh
                            ? "Worker"
                            : "Worker"}
                      </small>
                      <button
                        className="icon-button danger"
                        onClick={(e) => {
                          e.stopPropagation();
                          removeRole(role);
                        }}
                      >
                        <X size={14} />
                      </button>
                    </div>
                  ))}
                </div>
                <button
                  className="secondary-button"
                  style={{ marginTop: 8 }}
                  onClick={addRole}
                >
                  <Plus size={14} />
                  {zh ? "添加角色" : "Add Role"}
                </button>
              </div>
            </>
          )}
          {step === 2 &&
            (roles.length === 0 ? (
              <div className="empty-state-hint">
                {zh
                  ? "请先在「角色和资源」步骤中添加角色。"
                  : 'Please add roles in the "Roles & Resources" step first.'}
              </div>
            ) : (
              <>
                <div className="role-config-tabs">
                  {roles.map((role) => (
                    <button
                      key={role}
                      className={activeRoleTab === role ? "active" : ""}
                      onClick={() => setActiveRoleTab(role)}
                    >
                      {role}
                      {effectiveHeader === role && (
                        <span
                          className="role-chip"
                          style={{ marginLeft: 6, fontSize: 10 }}
                        >
                          Header
                        </span>
                      )}
                    </button>
                  ))}
                </div>
                {(() => {
                  const role = activeRoleTab || roles[0];
                  if (!role) return null;
                  const rr = roleResources[role];
                  if (!rr) return null;
                  const imageHasWhitespace = imageReferenceHasWhitespace(
                    rr.image,
                  );
                  const imageErrorId = `image-error-${role.replace(
                    /[^a-zA-Z0-9_-]/g,
                    "-",
                  )}`;
                  return (
                    <div className="role-resource-card" key={role}>
                      <div className="form-section-head">
                        <strong>{role}</strong>
                        {effectiveHeader === role && (
                          <span
                            className="role-chip"
                            style={{
                              background: "#f3eefe",
                              color: "var(--blue)",
                            }}
                          >
                            Header
                          </span>
                        )}
                      </div>
                      <section className="worker-config-section worker-placement-section">
                        <header className="worker-config-section-head">
                          <span className="worker-config-section-index">1</span>
                          <div>
                            <strong>
                              {zh ? "部署位置与资源" : "Placement & resources"}
                            </strong>
                            <small>
                              {zh
                                ? "选择设备规格和单 Worker 用量，再设置调度方式"
                                : "Choose a device specification and per-Worker request, then set scheduling"}
                            </small>
                          </div>
                        </header>
                        <div className="worker-cluster-field">
                          <span>{zh ? "目标集群" : "Target cluster"}</span>
                          <ClusterSelect
                            clusters={availableClusters}
                            value={rr.cluster}
                            zh={zh}
                            onChange={(newCluster) => {
                              updateRR(role, "cluster", newCluster);
                              if (rr.mounts.some((m) => m.type === "storage")) {
                                fetchStorageClasses(newCluster);
                              }
                            }}
                          />
                        </div>
                        <ResourcePlacementPicker
                          zh={zh}
                          cluster={rr.cluster}
                          nodes={allNodes}
                          loading={nodesLoading}
                          replicas={rr.replicas}
                          gpu={rr.gpu}
                          devices={rr.devices ?? []}
                          onChange={(placement) =>
                            setRoleResources((previous) => ({
                              ...previous,
                              [role]: { ...previous[role], ...placement },
                            }))
                          }
                        />
                      </section>
                      <section className="worker-config-section worker-runtime-section">
                        <header className="worker-config-section-head">
                          <span className="worker-config-section-index">2</span>
                          <div>
                            <strong>
                              {zh ? "容器运行配置" : "Container runtime"}
                            </strong>
                            <small>
                              {zh
                                ? "配置镜像、启动前脚本、环境变量和存储挂载"
                                : "Configure image, prepare script, environment and storage"}
                            </small>
                          </div>
                        </header>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>{zh ? "镜像" : "Image"}</small>
                          </div>
                          <input
                            value={rr.image}
                            onChange={(e) =>
                              updateRR(role, "image", e.target.value)
                            }
                            className={
                              imageHasWhitespace ? "input-invalid" : undefined
                            }
                            aria-invalid={imageHasWhitespace}
                            aria-describedby={
                              imageHasWhitespace ? imageErrorId : undefined
                            }
                            placeholder={
                              zh
                                ? "例如：docker.io/library/ubuntu:22.04（需确保集群可访问）"
                                : "Example: docker.io/library/ubuntu:22.04 (must be cluster-accessible)"
                            }
                          />
                          {imageHasWhitespace && (
                            <small
                              className="field-validation-error"
                              id={imageErrorId}
                              role="alert"
                            >
                              {zh
                                ? "镜像地址不能包含空格或其他空白字符。"
                                : "Image references cannot contain spaces or other whitespace."}
                            </small>
                          )}
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>
                              {zh
                                ? "准备脚本 (Ray 启动前)"
                                : "Prepare Script (before Ray starts)"}
                            </small>
                          </div>
                          <CodeEditorField
                            value={rr.prepareScript}
                            onChange={(e) =>
                              updateRR(role, "prepareScript", e.target.value)
                            }
                            minHeight={92}
                            label={`${role}/prepare.sh`}
                            placeholder={
                              zh
                                ? "pip install ray[default] or other setup commands"
                                : "pip install ray[default] or other setup commands"
                            }
                          />
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>
                              {zh ? "环境变量" : "Environment Variables"}
                            </small>
                            <button
                              className="secondary-button"
                              onClick={() => addRREnv(role)}
                            >
                              <Plus size={14} />
                              {zh ? "添加" : "Add"}
                            </button>
                          </div>
                          {rr.envs.map((env, index) => (
                            <div className="env-row" key={index}>
                              <input
                                value={env.key}
                                onChange={(e) =>
                                  updateRREnv(
                                    role,
                                    index,
                                    "key",
                                    e.target.value,
                                  )
                                }
                                placeholder="KEY"
                              />
                              <input
                                value={env.value}
                                onChange={(e) =>
                                  updateRREnv(
                                    role,
                                    index,
                                    "value",
                                    e.target.value,
                                  )
                                }
                                placeholder="value"
                              />
                              <button
                                className="icon-button danger"
                                onClick={() => removeRREnv(role, index)}
                              >
                                <X size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                        <div className="form-section" style={{ marginTop: 12 }}>
                          <div className="form-section-head">
                            <small>{zh ? "存储挂载" : "Volume Mounts"}</small>
                            <button
                              className="secondary-button"
                              onClick={() => addRRMount(role)}
                            >
                              <Plus size={14} />
                              {zh ? "添加" : "Add"}
                            </button>
                          </div>
                          {rr.mounts.map((mount, index) => (
                            <div className="mount-row" key={index}>
                              <div
                                className="mount-type-toggle"
                                onClick={(e) => {
                                  const next =
                                    mount.type === "storage"
                                      ? "host"
                                      : "storage";
                                  updateRRMount(role, index, "type", next);
                                  if (next === "storage") {
                                    fetchStorageClasses(rr.cluster);
                                  }
                                }}
                              >
                                <button
                                  type="button"
                                  className={
                                    mount.type === "host" ? "active" : ""
                                  }
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    updateRRMount(role, index, "type", "host");
                                  }}
                                >
                                  {zh ? "主机目录" : "Host directory"}
                                </button>
                                <button
                                  type="button"
                                  className={
                                    mount.type === "storage" ? "active" : ""
                                  }
                                  onClick={(e) => {
                                    e.stopPropagation();
                                    updateRRMount(
                                      role,
                                      index,
                                      "type",
                                      "storage",
                                    );
                                    fetchStorageClasses(rr.cluster);
                                  }}
                                >
                                  {zh ? "对象存储" : "Object storage"}
                                </button>
                              </div>
                              <label className="mount-field-box">
                                <span>
                                  {mount.type === "storage"
                                    ? zh
                                      ? "对象存储"
                                      : "Object storage"
                                    : zh
                                      ? "主机目录"
                                      : "Host directory"}
                                </span>
                                {mount.type === "storage" ? (
                                  <select
                                    value={mount.objectStorage}
                                    onChange={(e) =>
                                      updateRRMount(
                                        role,
                                        index,
                                        "objectStorage",
                                        e.target.value,
                                      )
                                    }
                                  >
                                    <option value="">
                                      {storageClassFetched &&
                                      storageClasses.length === 0
                                        ? zh
                                          ? "无可用存储类"
                                          : "No storage classes"
                                        : zh
                                          ? "选择存储类"
                                          : "Select storage class"}
                                    </option>
                                    {storageClasses.map((sc) => (
                                      <option key={sc.name} value={sc.name}>
                                        {sc.name}
                                      </option>
                                    ))}
                                  </select>
                                ) : (
                                  <input
                                    value={mount.hostPath}
                                    onChange={(e) =>
                                      updateRRMount(
                                        role,
                                        index,
                                        "hostPath",
                                        e.target.value,
                                      )
                                    }
                                    placeholder="/host/path"
                                  />
                                )}
                              </label>
                              <label className="mount-field-box">
                                <span>
                                  {zh ? "挂载到 Worker" : "Mount in worker"}
                                </span>
                                <input
                                  value={mount.mountPath}
                                  onChange={(e) =>
                                    updateRRMount(
                                      role,
                                      index,
                                      "mountPath",
                                      e.target.value,
                                    )
                                  }
                                  placeholder="/mnt/data"
                                />
                              </label>
                              <button
                                className="icon-button danger"
                                onClick={() => removeRRMount(role, index)}
                                title={zh ? "删除" : "Delete"}
                              >
                                <Trash2 size={14} />
                              </button>
                            </div>
                          ))}
                        </div>
                      </section>
                    </div>
                  );
                })()}
              </>
            ))}
          {step === 3 && (
            <>
              <div className="form-section">
                <div className="form-section-head">
                  <strong>{zh ? "选择 Head 节点" : "Select Head"}</strong>
                  <small>
                    {zh
                      ? "系统会从用户指定的 Header 角色里选择第一个 Worker 作为 Header。"
                      : "The first worker of the header role becomes the job header."}
                  </small>
                </div>
                <div className="role-template selectable">
                  {roles.map((role) => (
                    <button
                      key={role}
                      className={effectiveHeader === role ? "active" : ""}
                      onClick={() => setHeaderRole(role)}
                    >
                      <Check size={13} />
                      {role}
                      <small>
                        {effectiveHeader === role ? "Header" : "Worker"}
                      </small>
                    </button>
                  ))}
                </div>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>
                    {zh
                      ? "跨集群网络域 (可选)"
                      : "Cross-cluster Network Domain (optional)"}
                  </small>
                </div>
                <select
                  value={domain}
                  onChange={(e) => setDomain(e.target.value)}
                >
                  <option value="">
                    {zh ? "不使用跨集群网络" : "No cross-cluster network"}
                  </option>
                  {domains.map((d) => (
                    <option key={d.name} value={d.name}>
                      {d.name} ({d.cidr})
                    </option>
                  ))}
                </select>
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>
                    {zh
                      ? "SSH 公钥注入 (可选)"
                      : "SSH Public Key Injection (optional)"}
                  </small>
                </div>
                <select
                  value={sshPublicKey}
                  onChange={(e) => setSSHPublicKey(e.target.value)}
                >
                  <option value="">
                    {zh ? "不注入 SSH 公钥" : "No SSH key injection"}
                  </option>
                  {sshKeys.map((k) => (
                    <option key={`${k.user}-${k.index}`} value={k.public_key}>
                      {k.user} #{k.index + 1} ({k.public_key.slice(0, 40)}...)
                    </option>
                  ))}
                </select>
                {sshKeysLoaded && sshKeys.length === 0 && (
                  <small className="field-hint">
                    {zh
                      ? "未找到 SSH 公钥，请先在 SSH 公钥管理页面添加。"
                      : "No SSH keys found. Add keys in the SSH Key Management page first."}
                  </small>
                )}
                {sshPublicKey && (
                  <small className="field-hint">
                    {zh
                      ? "选中的公钥将注入到所有角色的 Pod 的 authorized_keys 中。"
                      : "The selected public key will be injected into all role pods' authorized_keys."}
                  </small>
                )}
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>
                    {zh
                      ? "运行脚本 (Ray 集群就绪后, 仅 Head 节点)"
                      : "Run Script (after Ray cluster ready, head only)"}
                  </small>
                </div>
                <CodeEditorField
                  value={runScript}
                  onChange={(e) => setRunScript(e.target.value)}
                  minHeight={112}
                  label="run.sh"
                  placeholder="python train.py --config /mnt/config/train.yaml"
                />
              </div>
              <div className="form-section">
                <div className="form-section-head">
                  <small>
                    {zh
                      ? "TensorBoard 日志目录 (可选)"
                      : "TensorBoard Log Directory (optional)"}
                  </small>
                </div>
                <input
                  value={tensorBoardDir}
                  onChange={(e) => setTensorBoardDir(e.target.value)}
                  placeholder="/data/tensorboard/train"
                />
              </div>
            </>
          )}
          {step === 4 && (
            <div className="yaml-preview">
              <div>
                <strong>{zh ? "任务 YAML 预览" : "Job YAML Preview"}</strong>
                <small>
                  {zh
                    ? "提交前可检查最终资源定义。"
                    : "Review the final resource definition before submitting."}
                </small>
              </div>
              {error && (
                <div className="cert-error" style={{ marginBottom: 12 }}>
                  {error}
                </div>
              )}
              <pre>{yaml}</pre>
            </div>
          )}
          <div className="step-actions">
            {step > 1 && (
              <button
                className="secondary-button"
                onClick={() => goToStep(step - 1)}
              >
                {zh ? "上一步" : "Previous"}
              </button>
            )}
            {step < 4 ? (
              <button
                className="primary-button"
                onClick={() => goToStep(step + 1)}
              >
                {step === 2 &&
                roles.length > 1 &&
                (activeRoleTab || roles[0]) !== roles[roles.length - 1]
                  ? zh
                    ? "下一个角色"
                    : "Next Role"
                  : zh
                    ? "下一步"
                    : "Next"}
              </button>
            ) : (
              <button
                className="primary-button"
                disabled={submitting || roles.length === 0}
                onClick={handleSubmit}
              >
                {submitting
                  ? zh
                    ? "提交中…"
                    : "Submitting…"
                  : zh
                    ? isEdit
                      ? restartAfterSave
                        ? "保存并重启"
                        : "保存修改"
                      : "提交任务"
                    : isEdit
                      ? restartAfterSave
                        ? "Save & Restart"
                        : "Save Changes"
                      : "Submit Job"}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
