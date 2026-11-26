import assert from "node:assert/strict";
import test from "node:test";
import {
  formatDurationSince,
  nodeAvailabilityNote,
  nodeAvailabilityReasonText,
  nodeAvailabilityState,
  nodeConditionMessage,
  nodeConditionStatusLabel,
  nodeConnectionState,
  nodeDegradationState,
  nodeKubernetesCordonState,
  nodePressureState,
  nodeReadinessState,
  nodeReasonLabel,
  nodeSchedulingBadgeState,
  nodeSchedulingState,
} from "../dist/test/utils/nodeState.js";

function node({
  unschedulable = false,
  conditions = [],
  operations,
  drainOperations,
} = {}) {
  return {
    metadata: { name: "node-a" },
    spec: { unschedulable, ...(operations ? { operations } : {}) },
    status: { conditions, ...(drainOperations ? { drainOperations } : {}) },
  };
}

// drainOperation 是排空请求在 Node spec 里的投影；Draining Condition 由控制面
// 稍后写入，因此排空状态以该请求为准。
const DRAIN_OPERATION = { id: "drain-1", type: "Drain" };

function condition(type, status, reason) {
  return { type, status, reason };
}

test("derives connection from AgentConnected type, status, and reason", () => {
  assert.equal(
    nodeConnectionState(
      node({
        conditions: [condition("AgentConnected", "True", "LeaseValid")],
      }),
    ).kind,
    "connected",
  );
  assert.equal(
    nodeConnectionState(
      node({
        conditions: [condition("AgentConnected", "False", "LeaseExpired")],
      }),
    ).kind,
    "disconnected",
  );
  assert.equal(
    nodeConnectionState(
      node({
        conditions: [
          condition("AgentConnected", "False", "HeartbeatNeverReceived"),
        ],
      }),
    ).kind,
    "neverConnected",
  );
  assert.equal(nodeConnectionState(node()).kind, "connectionUnknown");
});

test("keeps scheduling independent from connection and readiness", () => {
  const baseConditions = [
    condition("AgentConnected", "False", "LeaseExpired"),
    condition("Ready", "False", "LeaseExpired"),
  ];
  assert.equal(
    nodeSchedulingState(node({ conditions: baseConditions })).kind,
    "schedulable",
  );
  assert.equal(
    nodeSchedulingState(
      node({ unschedulable: true, conditions: baseConditions }),
    ).kind,
    "cordoned",
  );
  assert.equal(
    nodeSchedulingState(
      node({
        unschedulable: true,
        operations: [DRAIN_OPERATION],
        conditions: [
          ...baseConditions,
          condition("Draining", "True", "DrainRequested"),
        ],
      }),
    ).kind,
    "draining",
  );
});

test("drops a lingering Draining condition once the drain request is gone", () => {
  // Undrain removes the Drain operation from spec synchronously, but the Node
  // Controller rewrites the Draining condition only on its next pass. Reading the
  // request (not the stale condition) keeps the console from briefly still
  // showing "cancel drain" after the operator cancels it.
  assert.equal(
    nodeSchedulingState(
      node({
        unschedulable: true,
        conditions: [condition("Draining", "True", "DrainRequested")],
      }),
    ).kind,
    "cordoned",
  );
});

test("treats an omitted unschedulable field as schedulable", () => {
  const state = nodeSchedulingState({
    metadata: { name: "node-b" },
    spec: {},
    status: {},
  });
  assert.equal(state.kind, "schedulable");
});

test("reads a just-requested drain before the Draining condition lands", () => {
  // The Node Controller writes the Draining condition on a later pass, so the
  // console must derive "draining" from the Drain request itself. Otherwise the
  // drain toggle briefly reverts to the pre-drain state until the next refresh.
  const requested = nodeSchedulingState({
    metadata: { name: "node-c" },
    spec: {
      unschedulable: true,
      operations: [{ id: "drain-1", type: "Drain" }],
    },
    status: { conditions: [condition("Draining", "False", "NoDrainRequest")] },
  });
  assert.equal(requested.kind, "draining");

  // A re-drain requested after a completed drain gets a new operation; the old
  // completion must not hide the new active request.
  const redrain = nodeSchedulingState({
    metadata: { name: "node-c" },
    spec: {
      unschedulable: true,
      operations: [
        { id: "drain-1", type: "Drain" },
        { id: "drain-2", type: "Drain" },
      ],
    },
    status: {
      conditions: [condition("Draining", "True", "DrainRequested")],
      drainOperations: [{ operationID: "drain-1", completed: true }],
    },
  });
  assert.equal(redrain.kind, "draining");
});

