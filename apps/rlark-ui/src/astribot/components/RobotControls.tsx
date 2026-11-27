import { useEffect, useRef, useState } from "react";
import { House, Power } from "lucide-react";
import { ApiError } from "../../api";
import { stationsApi, type RobotPoseRefusal } from "../api";
import type { CRDNode } from "../../types";
import { hasNodeCategory } from "../../utils/nodes";
import { controlledRobot, robotPoseRefusalLabel, robotPowerState, type ControlledRobot, type RobotPowerDesired } from "../lib/siteNodeDisplay";
import { useRobotPower, type RobotPower } from "./useRobotPower";
import "./robotControls.css";

const HOLD_PERIOD_MS = 200;
const HOLD_TIMEOUT_MS = 1500;
const STOP_TIMEOUT_MS = 6000;
// One bounded wait for an in-flight hold before the stop is sent; past it the stop goes out anyway and the
// outcome is reported as unknown rather than pretending the pose settled.
const LAST_HOLD_WAIT_MS = 3000;

export type PoseReleaseState = "released" | "release-failed" | "release-unknown";

interface PoseSession {
  keys: Set<string>;
  state: "holding" | "releasing";
  release: () => void;
  done: Promise<PoseReleaseState>;
}

// Cross-component release sessions: a hold started on a tile, in the detail dialog or from the bulk bar shares
// this registry, so closing a dialog or pressing a power button releases the pose exactly once, and a new hold
// for the same robots waits until the previous stop has settled (or timed out) instead of racing it.
const sessions = new Map<string, PoseSession>();

const sessionKeys = (robots: ControlledRobot[]) => robots.map((robot) => `${robot.namespace}/${robot.node}`).sort();

const shares = (a: Set<string>, b: Set<string>) => [...a].some((key) => b.has(key));

// Releases any live session that involves these robots, bounded, and reports the outcome. Safe to call when
// nothing is held. The power controls call this before their confirmation so the machine is not moving while
// the browser blocks the event loop on window.confirm.
export async function releasePoseSessions(robots: ControlledRobot[]): Promise<PoseReleaseState> {
  const keys = new Set(sessionKeys(robots));
  const live = [...sessions.values()].filter((session) => shares(session.keys, keys));
  if (!live.length) return "released";
  const outcomes = await Promise.all(live.map((session) => {
    session.release();
    return session.done;
  }));
  return outcomes.includes("release-failed") ? "release-failed" : outcomes.includes("release-unknown") ? "release-unknown" : "released";
}

export function poseSessionBusy(robots: ControlledRobot[]): boolean {
  const keys = new Set(sessionKeys(robots));
  return [...sessions.values()].some((session) => shares(session.keys, keys));
}

const poseFailure = (robot: ControlledRobot, reason: unknown, zh: boolean) => {
  const refusal = reason instanceof ApiError && reason.status === 409 ? (reason.body as RobotPoseRefusal | undefined) : undefined;
  return `${robot.node}: ${refusal?.reason ? robotPoseRefusalLabel(refusal.reason, zh) : (reason as Error).message}`;
};

