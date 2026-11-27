export interface RobotBodyJoint {
  name: string;
  type: "revolute" | "continuous" | "prismatic" | "fixed";
  units: "rad" | "m" | "unitless";
  limits?: { lower: number; upper: number };
}

export interface RobotBodyEndEffector {
  name: string;
  kind: "gripper" | "dexterousHand" | "passiveTool";
  dof: number;
  placement?: string;
}

export interface RobotBodySensor {
  name: string;
  kind: "camera" | "imu";
  count: number;
  placement: string;
}

export interface RobotBodyDefinition {
  joints: RobotBodyJoint[];
  endEffectors: RobotBodyEndEffector[];
  sensors: RobotBodySensor[];
  physicalSpecs?: {
    payloadKg?: number;
    runtimeMinutes?: number;
    dimensionsMm?: { length: number; width: number; height: number };
    battery?: { capacityWh?: number; nominalVoltageV?: number };
  };
}

export interface RobotBodyInventory {
  state: "complete" | "partial" | "unavailable";
  source?: "driverInventory" | "runtimeDescription";
  sampleAgeSeconds?: number;
  joints: Array<{ name: string; type?: string }>;
  endEffectors: Array<{ name: string; kind?: string; dof?: number }>;
}

export interface RobotModelMock {
  name: string;
  displayName: string;
  vendor: string;
  description: string;
  body?: RobotBodyDefinition;
}

export interface RobotBodyDifference {
  section: "joints" | "endEffectors";
  name: string;
  kind: "missing" | "extra" | "conflict" | "evidence";
  expected?: string;
  observed?: string;
}

export interface RobotBodyComparison {
  status: "matched" | "mismatch" | "unknown";
  reason: string;
  differences: RobotBodyDifference[];
  observed?: RobotBodyInventory;
}

export const robotModelMocks: RobotModelMock[] = [
  {
    name: "s1u",
    displayName: "S1U 双臂协作机器人",
    vendor: "Infinigence Robotics",
    description: "双臂移动操作平台，含头部与腕部视觉传感器。",
    body: {
      joints: [
        { name: "left_shoulder_yaw", type: "revolute", units: "rad", limits: { lower: -2.6, upper: 2.6 } },
        { name: "left_shoulder_pitch", type: "revolute", units: "rad", limits: { lower: -1.8, upper: 1.8 } },
        { name: "left_elbow", type: "revolute", units: "rad", limits: { lower: -2.2, upper: 0.2 } },
        { name: "right_shoulder_yaw", type: "revolute", units: "rad", limits: { lower: -2.6, upper: 2.6 } },
        { name: "right_shoulder_pitch", type: "revolute", units: "rad", limits: { lower: -1.8, upper: 1.8 } },
        { name: "right_elbow", type: "revolute", units: "rad", limits: { lower: -2.2, upper: 0.2 } },
      ],
      endEffectors: [
        { name: "left_tool", kind: "gripper", dof: 1, placement: "left_wrist" },
        { name: "right_tool", kind: "gripper", dof: 1, placement: "right_wrist" },
      ],
      sensors: [
        { name: "head_rgb", kind: "camera", count: 1, placement: "head" },
        { name: "wrist_rgb", kind: "camera", count: 2, placement: "wrists" },
        { name: "base_imu", kind: "imu", count: 1, placement: "base" },
      ],
      physicalSpecs: {
        payloadKg: 3,
        runtimeMinutes: 90,
        dimensionsMm: { length: 680, width: 520, height: 1280 },
        battery: { capacityWh: 480, nominalVoltageV: 48 },
      },
    },
  },
  {
    name: "s1u-legacy",
    displayName: "S1U（历史目录项）",
    vendor: "Infinigence Robotics",
    description: "历史型号条目尚未发布本体定义。",
  },
];

