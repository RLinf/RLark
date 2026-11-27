import assert from "node:assert/strict";
import test from "node:test";
import {
  admissionStateClass,
  admissionStateLabel,
  classPasswordLabel,
  hasRegisteredRobotClass,
  isRobotNode,
  keyFingerprint,
  machineClassAccess,
  machineLabelChoices,
  machineModel,
  machineNotTickable,
  machineReach,
  machineStateLabel,
  parseLabelLines,
  parseTaintLines,
  robotPowerActionBlocked,
  robotPowerActionTiming,
  robotPowerActionWait,
  robotPowerAfter,
  robotPowerStateNote,
  robotPowerWaiting,
  unseenMachineOf,
  robotPowerDesired,
  robotPowerNodes,
  robotPowerState,
  robotPowerStateClass,
  robotPowerStateLabel,
  robotSelfCheckLabel,
  robotSelfCheckState,
  s1uRobotLabels,
  sshJoinFailure,
  unjoinedMachines,
  robotIDOfNode,
} from "../dist/test/astribot/lib/siteNodeDisplay.js";

test("a WireGuard key shows as the beginning wg show prints", () => {
  assert.equal(keyFingerprint("hG3m0e1yZr0u8m5h2a+4yR8Hn0Lz3pQkV2c7xW9eJ1s="), "hG3m0e1yZr…");
  assert.equal(keyFingerprint(undefined), "");
});

test("the machine's model comes from its robot model, else its category", () => {
  assert.equal(machineModel({ "rlark.io/robot-model": "s1u", "rlark.io/node-category": "robot" }), "s1u");
  assert.equal(machineModel({ "rlark.io/node-category": "gpu" }), "gpu");
  assert.equal(machineModel(undefined), "");
});

test("entry reachability applies only to robot machines with a registered NodeClass", () => {
  const classes = { s1u: { kind: "Robot" } };
  assert.equal(hasRegisteredRobotClass({ node_class: "", machine_type: "Unknown" }, classes), false);
  assert.equal(hasRegisteredRobotClass({ node_class: "s1u", machine_type: "Unknown" }, classes), false);
  assert.equal(hasRegisteredRobotClass({ node_class: "s1u", machine_type: "Server" }, classes), false);
  assert.equal(hasRegisteredRobotClass({ node_class: "missing", machine_type: "Robot" }, classes), false);
  assert.equal(hasRegisteredRobotClass({ node_class: "s1u", machine_type: "Robot" }, classes), true);
});

test("label and taint lines are read one per line and a bad line is named", () => {
  assert.deepEqual(parseLabelLines(s1uRobotLabels("s1u-09")).labels, {
    "rlark.io/node-category": "robot", "rlark.io/robot-model": "s1u", "rlark.io/robot-body": "s1u", "rlark.io/robot-id": "s1u-09",
  });
  assert.equal(parseLabelLines("a=b\n\nnovalue").error, "novalue");
  assert.deepEqual(parseTaintLines("rlark.io/node-category=robot:NoSchedule\nk:NoExecute").taints, ["rlark.io/node-category=robot:NoSchedule", "k:NoExecute"]);
  assert.equal(parseTaintLines("k=v").error, "k=v");
});

test("every admission state has a pill and a label", () => {
  for (const state of ["Pending", "Redeemed", "Joined", "Expired"]) {
    assert.match(admissionStateClass(state), /^status-/);
    assert.ok(admissionStateLabel(state, true));
  }
});

