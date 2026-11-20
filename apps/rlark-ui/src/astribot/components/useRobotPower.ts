import { useState } from "react";
import { stationsApi } from "../api";
import type { RobotPowerDesired } from "../lib/siteNodeDisplay";

// The desired power of robots' Nodes, which the Addon's power service converges on: one request per robot, each
// recorded by the Gateway with who made it. onChanged reloads what the page shows once they are set.
export function useRobotPower(zh: boolean, onChanged?: () => unknown) {
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const set = async (nodes: { node: string; namespace: string; desired: RobotPowerDesired }[], label: string) => {
    if (!nodes.length) return;
    const names = nodes.map((entry) => entry.node).join(zh ? "、" : ", ");
    if (!window.confirm(zh ? `${label}：${names}\n\n请确认这些机器人周围安全、现场有人守着急停。` : `${label}: ${names}\n\nMake sure the robots' surroundings are safe and someone is at their emergency stops.`)) return;
    setPending(true);
    setMessage("");
    const failed: string[] = [];
    await Promise.all(nodes.map((entry) => stationsApi.setRobotPower(entry.node, entry.namespace, entry.desired)
      .catch((reason) => failed.push(`${entry.node}: ${(reason as Error).message}`))));
    setMessage(failed.length
      ? failed.join("; ")
      : (zh ? `期望状态已改（${label}，${nodes.length} 台），机器人几秒内跟上；没跟上的原因见节点注解 rlark.io/power 的 waiting。` : `Desired state set (${label}, ${nodes.length}); the robots follow within seconds. Why one does not is in its Node's annotation rlark.io/power (waiting).`));
    setPending(false);
    await onChanged?.();
  };
  return { pending, message, set };
}

export type RobotPower = ReturnType<typeof useRobotPower>;