const matchedInventory: RobotBodyInventory = {
  state: "complete",
  source: "driverInventory",
  sampleAgeSeconds: 18,
  joints: robotModelMocks[0].body!.joints.map(({ name, type }) => ({ name, type })),
  endEffectors: robotModelMocks[0].body!.endEffectors.map(({ name, kind, dof }) => ({ name, kind, dof })),
};

const mismatchInventory: RobotBodyInventory = {
  ...matchedInventory,
  endEffectors: matchedInventory.endEffectors.map((item) =>
    item.name === "right_tool" ? { ...item, kind: "dexterousHand", dof: 6 } : item),
};

const partialInventory: RobotBodyInventory = {
  state: "partial",
  source: "runtimeDescription",
  sampleAgeSeconds: 74,
  joints: matchedInventory.joints,
  endEffectors: [],
};

export const robotBodyMocks: Record<string, RobotBodyComparison> = {
  "s1u-01-x86": { status: "matched", reason: "Matched", differences: [], observed: matchedInventory },
  "s1u-02-x86": {
    status: "mismatch",
    reason: "Mismatch",
    observed: mismatchInventory,
    differences: [{ section: "endEffectors", name: "right_tool", kind: "conflict", expected: "gripper · 1 DOF", observed: "dexterousHand · 6 DOF" }],
  },
  "s1u-03-x86": {
    status: "unknown",
    reason: "InsufficientEvidence",
    observed: partialInventory,
    differences: [{ section: "endEffectors", name: "right_tool", kind: "evidence", expected: "gripper · 1 DOF" }],
  },
};

export function compareRobotBody(model: RobotModelMock | undefined, nodeName: string): RobotBodyComparison {
  const comparison = robotBodyMocks[nodeName];
  if (!model?.body) return { status: "unknown", reason: "ModelBodyUndefined", differences: [], observed: comparison?.observed };
  if (!comparison || comparison.reason === "InsufficientEvidence") {
    return {
      status: "unknown",
      reason: comparison?.reason ?? "InventoryNotReported",
      observed: comparison?.observed,
      differences: comparison?.differences ?? [],
    };
  }
  const differences: RobotBodyDifference[] = [];
  const compare = (
    section: RobotBodyDifference["section"],
    expected: Array<{ name: string; key: string; value: string }>,
    actual: Array<{ name: string; key?: string; value?: string }>,
  ) => {
    const actualByName = new Map(actual.map((item) => [item.name, item]));
    const expectedNames = new Set(expected.map((item) => item.name));
    for (const item of expected) {
      const found = actualByName.get(item.name);
      if (!found) differences.push({ section, name: item.name, kind: "missing", expected: item.value });
      else if (found.key && found.key !== item.key) differences.push({ section, name: item.name, kind: "conflict", expected: item.value, observed: found.value });
    }
    for (const item of actual) {
      if (!expectedNames.has(item.name)) differences.push({ section, name: item.name, kind: "extra", observed: item.value });
    }
  };
  const joints = model.body.joints.map((item) => ({ name: item.name, key: item.type, value: item.type + " · " + item.units }));
  const observedJoints = comparison.observed!.joints.map((item) => ({ name: item.name, key: item.type, value: item.type }));
  const endEffectors = model.body.endEffectors.map((item) => ({ name: item.name, key: item.kind + ":" + item.dof, value: item.kind + " · " + item.dof + " DOF" }));
  const observedEndEffectors = comparison.observed!.endEffectors.map((item) => ({
    name: item.name,
    key: item.kind && item.dof !== undefined ? item.kind + ":" + item.dof : undefined,
    value: item.kind && item.dof !== undefined ? item.kind + " · " + item.dof + " DOF" : item.kind,
  }));
  compare("joints", joints, observedJoints);
  compare("endEffectors", endEffectors, observedEndEffectors);
  return { ...comparison, status: differences.length ? "mismatch" : "matched", differences };
}

export function robotModelMock(name: string | undefined) {
  return name ? robotModelMocks.find((item) => item.name === name) : undefined;
}