test("the add-node list shows the machines that are not nodes, the ones that can be ticked first", () => {
  const machine = (name, phase, problem, admissible, last_seen) => ({ name, site: "lab", node_class: "s1u", source: "Discovery", phase, problem, admissible, last_seen });
  const listed = unjoinedMachines([
    machine("refused", "Discovered", "CredentialRejected", false, "2027-01-15T08:00:30Z"),
    machine("older", "Discovered", undefined, true, "2027-01-15T08:00:00Z"),
    machine("joined", "Joined", undefined, false, "2027-01-15T08:00:40Z"),
    machine("newer", "Discovered", undefined, true, "2027-01-15T08:00:20Z"),
    machine("stopped", "Admitted", "JoinBlocked", true, "2027-01-15T08:00:10Z"),
    // Being joined to this cluster: it is in the join progress, not here; to another cluster, it is here and cannot be ticked.
    { ...machine("ticked", "Discovered", undefined, false, "2027-01-15T08:00:50Z"), joining: true, admit_cluster_id: "robots" },
    { ...machine("elsewhere", "Admitted", undefined, false, "2027-01-15T08:00:05Z"), joining: true, admit_cluster_id: "office", requested_by: "bob" },
  ], "robots");
  assert.deepEqual(listed.map((one) => one.name), ["newer", "stopped", "older", "refused", "elsewhere"]);
  assert.equal(machineNotTickable(listed[4], true), "正在加入集群 office（bob 提交）");
  // A machine whose password was changed cannot be ticked, and the list says why.
  assert.equal(listed[3].admissible, false);
  assert.equal(machineStateLabel(listed[3], true), "Orin 口令不符");
  assert.equal(machineStateLabel(listed[3], false), "Entry device refused the password");
  assert.equal(machineStateLabel({ phase: "Discovered" }, true), "未加入");
  assert.equal(machineStateLabel({ phase: "Admitted" }, true), "入网中");
  assert.equal(machineStateLabel({ phase: "Discovered", problem: "SomethingNew" }, true), "SomethingNew");
});

test("a class whose x86 password is not entered says so, and its machines cannot be ticked", () => {
  const password = (key, configured, set_by) => ({ secret: "fleet-orin-access", key, configured, set_by });
  const before = { s1u: { labels: {}, access: { entry: password("password", true), node: password("x86_password", false) } }, gpu: { labels: {} },
    arm: { access: { entry: password("password", true), node: password("arm_password", true, "alice") } } };
  // What the Gateway lists while the password is missing: the machine has the problem and is not admissible.
  const waiting = unjoinedMachines([{ name: "lab-01", site: "lab", node_class: "s1u", source: "Discovery", phase: "Discovered", problem: "AccessUnconfigured", admissible: false }], "robots");
  assert.equal(waiting[0].admissible, false);
  assert.equal(machineStateLabel(waiting[0], true), "该机型的口令未配置");
  // The classes that declare access, the one waiting for its password first; a class without access is not shown.
  const shown = machineClassAccess(before);
  assert.deepEqual(shown.map((one) => one.name), ["s1u", "arm"]);
  assert.equal(classPasswordLabel(shown[0].access.entry, true), "已配置（来自 fleet-orin-access）");
  assert.equal(classPasswordLabel(shown[0].access.node, true), "未配置");
  assert.equal(classPasswordLabel({ configured: true }, true), "已配置");
  assert.equal(classPasswordLabel({ configured: true, set_by: "alice" }, false), "Entered by alice");
  assert.equal(classPasswordLabel(shown[1].access.node, true), "已配置（来自 fleet-orin-access，alice 录入）");
  // Once it is saved, the machine can be ticked.
  const after = unjoinedMachines([{ name: "lab-01", site: "lab", node_class: "s1u", source: "Discovery", phase: "Discovered", serial: "1794925030610", admissible: true }], "robots");
  assert.equal(after[0].admissible, true);
  assert.equal(machineStateLabel(after[0], true), "未加入");
});

