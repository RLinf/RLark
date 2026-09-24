import {
  clusters,
  storageClasses as mockStorageClasses,
  type StorageClass,
} from "./data";
import type { CRDDomain, CRDJob, CRDWorkflow, NodeEventEntry } from "./types";
import { buildMockCRDNodes } from "./utils/nodes";

const nodes = buildMockCRDNodes();
const clusterNames = [
  ...new Set(nodes.map((node) => node.metadata.namespace!)),
];

const domains: CRDDomain[] = clusterNames.map((name, index) => ({
  apiVersion: "rlinf.io/v1alpha1",
  kind: "Domain",
  metadata: {
    name: `domain-${index + 1}`,
    creationTimestamp: "2026-08-01T08:00:00Z",
  },
  spec: { cidr: `10.${80 + index}.0.0/16` },
  status: { ipAllocations: [] },
}));

const makeTask = (
  name: string,
  cluster: string,
  role: string,
  image: string,
) => ({
  name,
  head: role === "actor",
  agentType: "Kubernetes",
  role,
  nodeSelector: { "rlark.io/cluster-id": cluster },
  runScript: `python -m rlark.${role}`,
  tensorBoardDir: role === "actor" ? "/logs/tensorboard" : undefined,
  kubernetes: {
    workload: {
      kind: "Deployment",
      replicas: 1,
      template: {
        spec: {
          containers: [
            {
              name,
              image,
              env: [{ name: "RLARK_TASK_ROLE", value: role }],
              volumeMounts: [
                { name: "data", mountPath: "/data" },
                { name: "local-cache", mountPath: "/cache" },
              ],
              resources: {
                requests: { cpu: "4", memory: "8Gi", "nvidia.com/gpu": "1" },
              },
            },
          ],
          volumes: [
            {
              name: "data",
              ephemeral: {
                volumeClaimTemplate: {
                  spec: {
                    accessModes: ["ReadWriteOnce"],
                    storageClassName: "training-datasets",
                    resources: { requests: { storage: "50Gi" } },
                  },
                },
              },
            },
            { name: "local-cache", hostPath: { path: "/var/lib/rlark/cache" } },
          ],
        },
      },
    },
  },
});

const jobTagPool = [
  { key: "project", values: ["rlark", "console", "vision", "robot"] },
  { key: "environment", values: ["development", "staging", "production"] },
  { key: "team", values: ["platform", "runtime", "frontend"] },
  { key: "priority", values: ["high", "medium", "low"] },
  { key: "wrtest", values: ["collection", "train", "training"] },
  { key: "workload", values: ["training", "evaluation"] },
  { key: "hardware", values: ["gpu-a100", "jetson-orin"] },
  { key: "region", values: ["cn-north", "cn-east", "cn-south"] },
  { key: "owner", values: ["alice", "bob", "carol"] },
  { key: "experiment", values: ["baseline", "ablation", "sweep"] },
  { key: "phase", values: ["prepare", "train", "eval"] },
  { key: "dataset", values: ["images", "pointclouds", "speech"] },
];

// 每个任务固定生成 10 个不同 key 的标签，便于验证多标签展示与筛选
const makeJobTags = (index: number) =>
  Array.from({ length: 10 }, (_, offset) => {
    const { key, values } =
      jobTagPool[(index * 3 + offset) % jobTagPool.length];
    return { key, values: [values[index % values.length]] };
  });