// 初始姿势: hold-to-move to the home pose, a momentary command with no desired state. While the button is held each
// robot is sent hold every 200 ms (the robot stops by itself half a second after the last one); letting go, leaving
// the button or the window sends stop. A refusal or failure of any robot stops them all.
export function RobotPoseButton({ robots, zh }: { robots: ControlledRobot[]; zh: boolean }) {
  // Confirming is the first hold on its way; the robot has not acknowledged it yet. The phase changes only from the
  // session that is still held, so a hold receipt that arrives after release cannot put the button back to moving.
  const [phase, setPhase] = useState<"idle" | "confirming" | "holding" | "releasing">("idle");
  const [message, setMessage] = useState("");
  const [hint, setHint] = useState("");
  const held = useRef<{ session: PoseSession; at: number; timer: number; busy: boolean; confirmed: boolean; last: Promise<unknown> } | null>(null);
  const failed = useRef(false);
  const alive = useRef(true);
  const ready = robots.length > 0 && robots.every((robot) => robotPowerState(robot.labels) === "ready");
  const blocked = robots.find((robot) => robotPowerState(robot.labels) !== "ready");
  const blockedState = blocked ? robotPowerState(blocked.labels) : "";
  const unavailable = ready ? "" : robots.length === 0
    ? (zh ? "没有机器人" : "No robot")
    : blockedState === "off"
      ? (zh ? "电源未上电，先上电" : "Power it on first")
      : blockedState === "powered"
        ? (zh ? "驱动没在跑，先启动驱动" : "Start the driver first")
        : blockedState === "estop"
          ? (zh ? "急停已按下" : "The emergency stop is pressed")
          : blockedState === "fault"
            ? (zh ? "电源状态为硬件报错" : "The power state reports a hardware error")
            : blockedState === "unknown"
              ? (zh ? "读不到电源状态，厂商监控没有应答" : "The power state is unreadable; the vendor's monitor does not answer")
              : (zh ? "电源状态未上报" : "The power state has not been reported");
  const blockedHint = unavailable ? (zh ? `回位不可用：${unavailable}` : `Home pose unavailable: ${unavailable}`) : "";

  const send = async (action: "hold" | "stop", targets: ControlledRobot[]) => (await Promise.all(targets.map((robot) =>
    stationsApi.robotPose(robot.node, robot.namespace, action, "home", AbortSignal.timeout(action === "stop" ? STOP_TIMEOUT_MS : HOLD_TIMEOUT_MS)).then(() => "", (reason) => poseFailure(robot, reason, zh))))).filter(Boolean);

  const release = () => {
    const hold = held.current;
    if (!hold || hold.session.state !== "holding") return;
    held.current = null;
    hold.session.state = "releasing";
    setPhase("releasing");
    window.clearInterval(hold.timer);
    const heldFor = performance.now() - hold.at;
    const session = hold.session;
    // The stop goes out even if the last hold is still in flight: a bounded wait, then stop regardless, with the
    // outcome honest about what was proven. UI closing does not wait for this promise.
    session.done = Promise.race([hold.last, new Promise((resolve) => window.setTimeout(resolve, LAST_HOLD_WAIT_MS))])
      .then(() => send("stop", robots))
      .then((failures): PoseReleaseState => {
        if (failures.length) {
          setMessage(failures.join("; "));
          return "release-failed";
        }
        if (!failed.current) setHint(heldFor < 400
          ? (zh ? "按住不放才会移动，松开即停。" : "Hold the button to move; let go to stop.")
          : (zh ? "已停止。" : "Stopped."));
        return "released";
      })
      .catch((): PoseReleaseState => {
        setMessage(zh ? "停止请求没有收到结果；机器人会在最后一次 hold 约 0.5 秒后自行停止。" : "The stop request had no result; the robot stops by itself about 0.5 s after the last hold.");
        return "release-unknown";
      })
      .finally(() => {
        sessions.delete([...session.keys].join("|"));
        if (alive.current && !held.current) setPhase("idle");
      });
  };
  const tick = () => {
    const hold = held.current;
    if (!hold || hold.busy) return;
    hold.busy = true;
    hold.last = send("hold", robots).then((failures) => {
      hold.busy = false;
      // A receipt that arrives after the button was let go settles nothing in the UI: the session is gone.
      if (held.current !== hold) return;
      if (failures.length) {
        failed.current = true;
        setMessage(failures.join("; "));
        release();
        return;
      }
      if (!hold.confirmed) {
        hold.confirmed = true;
        setPhase("holding");
      }
    });
  };
  const press = () => {
    if (held.current || !ready || poseSessionBusy(robots)) return;
    setMessage("");
    setHint("");
    failed.current = false;
    setPhase("confirming");
    const session: PoseSession = { keys: new Set(sessionKeys(robots)), state: "holding", release: () => {}, done: Promise.resolve("released") };
    session.release = release;
    sessions.set([...session.keys].join("|"), session);
    held.current = { session, at: performance.now(), timer: 0, busy: false, confirmed: false, last: Promise.resolve() };
    tick();
    held.current.timer = window.setInterval(tick, HOLD_PERIOD_MS);
  };

  // Let go anywhere, the window losing focus, the page going away or a robot no longer ready ends the hold.
  const releaseRef = useRef(release);
  releaseRef.current = release;
  useEffect(() => {
    alive.current = true;
    const end = () => releaseRef.current();
    window.addEventListener("pointerup", end);
    window.addEventListener("blur", end);
    return () => {
      alive.current = false;
      window.removeEventListener("pointerup", end);
      window.removeEventListener("blur", end);
      end();
    };
  }, []);
  useEffect(() => {
    if (!ready) releaseRef.current();
  }, [ready]);

  const label = zh ? "按住回位" : "Hold home";
  const moving = phase === "holding";
  const confirming = phase === "confirming";
  const releasing = phase === "releasing";
  return <>
    <button
      type="button"
      className={`secondary-button robot-pose-button${moving ? " is-moving" : ""}${confirming ? " is-confirming" : ""}${releasing ? " is-releasing" : ""}`}
      disabled={!ready && !confirming && !moving && !releasing}
      title={ready
        ? confirming
          ? (zh ? "已按下，等待机器人确认第一个 hold；松开即停" : "Pressed; waiting for the robot to acknowledge the first hold. Let go to stop")
          : (zh ? "按住移动到初始姿势，松开即停" : "Hold to move to the home pose; let go to stop")
        : blockedHint}
      onPointerDown={(event) => event.button === 0 && press()}
      onPointerUp={release}
      onPointerLeave={release}
      onPointerCancel={release}
      onContextMenu={(event) => event.preventDefault()}
    >
      <House size={14} />{confirming ? (zh ? "请求中…" : "Requesting…") : moving ? (zh ? "移动中…松开即停" : "Moving… let go to stop") : releasing ? (zh ? "停止中…" : "Stopping…") : label}
    </button>
    {!ready && <small className="robot-pose-hint">{blockedHint}</small>}
    {hint && <small className="robot-pose-hint" role="status">{hint}</small>}
    {message && <small role="alert" className="robot-controls-error">{message}</small>}
  </>;
}

