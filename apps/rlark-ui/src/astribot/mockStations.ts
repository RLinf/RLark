// The mock console's site-node fixtures: a robot cluster with a control plane, two cloud clusters, and enough
// states to exercise the cards, the full-row detail and the camera preview without a backend.
import type { StationItem } from "./api";
import type { CameraReport, CameraView } from "./lib/cameras";
import type { NodeAdmission, SiteNode } from "./lib/siteNodeDisplay";

const now = Date.now();
const ago = (seconds: number) => new Date(now - seconds * 1000).toISOString();
const uid = (n: number) => `3f712a0${n}-af6d-4c1c-9307-29a809b3da0${n}`;
const system = (n: number) => `d49b0c2${n}-53df-44fe-b14e-14e6524c231${n}`;
const cp = { "node-role.kubernetes.io/control-plane": "" };
const robot = (extra: Record<string, string> = {}) => ({ "rlark.io/robot-model": "s1u", ...extra });
const network = (n: number) => ({ public_key: `B5RwJ8qmVszSa08Nb30Ck6wHx9JpfQAeTwR6G2eC10${n}=`, links: [], link_summary: "" });

export const mockStationClusterNames = {
  robot: "深圳公司S1U调试集群",
  cloud: "金山云西北宁夏裸金属集群",
  local: "深圳公司本地开发集群",
};

const camera = (state: CameraReport["state"], extra: Partial<CameraReport> = {}): CameraReport => ({
  state,
  fps: state === "ok" ? 30 : 0,
  age_seconds: state === "ok" ? 0.1 : 120,
  step: 0,
  attempts: 0,
  next_attempt_at: null,
  last_action: null,
  last_result: null,
  diagnosis: null,
  usb: null,
  usb_advice: null,
  ...extra,
});

const cameras = (entries: Partial<Record<CameraView, CameraReport>>) => JSON.stringify({
  state: "ok",
  cameras: entries,
  updated_at: Math.floor(now / 1000),
});

const condition = (type: string, status: "True" | "False" | "Unknown", reason: string, message: string, seconds: number) =>
  ({ type, status, reason, message, lastTransitionTime: ago(seconds) });

const healthyConditions = () => [
  condition("Observed", "True", "NodeReported", "The node reported its state.", 64),
  condition("IdentityMatched", "True", "Matched", "Bound UID and report agree.", 64),
  condition("NetworkVerified", "True", "ProbeSucceeded", "The last probe succeeded.", 58),
  condition("RLarkLinked", "True", "LinkEstablished", "The node is connected to the management plane.", 58),
  condition("Rebooted", "Unknown", "NoRecentReboot", "No recent reboot was observed.", 78),
];

const conflictConditions = () => [
  condition("Observed", "True", "NodeReported", "The node reported its state.", 64),
  condition("IdentityMatched", "False", "UIDMismatch", "A node with this name reported a different UID.", 58),
  condition("NetworkVerified", "False", "ProbeUnavailable", "No successful probe result is available.", 41),
  condition("RLarkLinked", "False", "IdentityConflict", "A node with this name reported a different UID.", 58),
  condition("Rebooted", "True", "BootTimeChanged", "A boot time change was detected.", 78),
];

const offlineConditions = () => [
  condition("Observed", "Unknown", "ReportStale", "The node has not reported recently.", 7200),
  condition("IdentityMatched", "Unknown", "AwaitingReport", "Waiting for the node to report.", 7200),
  condition("NetworkVerified", "Unknown", "AwaitingProbe", "No probe result is available.", 7200),
  condition("RLarkLinked", "Unknown", "ObservationMissing", "Waiting for the node to report.", 7200),
  condition("Rebooted", "Unknown", "NoRecentReboot", "No recent reboot was observed.", 7200),
];