const jobs: CRDJob[] = [
  // 创建时间最新，desc 排序时排在列表最前面
  ["sim-replay-pipeline", 13, "Running"],
  ["scene-bake-rendering", 12, "Succeeded"],
  ["nav-policy-distill", 11, "Running"],
  ["grasp-dataset-augment", 10, "Succeeded"],
  ["lidar-calibration-suite", 9, "Pending"],
  ["edge-mapping-benchmark", 8, "Running"],
  ["talker-finetune-sft", 7, "Succeeded"],
  ["vision-data-collection", 6, "Pending"],
  ["slam-bag-replay", 5, "Running"],
  ["warehouse-evaluation", 4, "Succeeded"],
  ["robot-policy-training", 4, "Running"],
  ["dual-arm-transfer", 3, "Pending"],
  ["object-6d-pose-estim", 2, "Running"],
  ["audio-command-parser", 1, "Succeeded"],
  // 创建时间最早（其余任务均晚于它），desc 排序时固定落在列表页底部，
  // 且标签数量多，用于验证多标签展开弹层在屏幕底部自动翻转的修复
  ["multi-tag-preview", 0, "Running"],
].map(([name, indexValue, phase]) => {
  const index = Number(indexValue);
  const cluster = clusterNames[index % clusterNames.length];
  const taskNames = ["actor", "rollout"];
  return {
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Job",
    metadata: {
      name: String(name),
      creationTimestamp: `2026-08-${String(index + 6).padStart(2, "0")}T09:00:00Z`,
    },
    spec: {
      domain: domains[index % domains.length].metadata.name,
      tags: makeJobTags(index),
      tasks: [
        makeTask(
          taskNames[0],
          cluster,
          "actor",
          "registry.local/rlark/trainer:v1",
        ),
        makeTask(
          taskNames[1],
          cluster,
          "rollout",
          "registry.local/rlark/rollout:v1",
        ),
      ],
    },
    status: {
      phase: String(phase),
      tasks: taskNames.map((taskName, taskIndex) => ({
        name: taskName,
        phase:
          phase === "Succeeded"
            ? "Succeeded"
            : taskIndex === 0
              ? String(phase)
              : "Pending",
        message: "Mock data generated from the shared topology",
        observedNodes: [
          nodes.filter((node) => node.metadata.namespace === cluster)[taskIndex]
            ?.metadata.name,
        ].filter(Boolean) as string[],
      })),
    },
  };
});

const pods = jobs.flatMap((job, jobIndex) =>
  (job.status?.tasks ?? []).map((task, taskIndex) => {
    const podName = `${job.metadata.name}-${task.name}-${taskIndex}`;
    const domain = job.spec.domain ?? "";
    const node = task.observedNodes?.[0] ?? "";
    return {
      apiVersion: "rlinf.io/v1alpha1",
      kind: "Pod",
      metadata: { name: podName, namespace: "default" },
      spec: {
        taskName: task.name,
        taskNamespace: "default",
        podName,
        podNamespace: "default",
        domain,
      },
      status: {
        phase: task.phase,
        node,
        ip: `10.${80 + jobIndex}.0.${20 + taskIndex}`,
        message: task.message,
      },
    };
  }),
);

const pendingWorkerEvents: NodeEventEntry[] = [
  {
    type: "Warning",
    reason: "FailedScheduling",
    message: "Insufficient GPU resources for the requested worker.",
    lastTime: "2026-08-08T09:05:00Z",
    objectKind: "Pod",
  },
  {
    type: "Warning",
    reason: "ImagePullBackOff",
    message: "Back-off pulling the runtime image.",
    lastTime: "2026-08-08T09:06:00Z",
    objectKind: "Pod",
  },
  {
    type: "Warning",
    reason: "FailedMount",
    message: "The training dataset volume is not mounted yet.",
    lastTime: "2026-08-08T09:07:00Z",
    objectKind: "Pod",
  },
  {
    type: "Warning",
    reason: "NodeNotReady",
    message: "The selected node is unavailable because of memory pressure.",
    lastTime: "2026-08-08T09:08:00Z",
    objectKind: "Node",
  },
  {
    type: "Warning",
    reason: "FailedCreatePodSandBox",
    message: "The worker runtime environment is not ready.",
    lastTime: "2026-08-08T09:09:00Z",
    objectKind: "Pod",
  },
];

const pendingWorkerEventMap = Object.fromEntries(
  pods
    .filter(
      (pod) =>
        pod.metadata.name.startsWith("vision-data-collection-") &&
        pod.status.phase === "Pending",
    )
    .map((pod, index) => [
      pod.metadata.name,
      pendingWorkerEvents.map((event) => ({
        ...event,
        objectName:
          event.objectKind === "Pod" ? pod.spec.podName : pod.status.node,
        lastTime: new Date(
          Date.parse(event.lastTime ?? "") + index * 60_000,
        ).toISOString(),
      })),
    ]),
);