test("reports a completed drain as cordoned, not draining", () => {
  // A completed Drain operation stays in spec.operations, but the Node
  // Controller falls back to Cordoned, so the console must agree.
  const completed = nodeSchedulingState({
    metadata: { name: "node-d" },
    spec: {
      unschedulable: true,
      operations: [{ id: "drain-1", type: "Drain" }],
    },
    status: {
      conditions: [condition("Draining", "False", "DrainComplete")],
      drainOperations: [{ operationID: "drain-1", completed: true }],
    },
  });
  assert.equal(completed.kind, "cordoned");
});

test("does not read a maintenance operation as draining", () => {
  const maintained = nodeSchedulingState({
    metadata: { name: "node-e" },
    spec: {
      unschedulable: true,
      operations: [{ id: "maint-1", type: "EnterMaintenance" }],
    },
    status: {},
  });
  assert.equal(maintained.kind, "cordoned");
});

test("derives readiness, pressure, and degradation without inspecting messages", () => {
  const state = node({
    conditions: [
      {
        ...condition("Ready", "False", "KubernetesNodeNotReady"),
        message: "This arbitrary text must not control UI classification.",
      },
      condition("ResourcePressure", "True", "ResourcePressureDetected"),
      condition("Degraded", "True", "StorageUnavailable"),
    ],
  });
  assert.equal(nodeReadinessState(state).kind, "notReady");
  assert.equal(nodePressureState(state).kind, "pressure");
  assert.equal(nodeDegradationState(state).kind, "degraded");
});

test("keeps pressure unknown but treats readiness and degradation as two-state", () => {
  const state = node({
    conditions: [
      condition("Ready", "Unknown", "KubernetesNodeUnknown"),
      condition("ResourcePressure", "Unknown", "ResourcePressureUnknown"),
      condition("Degraded", "Unknown", "ResourcePressureUnknown"),
    ],
  });
  // The Node Controller only writes Ready and Degraded as True or False.
  assert.equal(nodeReadinessState(state).kind, "notReady");
  assert.equal(nodePressureState(state).kind, "pressureUnknown");
  assert.equal(nodeDegradationState(state).kind, "healthy");
});

test("reads placement availability from the Schedulable condition", () => {
  const available = nodeAvailabilityState(
    node({ conditions: [condition("Schedulable", "True", "Schedulable")] }),
  );
  assert.equal(available.kind, "available");
  assert.deepEqual(available.reasons, []);

  const cordoned = nodeAvailabilityState(
    node({
      unschedulable: true,
      conditions: [condition("Schedulable", "False", "Unschedulable")],
    }),
  );
  assert.equal(cordoned.kind, "unavailable");
  assert.deepEqual(
    cordoned.reasons.map((reason) => reason.kind),
    ["notSchedulable"],
  );
  assert.equal(
    nodeAvailabilityReasonText(cordoned, false),
    "Not schedulable: Cordoned",
  );
  assert.equal(
    nodeAvailabilityReasonText(cordoned, true),
    "不可调度：已暂停调度",
  );

  const missingCondition = nodeAvailabilityState(node());
  assert.equal(missingCondition.kind, "unavailable");
  assert.equal(
    nodeAvailabilityReasonText(missingCondition, false),
    "Not schedulable",
  );
});

test("shares the node list notes text between the list and detail header", () => {
  const draining = node({
    unschedulable: true,
    operations: [DRAIN_OPERATION],
    conditions: [
      condition("Draining", "True", "DrainRequested"),
      condition("Schedulable", "False", "DrainRequested"),
    ],
  });
  assert.equal(nodeAvailabilityNote(draining, true), "不可调度/驱逐任务中");
  assert.equal(
    nodeAvailabilityNote(draining, false),
    "Not schedulable / draining jobs",
  );

  const cordoned = node({
    unschedulable: true,
    conditions: [condition("Schedulable", "False", "Unschedulable")],
  });
  assert.equal(nodeAvailabilityNote(cordoned, true), "不可调度：已暂停调度");
  assert.equal(
    nodeAvailabilityNote(cordoned, false),
    "Not schedulable: Cordoned",
  );

  const available = node({
    conditions: [condition("Schedulable", "True", "Schedulable")],
  });
  assert.equal(nodeAvailabilityNote(available, true), "—");
});

