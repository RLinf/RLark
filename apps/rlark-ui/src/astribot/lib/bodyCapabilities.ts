export interface BodyCapabilities {
  version: number;
  model: string;
  variants: { id: string; cameras: string[] }[];
  jointGroups: { id: string; dimensions: number; positionUnit?: string; velocityUnit?: string }[];
  endpoints: string[];
}

interface BodySample {
  id: string;
  state: "waiting" | "stale" | "live";
  ageSeconds?: number;
  sourceUnixSeconds?: number;
  frame?: string;
  position?: number[];
  velocity?: number[];
  quaternion?: number[];
}

export interface StationBodyState {
  state: "unsupported" | "unavailable" | "available";
  body?: string;
  capabilities?: BodyCapabilities;
  snapshot?: { joints: BodySample[]; endpoints: BodySample[] };
}

export function partLabel(id: string, zh: boolean): string {
  const names: Record<string, [string, string]> = {
    arm_left: ["左臂", "Left arm"], arm_right: ["右臂", "Right arm"],
    torso: ["躯干", "Torso"], head: ["头部", "Head"],
    gripper_left: ["左夹爪", "Left gripper"], gripper_right: ["右夹爪", "Right gripper"],
    wrist_left: ["左腕", "Left wrist"], wrist_right: ["右腕", "Right wrist"],
  };
  return names[id]?.[zh ? 0 : 1] ?? id;
}