domains.forEach((domain) => {
  domain.status = {
    ipAllocations: pods
      .filter((pod) => pod.spec.domain === domain.metadata.name)
      .map((pod) => ({
        ip: pod.status.ip,
        job: pod.metadata.name.split("-").slice(0, -3).join("-"),
        task: pod.spec.taskName,
        pod: pod.spec.podName,
      })),
  };
});

const workflows: CRDWorkflow[] = [
  {
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Workflow",
    metadata: {
      name: "embodied-training-pipeline",
      creationTimestamp: "2026-08-05T08:30:00Z",
    },
    spec: {
      jobTemplates: jobs.map((job, index) => ({
        name: job.metadata.name,
        dependencies: index === 0 ? [] : [jobs[index - 1].metadata.name],
        spec: job.spec,
      })),
    },
    status: {
      phase: "Running",
      startTime: "2026-08-05T08:30:00Z",
      jobs: jobs.map((job) => ({
        name: job.metadata.name,
        phase: job.status?.phase ?? "Pending",
        message: "",
      })),
    },
  },
];

const storageClasses: StorageClass[] = mockStorageClasses.map((item) => ({
  ...item,
  clusters: item.clusters.length > 0 ? item.clusters : clusterNames,
}));

const imageRegistries = [
  {
    id: "ir-0123456789abcdef",
    name: "Mock Harbor",
    registry: "registry.example.com",
    username: "robot",
    clusterSelection: { mode: "All", clusters: [] as string[] },
  },
];

const json = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });

function clusterPayload() {
  return clusters.map((cluster) => ({
    ...cluster,
    id: cluster.name,
    name: cluster.name,
  }));
}

const sshUserKeys = [
  {
    index: 0,
    user: "admin",
    public_key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIMockAdminKey rlark-admin",
    added_at: "2026-08-10T08:30:00Z",
  },
];