test("treats an external Kubernetes cordon as unavailable placement", () => {
  const cordoned = node({
    conditions: [
      condition("Schedulable", "False", "KubernetesCordoned"),
      condition("KubernetesCordoned", "True", "KubernetesCordoned"),
    ],
  });
  assert.equal(nodeKubernetesCordonState(cordoned).kind, "kubernetesCordoned");
  // The controller folds the cordon into Schedulable, so the Node is unavailable.
  assert.equal(nodeAvailabilityState(cordoned).kind, "unavailable");
  assert.equal(
    nodeAvailabilityState(cordoned).reasons[0].kind,
    "notSchedulable",
  );

  assert.equal(
    nodeKubernetesCordonState(
      node({
        conditions: [
          condition("KubernetesCordoned", "False", "KubernetesNotCordoned"),
        ],
      }),
    ).kind,
    "kubernetesNotCordoned",
  );
  assert.equal(
    nodeKubernetesCordonState(
      node({
        conditions: [
          condition(
            "KubernetesCordoned",
            "Unknown",
            "KubernetesCordonUnobserved",
          ),
        ],
      }),
    ).kind,
    "kubernetesCordonUnknown",
  );
  // An unreported observation is unknown, not "not cordoned".
  assert.equal(
    nodeKubernetesCordonState(node()).kind,
    "kubernetesCordonUnknown",
  );
});

test("uses Schedulable as the single source instead of re-deriving conditions", () => {
  // Contradictory underlying conditions must not change placement: the console
  // and the scheduler both consume the controller's Schedulable conclusion.
  const schedulableDespiteConditions = nodeAvailabilityState(
    node({
      conditions: [
        condition("AgentConnected", "False", "LeaseExpired"),
        condition("Ready", "False", "KubernetesNodeNotReady"),
        condition("ResourcePressure", "True", "ResourcePressureDetected"),
        condition("Schedulable", "True", "Schedulable"),
      ],
    }),
  );
  assert.equal(schedulableDespiteConditions.kind, "available");

  const healthyButNotSchedulable = nodeAvailabilityState(
    node({
      conditions: [
        condition("AgentConnected", "True", "LeaseValid"),
        condition("Ready", "True", "KubernetesNodeReady"),
        condition("ResourcePressure", "False", "NoResourcePressure"),
        condition("Schedulable", "False", "Unschedulable"),
      ],
    }),
  );
  assert.equal(healthyButNotSchedulable.kind, "unavailable");
});

test("keeps service degradation as a display-only distinction", () => {
  const degraded = nodeAvailabilityState(
    node({
      conditions: [
        condition("Schedulable", "True", "Schedulable"),
        condition("Degraded", "True", "StorageUnavailable"),
      ],
    }),
  );
  assert.equal(degraded.kind, "serviceDegraded");
  assert.deepEqual(
    degraded.reasons.map((reason) => reason.kind),
    ["degraded"],
  );
  assert.equal(
    nodeAvailabilityReasonText(degraded, false),
    "Degraded: Storage unavailable",
  );
  assert.equal(
    nodeAvailabilityReasonText(degraded, true),
    "已降级：存储不可用",
  );
});

test("surfaces the Schedulable reason for unavailable nodes", () => {
  const offline = nodeAvailabilityState(
    node({
      conditions: [
        condition("AgentConnected", "False", "LeaseExpired"),
        condition("Ready", "False", "LeaseExpired"),
        condition("Schedulable", "False", "LeaseExpired"),
      ],
    }),
  );
  assert.equal(offline.kind, "unavailable");
  assert.deepEqual(
    offline.reasons.map((reason) => reason.kind),
    ["notSchedulable"],
  );
  assert.equal(
    nodeAvailabilityReasonText(offline, false),
    "Not schedulable: Heartbeat lease expired",
  );
  assert.equal(
    nodeAvailabilityReasonText(offline, true),
    "不可调度：心跳租约已过期",
  );
});

