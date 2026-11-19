"""TASK: single-task lifecycle as one 测试流程 with three sequential 测试观测点.

Shows a procedure that mutates the environment between observation points
(create -> stop/start -> delete) while keeping one Job for the whole flow.
"""

from __future__ import annotations

from kindtest import group

task = group("TASK", title="生命周期和错误恢复")

SINGLE_TASK_JOB = """
apiVersion: rlinf.io/v1alpha1
kind: Job
metadata:
  name: {{runId}}-single
  labels:
    {{labelKey}}: {{runId}}-single
spec:
  domain: {{domain}}
  tasks:
  - name: main
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
"""


@task.flow(
    "单任务生命周期",
    items=[
        ("TASK-01", "新建并 Running", "Task phase=Running，Pod Ready"),
        ("TASK-02", "stop/start", "stop 删除子 Task/Pod，start 按同名 Job 重建并 Running"),
        ("TASK-03", "删除完整性", "删除 Job 后 Task 与 Pod 均回收"),
    ],
)
def single_task_lifecycle(t):
    job = f"{t.run_id}-single"
    task_name = t.task_name(job)
    pod = f"{job}-main-0"
    ns = t.mgmt_ns("a")

    t.delete_job(job)
    t.apply(t.render(SINGLE_TASK_JOB), on="mgmt")
    try:
        t.require(lambda: t.kexists("task", task_name, on="mgmt", ns=ns),
                  timeout=180, desc="Task 出现")
        t.require(lambda: t.kexists("pod", pod, on="data", cluster="a"),
                  timeout=300, desc="Pod 出现")
        t.wait_pod("a", pod, 300)

        with t.observe("TASK-01"):
            t.eq(t.task_phase("a", task_name), "Running", "Task phase")
            t.eq(t.pod_ready("a", pod), True, "Pod Ready")

        with t.observe("TASK-02"):
            uid_before = t.task_uid("a", task_name)
            t.job_stopped(job, True)
            t.require(lambda: not t.kexists("pod", pod, on="data", cluster="a"),
                      timeout=200, desc="Pod 消失")
            # 文档契约（concepts.md「Job 停止/启动」）：停止会删除子 Task CR，
            # 启动时按同名 Job 重建新的 Task 与 Worker，因此 Task UID 会变化。
            t.require(lambda: not t.kexists("task", task_name, on="mgmt", ns=ns),
                      timeout=200, desc="Task 回收")
            t.job_stopped(job, False)
            t.require(lambda: t.kexists("task", task_name, on="mgmt", ns=ns),
                      timeout=300, desc="Task 重建")
            t.require(lambda: t.kexists("pod", pod, on="data", cluster="a"),
                      timeout=300, desc="Pod 恢复")
            t.wait_pod("a", pod, 300)
            t.truthy(t.task_uid("a", task_name) != uid_before,
                     "Task 按文档重建（UID 变化）")

        with t.observe("TASK-03"):
            t.delete_job(job)
            t.require(lambda: not t.kexists("task", task_name, on="mgmt", ns=ns),
                      timeout=200, desc="Task 回收")
            t.require(lambda: not t.kexists("pod", pod, on="data", cluster="a"),
                      timeout=200, desc="Pod 回收")
            t.truthy(True, "Job/Task/Pod 均已回收")
    finally:
        t.delete_job(job)