test("the list says whether RLark got through to each machine's Orin and x86, and why one cannot be ticked", () => {
  const read = { name: "m", phase: "Discovered", serial: "1794925030610", inspected_at: "2027-01-15T07:58:00Z", node_host_key_fingerprint: "SHA256:x86", admissible: true, joining: false };
  const states = (machine) => { const { orin, x86 } = machineReach(machine, true); return [orin.state, x86.state]; };
  assert.deepEqual(states(read), ["ok", "ok"]);
  assert.equal(machineNotTickable(read, true), "");
  // Not read yet, or read again after a fault that is not the machine's.
  assert.deepEqual(states({ ...read, inspected_at: undefined, admissible: false }), ["checking", "checking"]);
  assert.equal(machineNotTickable({ ...read, inspected_at: undefined, admissible: false }, true), "等 RLark 登录读取完");
  assert.deepEqual(states({ ...read, problem: "InspectFailed", admissible: false }), ["checking", "checking"]);
  // The Orin is not reached: nothing is known of the x86.
  for (const problem of ["EntryUnreachable", "HubUnreachable", "CredentialRejected", "HostKeyChanged"]) {
    assert.deepEqual(states({ ...read, problem, admissible: false }), ["fail", "unknown"], problem);
  }
  assert.equal(machineNotTickable({ ...read, problem: "CredentialRejected", admissible: false }, true), "Orin 不通：Orin 口令不符");
  // The Orin answers, the x86 behind it does not answer, refuses the class's account or shows another key.
  assert.deepEqual(states({ ...read, problem: "NodeUnreachable", node_host_key_fingerprint: undefined, admissible: false }), ["ok", "fail"]);
  for (const problem of ["NodeCredentialRejected", "NodeHostKeyChanged"]) {
    assert.deepEqual(states({ ...read, problem, admissible: false }), ["ok", "fail"], problem);
  }
  assert.equal(machineNotTickable({ ...read, problem: "NodeUnreachable", admissible: false }, true), "x86 不通：Orin 通，x86 不通");
  assert.equal(machineNotTickable({ ...read, problem: "NodeHostKeyChanged", admissible: false }, true), "x86 不通：x86 主机密钥变了");
  // RLark logs in to no machine of a class without its passwords, read before or not.
  for (const machine of [{ ...read, problem: "AccessUnconfigured", admissible: false }, { name: "n", phase: "Discovered", problem: "ClassIncomplete", admissible: false }]) {
    assert.deepEqual(states(machine), ["unknown", "unknown"], machine.problem);
  }
  assert.equal(machineNotTickable({ ...read, problem: "AccessUnconfigured", admissible: false }, true), "该机型的口令未配置，RLark 不登录");
  // Both answer, and something else holds it back.
  assert.equal(machineNotTickable({ ...read, problem: "DuplicateSerial", admissible: false }, true), "序列号重复");
});

test("labels can be replaced for ticked machines of one class only", () => {
  const classes = { s1u: { labels: { "rlark.io/robot-body": "s1u" } }, gpu: { labels: { "rlark.io/node-category": "gpu" } } };
  assert.deepEqual(machineLabelChoices([{ node_class: "s1u" }, { node_class: "s1u" }], classes), { "rlark.io/robot-body": "s1u" });
  assert.deepEqual(machineLabelChoices([{ node_class: "s1u" }, { node_class: "gpu" }], classes), {});
  assert.deepEqual(machineLabelChoices([], classes), {});
});

test("a robot's desired power, its power and its self-check read from its Node's labels, and a button sets the desired state", () => {
  const labels = { "rlark.io/power-state": "ready", "rlark.io/self-check-state": "failed" };
  assert.equal(robotPowerState(labels), "ready");
  assert.equal(robotPowerState({}), "");
  // A robot without the label is wanted ready: one that just joined.
  assert.equal(robotPowerDesired(labels), "ready");
  assert.equal(robotPowerDesired({ "rlark.io/power-desired": "off" }), "off");
  assert.equal(robotPowerStateLabel("estop", true), "急停已按下");
  assert.equal(robotPowerStateClass("fault"), "status-error");
  assert.equal(robotSelfCheckLabel(robotSelfCheckState(labels), true), "没过");
  assert.equal(robotSelfCheckState(undefined), "");
  assert.equal(robotPowerAfter("power_on", "off"), "powered");
  assert.equal(robotPowerAfter("power_on", "ready"), "ready");
  assert.equal(robotPowerAfter("driver_on", "off"), "ready");
  assert.equal(robotPowerAfter("driver_off", "ready"), "powered");
  assert.equal(robotPowerAfter("power_off", "ready"), "off");
  // The state in force is the Addon's word: ready since a Run used the robot, whatever the label still reads.
  assert.equal(robotPowerDesired({ "rlark.io/robot-power": "off.1794729000", "rlark.io/power-desired": "ready" }), "ready");
  // A batch acts on the chosen robots only, whether or not their Addon reports the power yet; never on another node.
  const robot = { "rlark.io/robot-model": "s1u" };
  const nodes = [{ key: "a", labels: { ...robot, ...labels } }, { key: "b", labels: {} }, { key: "c", labels: { ...robot, "rlark.io/power-state": "off" } }, { key: "d", labels: robot }];
  assert.deepEqual(robotPowerNodes(nodes, new Set(["a", "b", "d"])).map((node) => node.key), ["a", "d"]);
  assert.equal(isRobotNode(robot), true);
  assert.equal(isRobotNode({ "rlark.io/node-category": "gpu" }), false);
});

