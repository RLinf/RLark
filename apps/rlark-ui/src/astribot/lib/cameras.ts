// A robot's cameras as its model Addon reports them (astribot/s1u/device/cameras.py): the Gateway's
// GET .../nodes/{name}/cameras, live, and the same JSON as the Node's annotation rlark.io/cameras, written when a
// camera's state changes.
import { withQuery } from "../../api.js";

// The views a robot may have, in the order of its camera wall: head, torso / wrist_left, wrist_right.
export const CAMERA_VIEWS = ["head", "torso", "wrist_left", "wrist_right"] as const;
export type CameraView = (typeof CAMERA_VIEWS)[number];

export interface CameraReport {
  // slow: under 10 frames a second for 30 s; stale: no frame for 5 s; absent: none since the service started.
  state: "ok" | "slow" | "stale" | "absent";
  fps: number;
  age_seconds: number | null;
  // Where its recovery is: 0 none, 1 driver restarts, 2 USB reset, 3 stuck.
  step: number;
  attempts: number;
  next_attempt_at: number | null;
  last_action: string | null;
  last_result: string | null;
}

export interface CamerasReport {
  state: "ok" | "recovering" | "degraded" | "stuck";
  // The views of the robot's body only.
  cameras: Partial<Record<CameraView, CameraReport>>;
  updated_at: number;
}

// The console's status class it shows as (status-<tone>).
export type CameraTone = "online" | "pending" | "failed" | "stopped";

export interface CameraCell {
  view: CameraView;
  name: string;
  // Whether the robot's body has this camera; unknown (undefined) before the robot's first report.
  present?: boolean;
  badge: string;
  tone: CameraTone;
  age: string;
  // A frame can be asked for: the camera has sent one.
  streams: boolean;
}

const viewNames: Record<CameraView, [string, string]> = {
  head: ["头部", "Head"],
  torso: ["躯干", "Torso"],
  wrist_left: ["左腕", "Left wrist"],
  wrist_right: ["右腕", "Right wrist"],
};

export function cameraBadge(camera: CameraReport, zh: boolean): { badge: string; tone: CameraTone } {
  if (camera.step === 3) return { badge: zh ? "需要断电或 USB 复位" : "Needs a power cycle or USB reset", tone: "failed" };
  if (camera.state === "stale" || camera.state === "absent") {
    if (camera.step === 1 || camera.step === 2) {
      return { badge: zh ? `恢复中 第 ${camera.step} 步` : `Recovering, step ${camera.step}`, tone: "pending" };
    }
    return { badge: zh ? "无画面" : "No frames", tone: "failed" };
  }
  if (camera.state === "slow") return { badge: zh ? "帧率低" : "Low frame rate", tone: "pending" };
  return { badge: zh ? "正常" : "OK", tone: "online" };
}

export function cameraAge(seconds: number | null | undefined, zh: boolean): string {
  if (seconds == null) return "—";
  const shown = seconds < 10 ? seconds.toFixed(1) : String(Math.round(seconds));
  return zh ? `${shown} 秒前` : `${shown} s ago`;
}

// The wall's four cells, in its order, as the report says them.
export function cameraCells(report: CamerasReport | undefined, zh: boolean): CameraCell[] {
  return CAMERA_VIEWS.map((view) => {
    const name = viewNames[view][zh ? 0 : 1];
    if (!report) return { view, name, badge: zh ? "等待相机状态" : "Waiting for the cameras", tone: "stopped", age: "—", streams: false };
    const camera = report.cameras[view];
    if (!camera) return { view, name, present: false, badge: zh ? "该本体没有这路相机" : "This body has no such camera", tone: "stopped", age: "—", streams: false };
    return { view, name, present: true, ...cameraBadge(camera, zh), age: cameraAge(camera.age_seconds, zh), streams: camera.state !== "absent" };
  });
}

export function camerasPath(node: string, namespace: string): string {
  return withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/cameras`, { namespace });
}

// A view's stream (multipart JPEG, an <img> source; the session cookie authenticates it).
export function cameraStreamPath(node: string, namespace: string, view: CameraView | "wall", fps = 10): string {
  return withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/cameras/${view}/stream`, { namespace, fps });
}
