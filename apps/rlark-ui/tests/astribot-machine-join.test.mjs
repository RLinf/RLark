import assert from "node:assert/strict";
import test from "node:test";
import { machineJoinProgress, machineJoinSteps, machineJoinSummary } from "../dist/test/astribot/lib/machineJoin.js";
import { scanJoinMachines, seenTime, unseenFor } from "../dist/test/astribot/lib/siteNodeDisplay.js";

// Local times, as the page shows them.
const now = new Date(2026, 10, 20, 16, 0, 0).getTime();
const at = (day, hour, minute) => new Date(2026, 10, day, hour, minute, 0).toISOString();
const machine = (name, fields) => ({
  name, site: "lab", node_class: "s1u", phase: "Discovered", admissible: false, address: "192.168.31.100", serial: `S-${name}`,
  last_seen: at(20, 15, 59), admit_cluster_id: "robots", requested_at: at(20, 15, 50), ...fields,
});

test("the scan list shows only recent results from the latest run and active joins", () => {
  const recent = machine("recent", { discovery_scan_id: "run-b", admissible: true, last_seen: at(20, 15, 51) });
  const expired = machine("expired", { discovery_scan_id: "run-b", admissible: true, last_seen: at(20, 15, 50) });
  const previous = machine("previous", { discovery_scan_id: "run-a", last_seen: at(20, 15, 59) });
  const legacy = machine("legacy", {});
  const absent = machine("absent", { discovery_scan_id: "run-b", last_seen: undefined });
  const joining = machine("joining", { joining: true, last_seen: at(20, 5, 0) });
  const joined = machine("joined", { discovery_scan_id: "run-b", phase: "Joined" });
  const names = (items) => items.map((item) => item.name);
  const records = [expired, joined, recent, previous, legacy, absent, joining];
  assert.deepEqual(names(scanJoinMachines(records, "robots", now)), ["joining"]);
  assert.deepEqual(names(scanJoinMachines(records, "robots", now, "run-b")), ["recent", "joining"]);
  assert.deepEqual(names(scanJoinMachines(records, "office", now, "run-b")), ["recent"]);
  assert.deepEqual(names(scanJoinMachines(records, "robots", now, "run-c")), ["joining"]);
});

test("each admitted machine says the step it has reached, and the batch how many joined", () => {
  const progress = machineJoinProgress([
    machine("waiting", {}),
    machine("pushing", { phase: "Admitted", node_name: "s1u-03-x86", join: { phase: "Running", stage: "Uplink" } }),
    machine("named", { phase: "Admitted", node_name: "s1u-04-x86" }),
    machine("joined", { phase: "Joined", node_name: "s1u-05-x86" }),
    machine("gone", { problem: "EntryUnreachable", last_seen: at(18, 11, 53) }),
    machine("blocked", { phase: "Admitted", problem: "JoinBlocked", message: "the installer failed", node_name: "s1u-06-x86", join: { phase: "Blocked", stage: "Push" } }),
    // Asked for days ago, and the hub has not reached it since: the request stands.
    machine("waiting long", { problem: "HubUnreachable", last_seen: at(17, 9, 0), requested_at: at(17, 10, 0) }),
    // Another cluster's, one a day old that joined long since, one whose node was removed since (its request is
    // spent), and one never admitted: not this batch.
    machine("other", { admit_cluster_id: "office" }),
    machine("old", { phase: "Joined", requested_at: at(18, 12, 0) }),
    machine("removed", { message: "its node was removed; admit it again to join", admissible: true, join: { phase: "Succeeded", stage: "RotateEntryHostKey" } }),
    machine("free", { admit_cluster_id: undefined, requested_at: undefined }),
  ], "robots", now, true);
  assert.deepEqual([progress.total, progress.joined, progress.running, progress.failed], [7, 1, 3, 3]);
  assert.equal(machineJoinSummary(progress, true), "已加入 1 / 共 7 台，进行中 3 台，失败 3 台");
  const of = (name) => progress.rows.find((row) => row.machine.name === name);
  // Stopped first, then under way, then joined.
  assert.deepEqual(progress.rows.map((row) => row.state), ["failed", "failed", "failed", "running", "running", "running", "joined"]);
  const steps = machineJoinSteps.length;
  assert.deepEqual([of("waiting").done, of("waiting").text], [0, `第 1/${steps} 步：登录读取、分配节点名`]);
  assert.deepEqual([of("named").done, of("named").text], [1, `第 2/${steps} 步：登录 x86 预检`]);
  assert.deepEqual([of("pushing").done, of("pushing").text], [3, `第 4/${steps} 步：发送安装包`]);
  assert.deepEqual([of("joined").done, of("joined").text], [steps, "已加入，节点 s1u-05-x86"]);
  assert.equal(of("gone").text, "Hub 连不到这台机器，上次看到是 11-18 11:53；RLark 每 5 分钟再试");
  assert.equal(of("waiting long").text, "场地的 Hub 不应答，上次看到这台机器是 11-17 09:00；RLark 自动再试，间隔逐渐拉长到约 16 分钟");
  assert.equal(of("blocked").done, 4);
  assert.match(of("blocked").text, /^停在第 5\/9 步「安装并入网」：入网停住了：the installer failed。勾选它再点加入/);
});