export const mockStations: StationItem[] = [
  { metadata: { name: "10-0-2-57", namespace: mockStationClusterNames.cloud }, spec: { nodeUID: uid(0) }, status: { observation: { nodeUID: uid(0), systemUUID: system(0) }, node: { state: "Bound", bootTime: ago(14 * 86400), lastSeenTime: ago(33), reboots: 0, conditions: healthyConditions() } } },
  { metadata: { name: "10-0-2-196", namespace: mockStationClusterNames.cloud }, spec: { nodeUID: uid(1) }, status: { observation: { nodeUID: uid(1), systemUUID: system(1) }, node: { state: "Bound", bootTime: ago(2 * 86400 + 14400), lastSeenTime: ago(16), reboots: 1, conditions: healthyConditions() } } },
  { metadata: { name: "calliang", namespace: mockStationClusterNames.local }, spec: { nodeUID: uid(2) }, status: { observation: { nodeUID: uid(2), systemUUID: system(2) }, node: { state: "Bound", bootTime: ago(3 * 86400 + 14400), lastSeenTime: ago(24), reboots: 2, conditions: [condition("Observed", "True", "NodeReported", "The node reported its state.", 24), condition("IdentityMatched", "True", "Matched", "Bound UID and report agree.", 24), condition("NetworkVerified", "False", "ProbeUnavailable", "The last probe did not get through.", 31), condition("RLarkLinked", "True", "LinkEstablished", "The node is connected to the management plane.", 24), condition("Rebooted", "Unknown", "NoRecentReboot", "No recent reboot was observed.", 24)] } } },
  { metadata: { name: "wendychen", namespace: mockStationClusterNames.local }, spec: { nodeUID: uid(3) }, status: { observation: { nodeUID: uid(3), systemUUID: system(3) }, node: { state: "Bound", bootTime: ago(4 * 86400 + 14400), lastSeenTime: ago(32), reboots: 0, conditions: healthyConditions() } } },
  { metadata: { name: "alexfu", namespace: mockStationClusterNames.robot }, spec: { nodeUID: uid(4) }, status: { observation: { nodeUID: uid(4), systemUUID: system(4) }, node: { state: "Bound", bootTime: ago(5 * 86400 + 14400), lastSeenTime: ago(40), reboots: 1, conditions: healthyConditions() } } },
  { metadata: { name: "s1u-01-x86", namespace: mockStationClusterNames.robot }, spec: { nodeUID: uid(5) }, status: { observation: { nodeUID: uid(5), systemUUID: system(5) }, node: { state: "Bound", bootTime: ago(6 * 86400 + 14400), lastSeenTime: ago(48), reboots: 2, conditions: healthyConditions() } } },
  { metadata: { name: "s1u-02-x86", namespace: mockStationClusterNames.robot }, spec: { nodeUID: uid(6) }, status: { observation: { nodeUID: uid(6), systemUUID: system(6) }, node: { state: "Offline", lastSeenTime: ago(2 * 3600), reboots: 0, conditions: offlineConditions() } } },
  { metadata: { name: "s1u-03-x86", namespace: mockStationClusterNames.robot }, spec: { nodeUID: uid(7) }, status: { observation: { nodeUID: uid(8), systemUUID: system(7) }, node: { state: "Conflict", lastSeenTime: ago(64), reboots: 1, conditions: conflictConditions() } } },
];

