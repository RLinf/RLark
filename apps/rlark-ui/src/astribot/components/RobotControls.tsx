import { useEffect, useRef, useState } from "react";
import { House, Power } from "lucide-react";
import { ApiError } from "../../api";
import { stationsApi, type RobotPoseRefusal } from "../api";
import type { CRDNode } from "../../types";
import { hasNodeCategory } from "../../utils/nodes";
import { controlledRobot, robotPoseRefusalLabel, robotPowerState, type ControlledRobot, type RobotPowerDesired } from "../lib/siteNodeDisplay";
import { useRobotPower, type RobotPower } from "./useRobotPower";
import "./robotControls.css";

const poseFailure = (robot: ControlledRobot, reason: unknown, zh: boolean) => {
  const refusal = reason instanceof ApiError && reason.status === 409 ? (reason.body as RobotPoseRefusal | undefined) : undefined;
  return `${robot.node}: ${refusal?.reason ? robotPoseRefusalLabel(refusal.reason, zh) : (reason as Error).message}`;
};

// 初始姿势: hold-to-move to the home pose, a momentary command with no desired state. While the button is held each
// robot is sent hold every 200 ms (the robot stops by itself half a second after the last one); letting go, leaving
// the button or the window sends stop. A refusal or failure of any robot stops them all.
export function RobotPoseButton({ robots, zh }: { robots: ControlledRobot[]; zh: boolean }) {
  const [moving, setMoving] = useState(false);
  const [message, setMessage] = useState("");
  const held = useRef<{ robots: ControlledRobot[]; timer: number; busy: boolean; last: Promise<unknown> } | null>(null);
  const ready = robots.length > 0 && robots.every((robot) => robotPowerState(robot.labels) === "ready");

  const send = async (action: "hold" | "stop", targets: ControlledRobot[]) => (await Promise.all(targets.map((robot) =>
    stationsApi.robotPose(robot.node, robot.namespace, action).then(() => "", (reason) => poseFailure(robot, reason, zh))))).filter(Boolean);

  const release = () => {
    const hold = held.current;
    if (!hold) return;
    held.current = null;
    window.clearInterval(hold.timer);
    setMoving(false);
    void hold.last.then(() => send("stop", hold.robots)).then((failed) => failed.length && setMessage(failed.join("; ")));
  };
  const tick = () => {
    const hold = held.current!;
    if (hold.busy) return;
    hold.busy = true;
    hold.last = send("hold", hold.robots).then((failed) => {
      hold.busy = false;
      if (failed.length && held.current === hold) {
        setMessage(failed.join("; "));
        release();
      }
    });
  };
  const press = () => {
    if (held.current || !ready) return;
    setMessage("");
    setMoving(true);
    held.current = { robots, timer: 0, busy: false, last: Promise.resolve() };
    tick();
    held.current.timer = window.setInterval(tick, 200);
  };

  // Let go anywhere, the window losing focus, the page going away or a robot no longer ready ends the hold.
  const releaseRef = useRef(release);
  releaseRef.current = release;
  useEffect(() => {
    const end = () => releaseRef.current();
    window.addEventListener("pointerup", end);
    window.addEventListener("blur", end);
    return () => {
      window.removeEventListener("pointerup", end);
      window.removeEventListener("blur", end);
      end();
    };
  }, []);
  useEffect(() => {
    if (!ready) releaseRef.current();
  }, [ready]);

  const label = zh ? "初始姿势" : "Home pose";
  return <>
    <button
      type="button"
      className={`secondary-button robot-pose-button${moving ? " is-moving" : ""}`}
      disabled={!moving && !ready}
      title={ready
        ? (zh ? "按住移动到初始姿势，松开即停" : "Hold to move to the home pose; let go to stop")
        : (zh ? "选中的机器人都要已上电、驱动在跑" : "Every chosen robot must be powered with its driver running")}
      onPointerDown={(event) => event.button === 0 && press()}
      onPointerUp={release}
      onPointerLeave={release}
      onPointerCancel={release}
      onContextMenu={(event) => event.preventDefault()}
    >
      <House size={14} />{moving ? (zh ? "移动中…松开停止" : "Moving… let go to stop") : label}
    </button>
    {message && <small role="alert" className="robot-controls-error">{message}</small>}
  </>;
}

// The robot buttons of a page: power on and start the driver, stop the driver and power off, and hold-to-move to the
// home pose, for the chosen robots of those shown. single is a page of one robot: no count of chosen ones.
export function RobotControls({ robots, chosen, power, zh, single = false }: {
  robots: ControlledRobot[];
  chosen: ControlledRobot[];
  power: RobotPower;
  zh: boolean;
  single?: boolean;
}) {
  const press = (desired: RobotPowerDesired, label: string) =>
    void power.set(chosen.map((robot) => ({ node: robot.node, namespace: robot.namespace, desired })), label);
  return (
    <div className="robot-controls" role="group" aria-label={zh ? "机器人控制" : "Robot controls"}>
      <Power size={14} />
      <span>{single ? (zh ? "机器人" : "Robot") : (zh ? `机器人：已选 ${chosen.length} / ${robots.length} 台` : `Robots: ${chosen.length} of ${robots.length} chosen`)}</span>
      <button type="button" className="secondary-button" disabled={power.pending || chosen.length === 0} onClick={() => press("ready", zh ? "上电并启动驱动" : "Power on and start driver")}>{zh ? "上电并启动驱动" : "Power on and start driver"}</button>
      <button type="button" className="secondary-button" disabled={power.pending || chosen.length === 0} onClick={() => press("off", zh ? "关闭驱动并下电" : "Stop driver and power off")}>{zh ? "关闭驱动并下电" : "Stop driver and power off"}</button>
      <RobotPoseButton robots={chosen} zh={zh} />
      {power.message && <small role="status">{power.message}</small>}
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