export function installMockBackend() {
  const nativeFetch = window.fetch.bind(window);
  window.fetch = async (input: RequestInfo | URL, init?: RequestInit) => {
    const request = new Request(input, init);
    const url = new URL(request.url, window.location.origin);
    if (!url.pathname.startsWith("/api/")) return nativeFetch(input, init);
    const method = request.method.toUpperCase();
    const path = url.pathname;

    if (method === "GET" && path === "/api/v1/clusters")
      return json({ data: clusterPayload() });
    if (method === "GET" && path.startsWith("/api/v1/clusters/")) {
      const id = decodeURIComponent(path.split("/").pop()!);
      const cluster = clusterPayload().find((item) => item.id === id);
      return cluster
        ? json({
            data: {
              ...cluster,
              nodes: nodes.filter((node) => node.metadata.namespace === id),
            },
          })
        : json({ error: "not found" }, 404);
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/nodes")
      return json({ items: nodes });
    if (
      method === "GET" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/nodes/")
    ) {
      const name = decodeURIComponent(path.split("/").pop()!);
      const node = nodes.find((item) => item.metadata.name === name);
      return node ? json(node) : json({ error: "not found" }, 404);
    }
    if (
      method === "PATCH" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/nodes/")
    ) {
      const name = decodeURIComponent(path.split("/").pop()!);
      const node = nodes.find((item) => item.metadata.name === name);
      if (!node) return json({ error: "not found" }, 404);
      const patch = await request.json();
      if (patch.metadata?.labels) {
        node.metadata.labels = { ...patch.metadata.labels };
      }
      if (patch.metadata?.annotations) {
        node.metadata.annotations = { ...patch.metadata.annotations };
      }
      if (typeof patch.spec?.unschedulable === "boolean") {
        node.spec.unschedulable = patch.spec.unschedulable;
      }
      return json(node);
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/jobs/tags")
      return json({ items: jobTagPool });
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/jobs")
      return json({ items: jobs });
    if (
      method === "PATCH" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/jobs/") &&
      !path.endsWith("/logs")
    ) {
      const name = decodeURIComponent(path.split("/").pop()!);
      const job = jobs.find((item) => item.metadata.name === name);
      if (!job) return json({ error: "not found" }, 404);
      const patch = await request.json();
      if (typeof patch.spec?.stopped === "boolean") {
        job.spec.stopped = patch.spec.stopped;
        if (patch.spec.stopped) {
          job.status = { ...job.status, phase: "Stopped" };
        } else {
          job.status = {
            ...job.status,
            phase: "Pending",
            tasks: job.status?.tasks?.map((task) => ({
              ...task,
              phase: "Pending",
            })),
          };
        }
      }
      if (patch.metadata?.annotations) {
        job.metadata.annotations = {
          ...job.metadata.annotations,
          ...patch.metadata.annotations,
        };
      }
      return json(job);
    }
    if (
      method === "DELETE" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/jobs/")
    ) {
      const name = decodeURIComponent(path.split("/").pop()!);
      const index = jobs.findIndex((item) => item.metadata.name === name);
      if (index >= 0) jobs.splice(index, 1);
      return json({ success: true });
    }
    if (
      method === "GET" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/jobs/") &&
      path.endsWith("/logs")
    ) {
      const jobName = decodeURIComponent(path.split("/").at(-2)!);
      return json({
        pods: pods
          .filter((pod) => pod.metadata.name.startsWith(jobName))
          .map((pod) => ({
            taskName: pod.spec.taskName,
            podName: pod.spec.podName,
            phase: pod.status.phase,
            node: pod.status.node,
            logs: `[${jobName}] mock workload started\nShared topology loaded\n${pod.spec.taskName} is ${pod.status.phase}`,
          })),
      });
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/tasks") {
      const selectedJob = url.searchParams
        .get("labelSelector")
        ?.match(/rlinf\.io\/job=([^,]+)/)?.[1];
      const selectedJobs = selectedJob
        ? jobs.filter((job) => job.metadata.name === selectedJob)
        : jobs;
      return json({
        items: selectedJobs.flatMap((job) =>
          (job.status?.tasks ?? []).map((task) => ({
            apiVersion: "rlinf.io/v1alpha1",
            kind: "Task",
            metadata: {
              name: `${job.metadata.name}-${task.name}`,
              labels: { "rlinf.io/job": job.metadata.name },
            },
            status: { ...task },
          })),
        ),
      });
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/pods") {
      const selector = decodeURIComponent(
        url.searchParams.get("labelSelector") ?? "",
      );
      const selectedJob = jobs.find((job) =>
        selector.includes(`${job.metadata.name}-`),
      );
      return json({
        items: selectedJob
          ? pods.filter((pod) =>
              pod.metadata.name.startsWith(`${selectedJob.metadata.name}-`),
            )
          : pods,
      });
    }
    if (
      method === "GET" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/pods/") &&
      path.endsWith("/events")
    ) {
      const podName = decodeURIComponent(path.split("/").at(-2)!);
      return json({ events: pendingWorkerEventMap[podName] ?? [] });
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/domains")
      return json({ items: domains });
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/workflows")
      return json({ items: workflows });
    if (
      method === "PATCH" &&
      path.startsWith("/api/v1/rlinf.io/v1alpha1/workflows/")
    ) {
      const name = decodeURIComponent(path.split("/").pop()!);
      const workflow = workflows.find((item) => item.metadata.name === name);
      if (!workflow) return json({ error: "not found" }, 404);
      const patch = await request.json();
      if (typeof patch.spec?.stopped === "boolean") {
        workflow.spec.stopped = patch.spec.stopped;
        workflow.status = {
          ...workflow.status,
          phase: patch.spec.stopped ? "Stopping" : "Running",
        };
      }
      return json(workflow);
    }
    if (method === "GET" && path === "/api/v1/storage/storageclass")
      return json({
        data: Object.fromEntries(storageClasses.map((item) => [item.id, item])),
      });
    if (
      (method === "POST" && path === "/api/v1/storage/storageclass") ||
      (method === "PUT" && path.startsWith("/api/v1/storage/storageclass/"))
    ) {
      const payload = await request.json();
      const pathName =
        method === "PUT" ? decodeURIComponent(path.split("/").pop() || "") : "";
      const name = payload.name || pathName;
      if (!name) return json({ error: "name is required" }, 400);
      const next: StorageClass = {
        id: name,
        name,
        namespace: "kube-system",
        provider: payload.provider || "MinIO",
        clusters: Array.isArray(payload.clusters) ? payload.clusters : [],
        endpoint: payload.endpoint || "",
        region: payload.region || "",
        bucket: payload.bucket || "",
        accessKeyId: payload.access_key_id || payload.accessKeyId || "",
        pathStyle: Boolean(payload.path_style ?? payload.pathStyle),
        description: payload.description || "",
        createdAt:
          storageClasses.find((item) => item.name === name)?.createdAt ||
          new Date().toISOString(),
      };
      const index = storageClasses.findIndex(
        (item) => item.name === name || item.id === name,
      );
      if (index >= 0) storageClasses[index] = next;
      else storageClasses.push(next);
      return json({ success: true, data: next });
    }
    if (
      method === "DELETE" &&
      path.startsWith("/api/v1/storage/storageclass/")
    ) {
      const name = decodeURIComponent(path.split("/").pop() || "");
      const index = storageClasses.findIndex(
        (item) => item.name === name || item.id === name,
      );
      if (index >= 0) storageClasses.splice(index, 1);
      return json({ success: true });
    }
    if (method === "GET" && path === "/api/v1/image-registries")
      return json(imageRegistries);
    if (method === "POST" && path === "/api/v1/image-registries") {
      const payload = await request.json();
      const item = {
        id: `ir-${crypto.randomUUID().replaceAll("-", "").slice(0, 16)}`,
        name: payload.name,
        registry: payload.registry,
        username: payload.username,
        clusterSelection: payload.clusterSelection,
      };
      imageRegistries.push(item);
      return json(item, 201);
    }
    if (path.startsWith("/api/v1/image-registries/")) {
      const id = decodeURIComponent(path.split("/").pop() || "");
      const index = imageRegistries.findIndex((item) => item.id === id);
      if (index < 0) return json({ error: "not found" }, 404);
      if (method === "GET") return json(imageRegistries[index]);
      if (method === "PUT") {
        const payload = await request.json();
        imageRegistries[index] = {
          ...imageRegistries[index],
          name: payload.name,
          registry: payload.registry,
          username: payload.username,
          clusterSelection: payload.clusterSelection,
        };
        return json(imageRegistries[index]);
      }
      if (method === "DELETE") {
        imageRegistries.splice(index, 1);
        return json({ ok: true }, 202);
      }
    }
    if (method === "GET" && path.includes("/list"))
      return json({ data: { objects: [] } });
    if (method === "GET" && path === "/api/v1/ssh-user-keys")
      return json(sshUserKeys);
    if (method === "POST" && path === "/api/v1/ssh-user-keys") {
      const payload = await request.json();
      if (sshUserKeys.some((item) => item.user === payload.user))
        return json({ error: "public key name already exists" }, 409);
      if (sshUserKeys.some((item) => item.public_key === payload.public_key))
        return json({ error: "public key already exists" }, 409);
      sshUserKeys.push({
        index: sshUserKeys.length,
        user: payload.user,
        public_key: payload.public_key,
        added_at: new Date().toISOString(),
      });
      return json({ success: true });
    }
    if (method === "DELETE" && path.startsWith("/api/v1/ssh-user-keys/")) {
      const index = Number(path.split("/").pop());
      const user = url.searchParams.get("user");
      const keyIndex = sshUserKeys.findIndex(
        (item) => item.index === index && item.user === user,
      );
      if (keyIndex >= 0) sshUserKeys.splice(keyIndex, 1);
      sshUserKeys.forEach((item, itemIndex) => {
        item.index = itemIndex;
      });
      return json({ success: true });
    }
    if (method === "GET" && path === "/api/v1/addons")
      return json({ data: [] });
    if (method === "GET" && path === "/api/v1/installed-addons")
      return json({ data: [] });
    if (method === "GET" && path === "/api/v1/certificates/agent")
      return json([]);
    if (["POST", "PUT", "PATCH", "DELETE"].includes(method))
      return json({ data: {}, mock: true });
    return json({ error: `No mock route for ${method} ${path}` }, 404);
  };
}