export const mockSiteNodes: SiteNode[] = [
  { cluster_id: mockStationClusterNames.cloud, node_name: "10-0-2-57", online: true, labels: cp, network: network(0) },
  { cluster_id: mockStationClusterNames.cloud, node_name: "10-0-2-196", online: true, labels: {}, network: network(1) },
  { cluster_id: mockStationClusterNames.local, node_name: "calliang", online: true, labels: cp, network: network(2) },
  { cluster_id: mockStationClusterNames.local, node_name: "wendychen", online: true, labels: {}, network: network(3) },
  { cluster_id: mockStationClusterNames.robot, node_name: "alexfu", online: true, labels: cp, network: network(4) },
  { cluster_id: mockStationClusterNames.robot, node_name: "s1u-01-x86", online: true, labels: robot({ "rlark.io/power-state": "ready", "rlark.io/power-desired": "ready", "rlark.io/self-check-state": "ok" }), cameras: cameras({ head: camera("ok"), torso: camera("ok"), wrist_left: camera("slow", { fps: 7 }), wrist_right: camera("ok") }), network: network(5) },
  { cluster_id: mockStationClusterNames.robot, node_name: "s1u-02-x86", online: false, labels: robot({ "rlark.io/power-state": "powered", "rlark.io/power-desired": "ready", "rlark.io/self-check-state": "pending" }), cameras: cameras({ head: camera("stale"), torso: camera("absent") }), network: network(6) },
  { cluster_id: mockStationClusterNames.robot, node_name: "s1u-03-x86", online: true, labels: robot({ "rlark.io/power-state": "estop", "rlark.io/power-desired": "off", "rlark.io/self-check-state": "failed", "rlark.io/maintenance-state": "waiting" }), cameras: cameras({ head: camera("ok"), torso: camera("stale", { step: 3, attempts: 4, last_result: "USB reset did not hold", diagnosis: "Orbbec camera on USB 2.1" }) }), network: network(7) },
];

export const mockAdmissions: NodeAdmission[] = [
  { cluster_id: mockStationClusterNames.cloud, node_name: "10-0-2-57", state: "Joined", labels: cp, expires_at: ago(-86400), issued_by: "admin", public_key: network(0).public_key, tunnel_address: "10.254.240.20", ssh_login: "astri@10.254.240.20:22", ssh_host_key: "SHA256:h3GxpQ5FwUu32tTMo0sErSj72l9aW1L1f85A7eD3Kr0" },
  { cluster_id: mockStationClusterNames.local, node_name: "calliang", state: "Joined", labels: cp, expires_at: ago(-86400), issued_by: "admin", public_key: network(2).public_key, tunnel_address: "10.254.240.22", ssh_login: "astri@10.254.240.22:22", ssh_host_key: "SHA256:h3GxpQ5FwUu32tTMo0sErSj72l9aW1L1f85A7eD3Kr2" },
  { cluster_id: mockStationClusterNames.local, node_name: "wendychen", state: "Expired", labels: {}, expires_at: ago(86400), issued_by: "admin", public_key: network(3).public_key, tunnel_address: "10.254.240.23", ssh_login: "astri@10.254.240.23:22", ssh_host_key: "SHA256:h3GxpQ5FwUu32tTMo0sErSj72l9aW1L1f85A7eD3Kr3" },
  { cluster_id: mockStationClusterNames.robot, node_name: "s1u-01-x86", state: "Joined", labels: robot(), expires_at: ago(-86400), issued_by: "admin", public_key: network(5).public_key, tunnel_address: "10.254.240.25", ssh_login: "astri@10.254.240.25:22", ssh_host_key: "SHA256:h3GxpQ5FwUu32tTMo0sErSj72l9aW1L1f85A7eD3Kr5" },
  { cluster_id: mockStationClusterNames.robot, node_name: "s1u-03-x86", state: "Redeemed", labels: robot(), expires_at: ago(86400), issued_by: "admin", public_key: network(7).public_key, tunnel_address: "10.254.240.27", ssh_login: "astri@10.254.240.27:22", ssh_host_key: "SHA256:h3GxpQ5FwUu32tTMo0sErSj72l9aW1L1f85A7eD3Kr7" },
];

export const mockStationCameraReport = (nodeName: string) => {
  const entry = mockSiteNodes.find((node) => node.node_name === nodeName);
  return entry?.cameras ? JSON.parse(entry.cameras) : { state: "ok", cameras: {}, updated_at: Math.floor(now / 1000) };
};

export const mockStationCameraFrame = (view: string) => `<svg xmlns="http://www.w3.org/2000/svg" width="640" height="360"><rect width="640" height="360" fill="#172534"/><path d="M0 360 192 190h256l192 170Z" fill="#394957"/><text x="28" y="64" fill="#7dd3fc" font-size="34">${view}</text><text x="28" y="104" fill="#9ca8ba" font-size="20">mock frame</text></svg>`;
