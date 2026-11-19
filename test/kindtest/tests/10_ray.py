"""REG: cross-cluster Ray - one 测试流程 with four 测试观测点.

The flow (procedure) runs *once*: it creates the Job, waits it Ready and tears it
down.  Each ``with t.observe(...)`` is an observation point that produces one
report row.  The Job uses the redesigned contract
(placement/runtime/workload/carrier), matching the deployed target schema.
"""

from __future__ import annotations

from kindtest import group

reg = group("REG", title="基本功能回归")

RAY_JOB = """
apiVersion: rlinf.io/v1alpha1
kind: Job
metadata:
  name: {{runId}}-ray
  labels:
    {{labelKey}}: {{runId}}-ray
spec:
  domain: {{domain}}
  tasks:
  - name: head
    head: true
    role: Actor
    placement:
      nodeSelector:
        kubernetes.io/hostname: {{nodeA}}
    runtime:
      kind: Kubernetes
    workload:
      kind: rlinf.io/ray
      version: v1
      parameters:
        role: head
      carrier:
        type: rlinf.io/kubernetes-statefulset
        version: v1
        parameters:
          replicas: 1
      template:
        spec:
          containers:
          - name: main
            image: {{taskImage}}
            imagePullPolicy: IfNotPresent
  - name: worker1
    role: Env
    placement:
      nodeSelector:
        kubernetes.io/hostname: {{nodeB}}
    runtime:
      kind: Kubernetes
    workload:
      kind: rlinf.io/ray
      version: v1
      parameters:
        role: worker
      carrier:
        type: rlinf.io/kubernetes-statefulset
        version: v1
        parameters:
          replicas: 1
      template:
        spec:
          containers:
          - name: main
            image: {{taskImage}}
            imagePullPolicy: IfNotPresent
  - name: worker2
    role: Env
    placement:
      nodeSelector:
        kubernetes.io/hostname: {{nodeB2}}
    runtime:
      kind: Kubernetes
    workload:
      kind: rlinf.io/ray
      version: v1
      parameters:
        role: worker
      carrier:
        type: rlinf.io/kubernetes-statefulset
        version: v1
        parameters:
          replicas: 1
      template:
        spec:
          containers:
          - name: main
            image: {{taskImage}}
            imagePullPolicy: IfNotPresent
"""


@reg.flow(
    "跨集群 StatefulSet Ray",
    items=[
        ("REG-01", "Ray Active", "head+2 worker，3 Active"),
        ("REG-02", "rank 和放置", "head 与两 worker 落在 config.placement 指定的节点"),
        ("REG-03", "Domain DNS", "worker 能解析 head 的 .rlark-domain 名称"),
        ("REG-04", "跨集群连接", "两 worker 到 head 6379 成功"),
    ],
)
def cross_cluster_ray(t):
    job = f"{t.run_id}-ray"
    head = t.head_pod(job)
    workers = [f"{job}-worker1-0", f"{job}-worker2-0"]

    # -- 流程步骤：只建立/销毁一次集群，观测点复用 --
    t.delete_job(job)
    t.apply(t.render(RAY_JOB), on="mgmt")
    try:
        t.require(lambda: t.kexists("pod", head, on="data", cluster="a"),
                  timeout=300, desc=f"{head} 出现")
        t.wait_pod("a", head, 300)
        for pod in workers:
            t.require(lambda p=pod: t.kexists("pod", p, on="data", cluster="b"),
                      timeout=300, desc=f"{pod} 出现")
            t.wait_pod("b", pod, 300)
        t.sleep(20)  # let Ray form the cluster

        with t.observe("REG-01"):
            t.evidence(t.kexec("a", head, "ray", "status").combined(), "REG-ray-status.txt")
            t.ge(t.ray_active("a", head), 3, "Ray Active")

        with t.observe("REG-02"):
            t.eq(t.pod_node("a", head), t.config.get("placement.a"), "head 节点")
            t.eq([t.pod_node("b", p) for p in workers],
                 [t.config.get("placement.b"), t.config.get("placement.b2")], "worker 节点")

        with t.observe("REG-03"):
            resolved = t.dns_resolve("b", workers[0], f"{head}.rlark-domain")
            t.evidence(f"{head}.rlark-domain -> {resolved}", "REG-dns.txt")
            t.nonempty(resolved, "解析结果")

        with t.observe("REG-04"):
            t.eq([t.tcp_connect("b", p, f"{head}.rlark-domain", 6379) for p in workers],
                 [True, True], "worker→head 连接")
    finally:
        t.delete_job(job)