test("a machine no hub has seen lately says so while it waits", () => {
  const progress = machineJoinProgress([machine("quiet", { last_seen: at(18, 11, 53) })], "robots", now, true);
  assert.equal(progress.rows[0].text, "第 1/9 步：登录读取、分配节点名（Hub 已 2 天没看到）");
  assert.equal(machineJoinProgress([machine("quiet", { last_seen: at(18, 11, 53) })], "robots", now, false).rows[0].text,
    "Step 1/9: Read and named (not seen by a hub for 2 d)");
});

test("a sighting shows its date when it was not today, and how long ago", () => {
  assert.equal(seenTime(at(20, 9, 5), now), "09:05:00");
  assert.equal(seenTime(at(18, 11, 53), now), "11-18 11:53");
  assert.equal(seenTime(new Date(2025, 11, 31, 23, 0).toISOString(), now), "2025-12-31 23:00");
  assert.equal(seenTime(undefined, now), "—");
  assert.equal(unseenFor(at(20, 15, 35), now, true), "Hub 已 25 分钟没看到");
  assert.equal(unseenFor(at(20, 5, 0), now, true), "Hub 已 11 小时没看到");
  assert.equal(unseenFor(undefined, now, true), "没有被看到过");
});

test("join credentials are grouped once per NodeClass and validated before admitting", async () => {
  const { joinCredentialGroups, joinCredentialProblem } = await import("../dist/test/astribot/lib/machineJoin.js");
  const access = {
    entry: { secret: "s1u-access", key: "password", configured: true },
    node: { secret: "s1u-access", key: "x86_password", configured: true },
  };
  const classes = { s1u: { access } };
  const groups = joinCredentialGroups([
    machine("a", { node_class: "s1u" }),
    machine("b", { node_class: "s1u" }),
    machine("c", { node_class: "x86" }),
  ], { ...classes, x86: { access } });
  assert.deepEqual(groups.map((group) => [group.name, group.machines.length]), [["s1u", 2], ["x86", 1]]);
  assert.equal(joinCredentialProblem(groups, {}, true), "");

  const missingNode = { s1u: { access: { ...access, node: { ...access.node, configured: false } } }, x86: { access } };
  assert.match(joinCredentialProblem(joinCredentialGroups([machine("a")], missingNode), {}, true), /x86 登录口令/);
  assert.equal(joinCredentialProblem(joinCredentialGroups([machine("a")], missingNode), { s1u: { password: "typed", sudo: "" } }, true), "");

  const missingEntry = { s1u: { access: { ...access, entry: { ...access.entry, configured: false } } } };
  assert.match(joinCredentialProblem(joinCredentialGroups([machine("a")], missingEntry), { s1u: { password: "typed", sudo: "" } }, true), /Orin 入口口令/);

  const noAccess = { s1u: {} };
  assert.match(joinCredentialProblem(joinCredentialGroups([machine("a")], noAccess), { s1u: { password: "typed", sudo: "" } }, true), /没有声明登录方式/);
  assert.match(joinCredentialProblem([], {}, false), /No machines/);
});