test("derives the detail scheduling badge from effective placement", () => {
  // An offline Node keeps spec.unschedulable false; the badge must follow the
  // controller's Schedulable decision rather than the administrative intent.
  const offline = node({
    conditions: [
      condition("AgentConnected", "False", "LeaseExpired"),
      condition("Ready", "False", "LeaseExpired"),
      condition("Schedulable", "False", "LeaseExpired"),
    ],
  });
  assert.equal(nodeSchedulingBadgeState(offline), "notSchedulable");

  const cordoned = node({
    unschedulable: true,
    conditions: [condition("Schedulable", "False", "Unschedulable")],
  });
  assert.equal(nodeSchedulingBadgeState(cordoned), "notSchedulable");

  const available = node({
    conditions: [condition("Schedulable", "True", "Schedulable")],
  });
  assert.equal(nodeSchedulingBadgeState(available), "schedulable");

  // A degraded Node remains eligible for placement, so the badge stays green.
  const degraded = node({
    conditions: [
      condition("Schedulable", "True", "Schedulable"),
      condition("Degraded", "True", "StorageUnavailable"),
    ],
  });
  assert.equal(nodeSchedulingBadgeState(degraded), "schedulable");

  const draining = node({
    unschedulable: true,
    operations: [DRAIN_OPERATION],
    conditions: [
      condition("Draining", "True", "DrainRequested"),
      condition("Schedulable", "False", "DrainRequested"),
    ],
  });
  assert.equal(nodeSchedulingBadgeState(draining), "draining");

  // A missing Schedulable condition is fail-closed, matching the scheduler.
  assert.equal(nodeSchedulingBadgeState(node()), "notSchedulable");
});

test("formats heartbeat and disconnected durations deterministically", () => {
  const now = Date.parse("2026-10-27T10:00:00Z");
  assert.equal(formatDurationSince("2026-10-27T09:59:42Z", now), "18s");
  assert.equal(formatDurationSince("2026-10-27T08:30:00Z", now), "1h");
  assert.equal(formatDurationSince("2026-10-25T10:00:00Z", now), "2d");
  assert.equal(formatDurationSince("not-a-time", now), undefined);
  assert.equal(formatDurationSince("2026-10-27T11:00:00Z", now), undefined);
});

test("localizes condition status without inferring health", () => {
  assert.equal(nodeConditionStatusLabel("True", true), "是");
  assert.equal(nodeConditionStatusLabel("False", true), "否");
  assert.equal(nodeConditionStatusLabel("Unknown", true), "未知");
  assert.equal(nodeConditionStatusLabel("True", false), "True");
  assert.equal(nodeConditionStatusLabel("False", false), "False");
  assert.equal(nodeConditionStatusLabel("Unknown", false), "Unknown");
});

test("localizes every reason the Node Controller writes", () => {
  // Available reasons were missing from the label map, so Chinese consoles
  // showed raw English machine codes for healthy capabilities.
  assert.equal(nodeReasonLabel("RuntimeAvailable", true), "运行时可用");
  assert.equal(nodeReasonLabel("NetworkAvailable", true), "网络可用");
  assert.equal(nodeReasonLabel("StorageAvailable", true), "存储可用");
  assert.equal(nodeReasonLabel("DeviceAvailable", true), "设备可用");
  assert.equal(nodeReasonLabel("NoResourcePressure", true), "无资源压力");
  assert.equal(nodeReasonLabel("RuntimeAvailable", false), "Runtime available");
  assert.equal(
    nodeReasonLabel("NoResourcePressure", false),
    "No resource pressure",
  );
  // Unknown reasons stay machine-readable instead of pretending to translate.
  assert.equal(nodeReasonLabel("SomeNewReason", true), "SomeNewReason");
});

test("localizes the fixed condition messages and falls back for unknown types", () => {
  const english =
    "Agent connectivity is derived from the node heartbeat Lease.";
  assert.equal(
    nodeConditionMessage("AgentConnected", english, true),
    "Agent 连接状态来自节点心跳租约。",
  );
  assert.equal(nodeConditionMessage("AgentConnected", english, false), english);
  assert.equal(nodeConditionMessage("FutureCondition", english, true), english);
  assert.equal(
    nodeConditionMessage("AgentConnected", undefined, true),
    undefined,
  );
});