// The robot buttons of a page: power on and start the driver, stop the driver and power off, and hold-to-move to
// the home pose, for the chosen robots of those shown. single is a page of one robot: no count of chosen ones.
export function RobotControls({ robots, chosen, power, zh, single = false, showMessage = true, showPose = true }: {
  robots: ControlledRobot[];
  chosen: ControlledRobot[];
  power: RobotPower;
  zh: boolean;
  single?: boolean;
  showMessage?: boolean;
  showPose?: boolean;
}) {
  const press = async (desired: RobotPowerDesired, label: string) => {
    // The machine must not be moving while the browser blocks on the confirmation: release first, bounded, then
    // let useRobotPower run its confirm and requests.
    await releasePoseSessions(chosen);
    void power.set(chosen.map((robot) => ({ key: robot.key, node: robot.node, namespace: robot.namespace, desired, action: desired === "off" ? "power_off" : "power_on" })), label);
  };
  return (
    <div className="robot-controls" role="group" aria-label={zh ? "机器人控制" : "Robot controls"}>
      <Power size={14} />
      <span>{single ? (zh ? "机器人" : "Robot") : (zh ? `机器人：已选 ${chosen.length} / ${robots.length} 台` : `Robots: ${chosen.length} of ${robots.length} chosen`)}</span>
      <button type="button" className="secondary-button" disabled={power.pending || chosen.length === 0} onClick={() => void press("ready", zh ? "上电并启动驱动" : "Power on and start driver")}>{zh ? "上电并启动驱动" : "Power on and start driver"}</button>
      <button type="button" className="secondary-button" disabled={power.pending || chosen.length === 0} onClick={() => void press("off", zh ? "关闭驱动并下电" : "Stop driver and power off")}>{zh ? "关闭驱动并下电" : "Stop driver and power off"}</button>
      {showPose && <RobotPoseButton robots={chosen} zh={zh} />}
      {showMessage && power.message && <small role="status">{power.message}</small>}
    </div>
  );
}

// The controls of one robot's node page; nothing for a node that is not a robot.
export function RobotNodeControls({ node, zh }: { node: CRDNode; zh: boolean }) {
  const power = useRobotPower(zh);
  if (!hasNodeCategory(node, "robot")) return null;
  const robot = controlledRobot(node);
  return <RobotControls robots={[robot]} chosen={[robot]} power={power} zh={zh} single />;
}
