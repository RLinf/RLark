// A robot's cameras as its model Addon reports them (astribot/s1u/device/cameras.py): the Node's annotation
// rlark.io/cameras (the site-node view's `cameras`), written when a camera's state changes, and the same JSON live
// (rates and ages) from the Gateway's GET .../nodes/{name}/cameras.
import { withQuery } from "../../api.js";

// The views a robot may have, in the order of its camera wall: head, torso / wrist_left, wrist_right.
export const CAMERA_VIEWS = ["head", "torso", "wrist_left", "wrist_right"] as const;
export type CameraView = (typeof CAMERA_VIEWS)[number];

export interface CameraReport {
  // slow: under 10 frames a second for 30 s; stale: no frame for 5 s; absent: none since the service started.
  state: "ok" | "slow" | "stale" | "absent";
  fps: number;
  age_seconds: number | null;
  // Where its recovery is: 0 none, 1 driver restarts, 2 USB reset, 3 stuck (out of the ladder until it has sent for
  // 5 minutes again or moved to another USB port).
  step: number;
  attempts: number;
  next_attempt_at: number | null;
  last_action: string | null;
  // Stuck: why (相机需要断电或 USB 复位, 疑似硬件 when a USB reset did not hold, or the Orin's USB lacks it).
  last_result: string | null;
  // Stuck: what the Orin showed of it (on the USB or not and where, the vendor's camera process, the Orbbec SDK's streams).
  diagnosis: string | null;
  // Where it is on the Orin's USB, null before the Orin is read or while it is not on the bus.
  usb: CameraUsb | null;
  // Advice when its hub holds fewer cameras than another hub on its USB root port; no state of its own.
  usb_advice: string | null;
}

export interface CameraUsb {
  path: string;
  device: number;
  speed: number;
  // vendor:product of each hub from the root port down.
  hubs: string[];
}

export interface CamerasReport {
  state: "ok" | "recovering" | "degraded" | "stuck";
  // The views of the robot's body only.
  cameras: Partial<Record<CameraView, CameraReport>>;
  updated_at: number;
  // Only the live report includes the camera service's clock exchange.
  clock?: { received_ms: number; sent_ms: number };
}

// The robot's last published report, from its Node's annotation; undefined when there is none.
export function publishedCameras(annotation: string | undefined): CamerasReport | undefined {
  return annotation ? JSON.parse(annotation) as CamerasReport : undefined;
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
  frames: boolean;
  // A stuck camera's reason and diagnosis.
  note?: string;
  advice?: string;
}

const viewNames: Record<CameraView, [string, string]> = {
  head: ["头部", "Head"],
  torso: ["躯干", "Torso"],
  wrist_left: ["左腕", "Left wrist"],
  wrist_right: ["右腕", "Right wrist"],
};

// A stuck camera that sends again is as its frames say: it leaves stuck once they have held for 5 minutes.
export function cameraBadge(camera: CameraReport, zh: boolean): { badge: string; tone: CameraTone } {
  if (camera.state === "stale" || camera.state === "absent") {
    if (camera.step === 3) return { badge: zh ? "需要断电或 USB 复位" : "Needs a power cycle or USB reset", tone: "failed" };
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

// The wall's four cells, in its order, as the live report says them. While the report cannot be read (live false),
// each view of the last published one says so, with no state of its own.
export function cameraCells(report: CamerasReport | undefined, zh: boolean, live = true): CameraCell[] {
  return CAMERA_VIEWS.map((view) => {
    const name = viewNames[view][zh ? 0 : 1];
    const unread = { badge: zh ? "读不到实时状态" : "Live state unavailable", tone: "stopped" as const, age: "—", frames: false };
    if (!report) return live ? { view, name, badge: zh ? "等待相机状态" : "Waiting for the cameras", tone: "stopped", age: "—", frames: false } : { view, name, ...unread };
    const camera = report.cameras[view];
    if (!camera) return { view, name, present: false, badge: zh ? "该本体没有这路相机" : "This body has no such camera", tone: "stopped", age: "—", frames: false };
    if (!live) return { view, name, present: true, ...unread };
    const stuck = camera.step === 3 && (camera.state === "stale" || camera.state === "absent");
    return {
      view, name, present: true, ...cameraBadge(camera, zh), age: cameraAge(camera.age_seconds, zh), frames: camera.state !== "absent",
      note: stuck ? [camera.last_result, camera.diagnosis].filter(Boolean).join("；") : undefined,
      advice: camera.usb_advice ?? undefined,
    };
  });
}

export function camerasPath(node: string, namespace: string): string {
  return withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/cameras`, { namespace });
}

// A view's newest JPEG (its age in the header X-Age-Seconds; 404 the body has no such view, 503 no frame yet).
export function cameraFramePath(node: string, namespace: string, view: CameraView): string {
  return withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/cameras/${view}.jpg`, { namespace });
}

// The preview rates a camera can be streamed at, slowest first. A robot wall with several cards starts every
// card at the lowest rate so that it does not ask the camera service for high frame rates by default.
export const CAMERA_FPS_OPTIONS = [5, 10, 15, 30] as const;
export type CameraFps = (typeof CAMERA_FPS_OPTIONS)[number];
export const DEFAULT_CAMERA_FPS: CameraFps = CAMERA_FPS_OPTIONS[0];

export function isCameraFps(value: number): value is CameraFps {
  return (CAMERA_FPS_OPTIONS as readonly number[]).includes(value);
}

export function cameraStreamPath(node: string, namespace: string, view: CameraView, fps: number): string {
  return withQuery(`/api/v1/rlinf.io/v1alpha1/nodes/${encodeURIComponent(node)}/cameras/${view}/stream`, { namespace, fps });
}

export function readCameraAge(header: string | null): number | undefined {
  if (!header?.trim()) return undefined;
  const seconds = Number(header);
  return Number.isFinite(seconds) && seconds >= 0 ? seconds : undefined;
}

export function formatCameraMilliseconds(ms: number | undefined): string {
  if (ms === undefined) return "—";
  return ms < 1000 ? `${Math.round(ms)} ms` : `${(ms / 1000).toFixed(1)} s`;
}
