import { clusters, type StorageClass } from "./data";
import type { CRDDomain, CRDJob, CRDWorkflow } from "./types";
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
      pvcStorageMap: { data: "training-datasets" },
      template: {
        spec: {
          containers: [
            {
              name,
              image,
              env: [{ name: "RLARK_TASK_ROLE", value: role }],
              volumeMounts: [{ name: "data", mountPath: "/data" }],
              resources: {
                requests: { cpu: "4", memory: "8Gi", "nvidia.com/gpu": "1" },
              },
            },
          ],
          volumes: [
            { name: "data", persistentVolumeClaim: { claimName: "data" } },
          ],
        },
      },
    },
  },
});

const jobs: CRDJob[] = [
  ["robot-policy-training", 0, "Running"],
  ["warehouse-evaluation", 1, "Succeeded"],
  ["vision-data-collection", 2, "Pending"],
].map(([name, indexValue, phase]) => {
  const index = Number(indexValue);
  const cluster = clusterNames[index % clusterNames.length];
  const taskNames = ["actor", "rollout"];
  return {
    apiVersion: "rlinf.io/v1alpha1",
    kind: "Job",
    metadata: {
      name: String(name),
      creationTimestamp: `2026-08-0${index + 6}T09:00:00Z`,
    },
    spec: {
      domain: domains[index % domains.length].metadata.name,
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

const storageClasses: StorageClass[] = [
  {
    id: "training-datasets",
    name: "training-datasets",
    namespace: "default",
    provider: "MinIO",
    clusters: clusterNames,
    endpoint: "https://minio.mock.local",
    region: "local",
    bucket: "rlark-training",
    accessKeyId: "mock-access-key",
    pathStyle: true,
    description: "Shared datasets for the mock topology",
    createdAt: "2026-08-01T08:00:00Z",
  },
  {
    id: "evaluation-results",
    name: "evaluation-results",
    namespace: "default",
    provider: "AWS S3",
    clusters: clusterNames.slice(0, 2),
    endpoint: "https://s3.mock.local",
    region: "cn-east-1",
    bucket: "rlark-evaluation",
    accessKeyId: "mock-access-key",
    pathStyle: false,
    description: "Evaluation artifacts",
    createdAt: "2026-08-02T08:00:00Z",
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
      if (typeof patch.spec?.unschedulable === "boolean") {
        node.spec.unschedulable = patch.spec.unschedulable;
      }
      return json(node);
    }
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/jobs")
      return json({ items: jobs });
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
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/domains")
      return json({ items: domains });
    if (method === "GET" && path === "/api/v1/rlinf.io/v1alpha1/workflows")
      return json({ items: workflows });
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
        namespace: payload.namespace || "default",
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
    if (method === "GET" && path.includes("/list"))
      return json({ data: { objects: [] } });
    if (method === "GET" && path === "/api/v1/ssh-user-keys") return json([]);
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