test("a fixed tile action says why the current state blocks it and how long the device takes", () => {
  // The action pair is always shown; only the half that cannot act on this state is disabled and explained.
  assert.equal(robotPowerActionBlocked("power_on", "ready", true), "已上电，驱动在跑");
  assert.equal(robotPowerActionBlocked("power_on", "off", true), "");
  assert.equal(robotPowerActionBlocked("power_on", "estop", true), "急停已按下，先解除急停");
  assert.equal(robotPowerActionBlocked("power_off", "off", true), "已下电");
  assert.equal(robotPowerActionBlocked("power_off", "ready", true), "");
  assert.equal(robotPowerActionBlocked("driver_on", "powered", true), "");
  assert.equal(robotPowerActionBlocked("driver_on", "off", true), "先上电");
  assert.equal(robotPowerActionBlocked("driver_on", "ready", true), "驱动已在跑");
  assert.equal(robotPowerActionBlocked("driver_off", "ready", true), "");
  assert.equal(robotPowerActionBlocked("driver_off", "powered", true), "驱动已停");
  assert.equal(robotPowerActionBlocked("driver_on", "unknown", true), "读不到电源状态，电源服务会拒绝");
  assert.equal(robotPowerActionBlocked("driver_on", "", false), "The power state has not been reported");
  // Device actions take different times; the accepted state says which.
  assert.equal(robotPowerActionTiming("power_off", true), "下电约十几秒");
  assert.equal(robotPowerActionTiming("driver_on", true), "驱动启动约几秒");
  assert.match(robotPowerActionWait("power_on", true), /10 秒内不能再上电/);
  // A visible note explains the state-level reason; a ready robot needs none.
  assert.match(robotPowerStateNote("estop", true), /上电与启动驱动不可用/);
  assert.equal(robotPowerStateNote("ready", true), "");
  // The power service's own waiting text comes from the rlark.io/power annotation.
  assert.equal(robotPowerWaiting('{"state":"off","waiting":"10 s before power on"}'), "10 s before power on");
  assert.equal(robotPowerWaiting('{"state":"off"}'), "");
  assert.equal(robotPowerWaiting("not json"), "");
});

test("a robot's ID is its node's name without -x86", () => {
  assert.equal(robotIDOfNode("s1u-09-x86"), "s1u-09");
  assert.equal(robotIDOfNode(" lab-robot "), "lab-robot");
});

test("a refused SSH join says in words what to do", () => {
  for (const reason of ["Unreachable", "LoginRejected", "NoSudo", "SudoPasswordRejected", "NodeName"]) {
    const said = sshJoinFailure(reason, "detail", true);
    assert.ok(said.endsWith("（detail）") && said.length > "（detail）".length, reason);
  }
  assert.equal(sshJoinFailure("LoginRejected", "detail", false), "detail");
  assert.equal(sshJoinFailure(undefined, "Station API does not serve its bootstrap CA", true), "Station API does not serve its bootstrap CA");
});

test("removing a node whose machine no hub has seen for ten minutes is warned about", () => {
  const now = Date.parse("2026-11-20T03:00:00Z");
  const machine = (last_seen) => ({ name: "m", phase: "Joined", cluster_id: "c", node_name: "s1u-01-x86", admissible: false, last_seen });
  assert.equal(unseenMachineOf([machine("2026-11-20T02:55:00Z")], "c", "s1u-01-x86", now), undefined);
  assert.equal(unseenMachineOf([machine("2026-11-18T03:53:15Z")], "c", "s1u-01-x86", now)?.last_seen, "2026-11-18T03:53:15Z");
  assert.ok(unseenMachineOf([machine(undefined)], "c", "s1u-01-x86", now));
  assert.equal(unseenMachineOf([machine("2026-11-18T03:53:15Z")], "c", "other", now), undefined);
});
