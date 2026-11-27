"""The node tuner of the Addon s1u-robot, on every S1U robot x86. It makes the host follow its node profile and tells
the cluster the result, with no shell on the box:

- copies node_profile.sh and node_profile.env, which this image holds beside this file, to the host's /run/rlark/s1u
  and runs the script in the host's namespaces: install once (a new image, so a new profile, rolls this Pod), then
  apply and status every minute;
- publishes the result on its own Node: label rlark.io/node-profile-state = ok | drift | kubelet-restart-required |
  reboot-required, and annotation rlark.io/node-profile-report with the status lines that are not ok;
- does by itself, while nobody uses the robot, what would end a use of it (maintenance): the restart of the whole
  robot its profile needs (kernel command line, systemd's default affinity; S1U_NODE_REBOOT "idle", the Addon's
  default) or a person asks for, the restart of kubelet alone (a new kubelet configuration of the profile; the Pods
  keep running), the Orin container's switch of a payload's version, and the jump of a clock that is more than
  STEP_SECONDS off the site's time (the clock container's report, /run/rlark/s1u/clock.json: it jumps only while the
  driver is stopped, so a robot kept ready would otherwise make the seconds up at 500 ppm for hours), only while no Run
  holds the robot (its claim file). A robot is in use while a component marks
  it under /run/rlark/in-use: a Station that serves a Run, a session attached to its device layer. Powered motors and
  a running driver are no use: a robot is kept ready all the time (user 11-15), and one that waited for its motors to
  be off never restarted (11-16). While the robot is in use the maintenance waits, as long as that takes;
- for a maintenance it holds a lease on the host, /run/rlark/s1u/maintenance, which the robot's power service follows
  (device/power.py): while the lease is held no session is let onto the robot (its bring_up is refused with the
  lease's reason, which the Run shows), and the robot is lowered to what the maintenance needs, the desired state
  it is kept at untouched: off for a restart of the robot (the vendor's restart wants the driver stopped and the
  motors off), the driver stopped for kubelet's, nothing for a payload. The lease is asked for, the marks read again,
  and only then held: a session admitted meanwhile keeps the robot. It ends with the maintenance (a restart of the
  host empties /run), or LEASE_SECONDS after this container last renewed it, and the power service then brings the
  robot back to its desired state: a robot a person powered off stays off. A robot whose motors are off while its
  restart waits (its wave, its place, somebody using it) is not brought up meanwhile: a new robot is powered for the
  first time after its first restart. Not begun while the emergency stop is
  pressed or the hardware reports an error, nor with the API server away. Motors that lose their power let the arms
  fall, and the model's way to off lets them sink first (the driver stopped, the motors off S1U_DRIVER_STOP_SECONDS
  later: the power service's). Whether that may happen with nobody there is the site's to declare
  (S1U_NODE_POWER_OFF, the Addon's parameter powerOff). "unattended", the default: the tuner has the robot powered
  off by itself; "attended": a restart of a robot whose motors are on waits until a person has powered it off at
  RLark. A robot whose motors
  are off, a new one before its first restart among them, is restarted either way, and so is one a person asks
  to have restarted (below). Where the robot is with it is on its Node:
  label rlark.io/maintenance-state = none | waiting | queued | lowering | acting | failed, annotation
  rlark.io/maintenance {"kind": host | kubelet | payload | clock, "of": what it is done for, "state", "why", "since",
  "change": the image, "retry": when a failed one is begun again};
- each restart's attempt is recorded on the Node (annotation rlark.io/node-profile-restart) with what the restart
  changes, the boot's identifier or kubelet's start: changed and the rules still do not hold, it is not repeated for
  this profile; unchanged five minutes later, the restart did not happen and is tried again, three times, the lease
  held meanwhile. A robot that does not come down within LOWER_SECONDS, a payload not switched within
  PAYLOAD_SECONDS, a restart that did not happen or did not help: the maintenance failed and the lease is given up.
  A restart that failed so is not tried again for the same profile or request; the others are, FAILED_SECONDS
  later. With S1U_NODE_REBOOT "never" the label stays reboot-required or
  kubelet-restart-required until an operator restarts it;
- restarts the whole robot once on request: the Node's label rlark.io/restart-request with a value of the asker's
  choosing (through the robot's node object in RLark). A maintenance like the others, whatever S1U_NODE_REBOOT says
  (that parameter is about restarts nobody asked for); refused, with the reason in the report, while the node
  profile does not hold, since a restart brings back what is on the disk; the same value is not done twice.
  Annotation rlark.io/restart-request-state says what became of it, "done" with the time of the boot that followed;
- keeps the robots of a site from being down together (the Addon's parameters rollout "staged", its default, and
  rolloutParallel): a robot's wave is its Node's label rlark.io/rollout-wave, a whole number; without it the robot is
  of the last wave. A wave acts once every Ready robot of the earlier waves has been done with the same image for
  rolloutSeconds, at least one has, and none failed. Within that, no more robots hold a lease at once than
  rolloutParallel says (a number, or a share of the site's robots, at least one): a robot ready for its maintenance
  queues on its Node first and takes the lease when fewer robots hold one or queue before it; one that does not
  come back keeps its place, and while a robot's maintenance with this image failed the others do not begin theirs.
  With the API server away nothing of this is done. Where a robot is with the image, and whom it waits for, is on
  its Node: label rlark.io/rollout-state, annotation rlark.io/rollout. "paused" holds every robot, "all" is neither
  waves nor a bound. The Pods themselves are replaced by RLark's rollout of what it places (one robot, then a
  tenth at a time, robots nobody uses); what is held back here is the acting;
- publishes whether the whole node conforms to the S1U node's specification and its fingerprints (node_spec.py),
  and which body the box drives, as the vendor's file on the box says: annotations rlark.io/body-serial and
  s1u.rlark.io/body-parameters (the file's digest);
- tells the robot's device layer which robot this x86 is: the host's /run/rlark/s1u/robot.json, {"robot_identifier",
  "body"}, its Node's labels rlark.io/robot-id and rlark.io/robot-body (the admission record's), once the host's CPUs
  follow the profile (no restart of the host or of kubelet is due). The device container starts on it;
- publishes the robot's self-check, which its device layer runs by itself and leaves on the host
  (/run/rlark/s1u/self-check.json: the round without motor power after the robot joined, the full round at every
  session's attach): label rlark.io/self-check-state = ok | failed | pending (none yet), annotation
  rlark.io/self-check with the round, when, and each item that failed.

With S1U_NODE_WANTED "removed" (the Addon's parameter profile, before the Addon itself is uninstalled) it takes the
profile back instead (node_profile.sh uninstall), and the label says removed.

Standard library only (any image with python3, bash and nsenter). Usage: node_tuner.py [--once]"""
import calendar
import glob
import hashlib
import json
import os
import shutil
import ssl
import subprocess
import sys
import time
import urllib.request

import node_spec

HOST = "/host"  # the host's /run/rlark/s1u
SOURCE = os.path.dirname(os.path.abspath(__file__))
ON_HOST = "/run/rlark/s1u"
ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"
STATE, REPORT = "rlark.io/node-profile-state", "rlark.io/node-profile-report"
BOOT = "rlark.io/boot-report"
CATEGORY = "rlark.io/node-category"
RESTART = "rlark.io/node-profile-restart"
# A restart on request: the label is the request (set on the robot's node object in RLark, whose label sync brings it
# here), the annotation what became of it: {"profile": the request, "was": the boot it was asked in, "at", "attempts",
# "said", "done": when the host booted after it}.
REQUEST, REQUESTED = "rlark.io/restart-request", "rlark.io/restart-request-state"
# What can take a robot off line is done in waves (gate): the label is the robot's wave, a whole number, and a robot
# without it is of the last one; the annotation is where the robot is with the image it runs, {"change": the image,
# "state": done | acting | waiting | paused | failed, "since", "why"}, and the label repeats the state.
WAVE, ROLLOUT, ROLLOUT_STATE = "rlark.io/rollout-wave", "rlark.io/rollout", "rlark.io/rollout-state"
ATTEMPTS, RESTART_SECONDS = 3, 300  # tries of a restart that does not happen, and how long one may take
# A maintenance (the module's words): where the robot is with it on its Node, and what each kind lowers the robot to
# while its lease is held (the power service's desired states).
MAINTENANCE, MAINTENANCE_STATE = "rlark.io/maintenance", "rlark.io/maintenance-state"
CEILING = {"host": "off", "kubelet": "powered", "payload": "ready", "clock": "powered"}
STEP_SECONDS = 0.1  # clock/clock.py's: a clock this far off the site's is jumped, its driver stopped
CLOCK_SECONDS = 120  # the clock is jumped within this long of the driver's stop (it measures every 16 s)
LEASE_SECONDS = 600  # a lease this container has not renewed for this long is over: the tuner is gone
LOWER_SECONDS, LOWER_PASS_SECONDS = 300, 90  # the robot comes down within this long; waited for in one pass
PAYLOAD_SECONDS = 600  # the Orin container switches its payloads within this long
FAILED_SECONDS = 3600  # a maintenance that failed short of its restart is begun again this long after
POWER_STATE, ORIN_ALL = "rlark.io/power-state", "s1u.rlark.io/orin"  # the power service's and the Orin container's
SERIAL, PARAMETERS = "rlark.io/body-serial", "s1u.rlark.io/body-parameters"
ROBOT, BODY = "rlark.io/robot-id", "rlark.io/robot-body"
SELF_CHECK_STATE, SELF_CHECK = "rlark.io/self-check-state", "rlark.io/self-check"


def host(*arguments, quiet=False):
    """node_profile.sh ``arguments`` in the host's namespaces: (exit code, output), the output in this Pod's log
    too unless ``quiet``."""
    result = subprocess.run(
        ["nsenter", "--target", "1", "--mount", "--uts", "--ipc", "--net", "--",
         "bash", f"{ON_HOST}/node_profile.sh", *arguments],
        stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True)
    if not quiet:
        print(result.stdout, end="", flush=True)
    return result.returncode, result.stdout


def state(status):
    """(label value, report) from the script's status lines: reboot-required when a rule holds only after a
    restart of the host, kubelet-restart-required when a restart of kubelet is enough, drift when another fails, with those rules as the report."""
    failed = [line for line in status.splitlines() if line.startswith(("REBOOT", "KUBELET", "DRIFT"))]
    report = "; ".join(line.split(None, 1)[1] for line in failed)
    if any(line.startswith("REBOOT") for line in failed):
        return "reboot-required", report
    if any(line.startswith("KUBELET") for line in failed):
        return "kubelet-restart-required", report
    return ("drift", report) if failed else ("ok", "")


def publish(value, report, labels={}, **annotations):
    """The state on this Pod's Node, through the API server with the Pod's service account, with how long the
    host's last boot took (boot_report.py: seconds from the power button to each milestone)."""
    boot = json.load(open(f"{HOST}/boot_report.json")) if os.path.exists(f"{HOST}/boot_report.json") else {}
    request = urllib.request.Request(
        "https://%s:%s/api/v1/nodes/%s" % (os.environ["KUBERNETES_SERVICE_HOST"],
                                            os.environ["KUBERNETES_SERVICE_PORT"], os.environ["NODE_NAME"]),
        data=json.dumps({"metadata": {"labels": dict(labels, **{STATE: value}), "annotations": {
            REPORT: report[:1000], BOOT: json.dumps(boot.get("seconds_from_power_button", {})), **annotations}}}).encode(),
        method="PATCH",
        headers={"Authorization": "Bearer " + open(f"{ACCOUNT}/token").read(),
                 "Content-Type": "application/merge-patch+json"})
    urllib.request.urlopen(request, context=ssl.create_default_context(cafile=f"{ACCOUNT}/ca.crt"), timeout=10)


def node():
    """This Node's (labels, annotations)."""
    request = urllib.request.Request(
        "https://%s:%s/api/v1/nodes/%s" % (os.environ["KUBERNETES_SERVICE_HOST"],
                                            os.environ["KUBERNETES_SERVICE_PORT"], os.environ["NODE_NAME"]),
        headers={"Authorization": "Bearer " + open(f"{ACCOUNT}/token").read()})
    context = ssl.create_default_context(cafile=f"{ACCOUNT}/ca.crt")
    with urllib.request.urlopen(request, context=context, timeout=10) as reply:
        metadata = json.load(reply)["metadata"]
    return metadata.get("labels", {}), metadata.get("annotations", {})


def identify(labels, value):
    """Which robot this x86 is, for its device layer (the host's /run/rlark/s1u/robot.json), from the Node's
    ``labels``; not while the node profile's state ``value`` says the host's CPUs do not follow the profile yet."""
    if value in ("reboot-required", "kubelet-restart-required") or ROBOT not in labels or BODY not in labels:
        return
    text = json.dumps(dict(robot_identifier=labels[ROBOT], body=labels[BODY])) + "\n"
    if not os.path.exists(f"{HOST}/robot.json") or open(f"{HOST}/robot.json").read() != text:
        with open(f"{HOST}/robot.json.new", "w") as told:
            told.write(text)
        os.replace(f"{HOST}/robot.json.new", f"{HOST}/robot.json")


def self_check():
    """(label value, annotation) of the robot's self-check as its device layer left it on the host."""
    try:
        result = json.load(open(f"{HOST}/self-check.json"))
    except (OSError, ValueError):  # none yet (a robot that just joined, or booted), or being written
        return "pending", "{}"
    failed = [item for item in result["items"] if not item["ok"]]
    # At most 2000 characters, and JSON: what is cut is the failed items' details, each to the same length at most.
    for cut in range(2000, -1, -10):
        text = json.dumps(dict(round=result["round"], checked_at=result["checked_at"], failed=[
            dict(name=item["name"], detail=item["detail"][:cut]) for item in failed]), ensure_ascii=False)
        if len(text) <= 2000:
            return result["state"], text


def robots():
    """Every robot's Node of the cluster as gate and place read it: name, wave, whether it is Ready and since when
    (seconds), and its rollout and maintenance annotations."""
    request = urllib.request.Request(
        "https://%s:%s/api/v1/nodes?labelSelector=%s%%3Drobot" % (
            os.environ["KUBERNETES_SERVICE_HOST"], os.environ["KUBERNETES_SERVICE_PORT"], CATEGORY),
        headers={"Authorization": "Bearer " + open(f"{ACCOUNT}/token").read()})
    context = ssl.create_default_context(cafile=f"{ACCOUNT}/ca.crt")
    with urllib.request.urlopen(request, context=context, timeout=10) as reply:
        items = json.load(reply)["items"]
    found = []
    for item in items:
        ready = next(condition for condition in item["status"]["conditions"] if condition["type"] == "Ready")
        found.append(dict(
            name=item["metadata"]["name"], wave=item["metadata"].get("labels", {}).get(WAVE),
            ready=ready["status"] == "True",
            since=calendar.timegm(time.strptime(ready["lastTransitionTime"], "%Y-%m-%dT%H:%M:%SZ")),
            rollout=json.loads(item["metadata"].get("annotations", {}).get(ROLLOUT, "{}")),
            maintenance=json.loads(item["metadata"].get("annotations", {}).get(MAINTENANCE, "{}"))))
    return found


def gate(wave, change, others, seconds, now):
    """Why a robot of ``wave`` (its label, None for the last wave) may not yet do, for the image ``change``, what
    can take it off line; "" when it may. It waits for the earlier waves: every robot of them that is Ready has
    been done with this image for ``seconds``, at least one has, and none failed. A robot that began to act and has
    not been Ready since for ``seconds`` failed: it cannot say so itself."""
    earlier = [robot for robot in others if robot["wave"] is not None and (wave is None or int(robot["wave"]) < int(wave))]
    if not earlier:
        return ""
    done = 0
    for robot in earlier:
        rollout = robot["rollout"] if robot["rollout"].get("change") == change else {}
        state = rollout.get("state")
        if state == "failed" or state == "acting" and not robot["ready"] and now - robot["since"] > seconds:
            return "wave %s: %s failed with this image (%s)" % (robot["wave"], robot["name"], rollout.get("why") or (
                "not Ready since it began"))
        if state == "done" and now - rollout["since"] >= seconds:
            done += 1
        elif robot["ready"]:
            return "wave %s: %s is not done with this image for %d s yet (%s)" % (
                robot["wave"], robot["name"], seconds, state or "not begun")
    return "" if done else "no robot of an earlier wave is Ready and done with this image"


def place(name, queued, others, parallel, change):
    """Why the robot ``name``, ready for its maintenance and queued on its Node since ``queued`` (None: not yet), may
    not take its lease now; "" when it may. No more robots than ``parallel`` (a number, or "N%" of the site's robots,
    at least one) hold a lease at once, in the order they queued; a robot that holds one and is gone keeps its place.
    While a robot's maintenance with the image ``change`` failed, nobody begins another."""
    for robot in others:
        if robot["maintenance"].get("state") == "failed" and robot["maintenance"].get("change") == change:
            return "%s failed its maintenance with this image (%s)" % (robot["name"], robot["maintenance"].get("why"))
    if queued is None:
        return "queued"
    bound = (max(1, -(-(len(others) + 1) * int(parallel[:-1]) // 100)) if parallel.endswith("%") else int(parallel))
    before = sorted(robot["name"] for robot in others if robot["maintenance"].get("state") in ("lowering", "acting") or (
        robot["maintenance"].get("state") == "queued" and (robot["maintenance"]["since"], robot["name"]) < (queued, name)))
    return "" if len(before) < bound else "%d of the site's robots at a time; before it: %s" % (bound, ", ".join(before[:5]))


def lease(**held):
    """The tuner's word to the robot's power service, on the host: the lease of a maintenance (kind, of, power: what
    the robot is lowered to, why, held: whether it has been taken, since), or that there is none."""
    with open(f"{HOST}/maintenance.new", "w") as told:
        told.write(json.dumps(dict(dict(kind=""), **held, until=time.time() + LEASE_SECONDS)) + "\n")
    os.replace(f"{HOST}/maintenance.new", f"{HOST}/maintenance")


def lowered(kind, sleep=time.sleep):
    """Why the robot is not down yet for a restart of ``kind`` after LOWER_PASS_SECONDS, "" once it is: the power
    service stops the driver and powers the motors off a few seconds after it reads the lease."""
    until = time.monotonic() + LOWER_PASS_SECONDS
    while True:
        up = host("up", kind, quiet=True)[1].strip()
        if not up or time.monotonic() >= until:
            return up
        sleep(5)


def running(kind):
    """What a restart of ``kind`` changes: the boot's identifier (host), kubelet's start in clock ticks since boot
    (kubelet; this Pod is in the host's PID namespace)."""
    if kind == "host":
        return open("/proc/sys/kernel/random/boot_id").read().strip()
    for path in glob.glob("/proc/[0-9]*/comm"):
        try:
            if open(path).read().strip() == "kubelet":
                return open(os.path.dirname(path) + "/stat").read().rsplit(")", 1)[1].split()[19]
        except OSError:  # it ended meanwhile
            pass
    return None


def restart(kind, record, profile, current, now):
    """Whether the restart of ``kind`` this ``profile`` needs is tried now: (the record to keep on the Node, None)
    when it is, else (None, why not). ``record`` is the last attempt's, ``current`` what running(kind) reads now."""
    if record.get("kind") != kind or record.get("profile") != profile:
        record = dict(kind=kind, profile=profile, attempts=0)
    elif record["was"] != current:
        return None, "it was restarted for this profile and the rules still do not hold"
    elif now - record["at"] < RESTART_SECONDS:
        return None, "its restart was asked for %d s ago" % (now - record["at"])
    elif record["attempts"] >= ATTEMPTS:
        return None, "%d attempts did not restart it; the last said: %s" % (record["attempts"], record.get("said"))
    return dict(record, was=current, at=now, attempts=record["attempts"] + 1), None


def follow(profile, source):
    """One pass against the API server: the state on the Node, and one step of the maintenance the robot needs.
    Whether the next pass should come at once (the robot queued for its lease just now)."""
    value, report = state(host("status")[1])
    labels, annotations = node()
    identify(labels, value)
    name, mode, change, now = os.environ["NODE_NAME"], os.environ["S1U_NODE_ROLLOUT"], os.environ["S1U_IMAGE"], time.time()
    others = [other for other in robots() if other["name"] != name] if mode == "staged" else []
    # Whether this robot's wave may act yet (the Addon's parameter rollout: staged, paused, or all at once).
    wave = {"all": "", "paused": "the Addon's parameter rollout is paused"}.get(mode)
    if wave is None:
        wave = gate(labels.get(WAVE), change, others, int(os.environ["S1U_NODE_ROLLOUT_SECONDS"]), now)
    # The one maintenance the robot needs now: the restart its profile needs, the restart a person asks for (the
    # Node's label REQUEST, any value of theirs as the request's number; only of a node whose profile holds, because a
    # restart shows what the disk says: 11-14, s1u_02 with its uplink moved on the running system only would have
    # come back on the earlier one), the Orin container's switch of a payload it has placed.
    records = {RESTART: json.loads(annotations.get(RESTART, "{}")), REQUESTED: json.loads(annotations.get(REQUESTED, "{}"))}
    request, asked = labels.get(REQUEST), records[REQUESTED]
    needed = {"reboot-required": "host", "kubelet-restart-required": "kubelet"}.get(value)
    payloads = json.loads(annotations.get(ORIN_ALL, "{}")).get("payloads", {})
    switching = sorted("%s %s" % (payload, versions["wanted"]) for payload, versions in payloads.items()
                       if versions.get("running") not in (None, versions["wanted"]))
    work = None
    if needed and os.environ["S1U_NODE_REBOOT"] == "idle":
        work = dict(kind=needed, of="%s %s" % (needed, profile), why="restarting %s for its node profile" % (
            "the robot" if needed == "host" else "kubelet"), record=RESTART, key=profile)
    elif request and asked.get("profile") == request and asked["was"] != running("host"):
        if "done" not in asked:
            booted = next(int(line.split()[1]) for line in open("/proc/stat") if line.startswith("btime"))
            records[REQUESTED] = asked = dict(asked, done=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime(booted)))
    elif request and value != "ok":
        report = f"restart request {request} refused, the node profile does not hold; {report}"
    elif request:
        work = dict(kind="host", of="request %s" % request, why="restarting the robot on request %s" % request,
                    record=REQUESTED, key=request)
    elif switching:
        work = dict(kind="payload", of="payload %s" % ", ".join(switching), why="switching the Orin to %s" % ", ".join(switching))
    elif source and os.path.exists(f"{HOST}/clock.json"):
        clocked = json.load(open(f"{HOST}/clock.json"))
        if now - clocked["at"] < 60 and abs(clocked["offset_ms"]) > STEP_SECONDS * 1e3:
            work = dict(kind="clock", of="clock", why="jumping the clock %+.0f ms to the site's time" % -clocked["offset_ms"])
    before, held = json.loads(annotations.get(MAINTENANCE, "{}")), {}
    if os.path.exists(f"{HOST}/maintenance"):
        held = json.load(open(f"{HOST}/maintenance"))
    where, why, attempt, again, retry = "none", "", None, False, None
    if work:
        mine = before if before.get("of") == work["of"] else {}
        holds = held.get("of") == work["of"] and held.get("held")
        if "record" in work:
            # Whether this restart is still to be tried: one that happened and did not help, or did not happen three
            # times, is not, and the robot is not lowered for it.
            attempt, said = restart(work["kind"], records[work["record"]], work["key"], running(work["kind"]), now)
            if not attempt and not said.startswith("its restart was asked"):
                where, why = "failed", said
        if where != "failed" and mine.get("state") == "failed" and now < mine.get("retry", 0):
            where, why, retry = "failed", mine["why"], mine["retry"]
        if where != "failed" and not holds:
            # Not begun: its wave (a person's request has none), the emergency stop or an error, somebody using the
            # robot, motors nobody is there to see go off, its place among the site's robots.
            marked = "; ".join(host("marked", quiet=True)[1].splitlines())
            claim = json.load(open(f"{HOST}/claim"))["holder"] if os.path.exists(f"{HOST}/claim") else ""
            why = ("" if work["of"].startswith("request") or work["kind"] == "clock" else wave) or (
                "Run %s holds the robot" % claim if work["kind"] == "clock" and claim else "") or (
                "the robot is in %s" % labels[POWER_STATE] if labels.get(POWER_STATE) in ("estop", "fault") else "") or (
                "in use (%s)" % marked if marked else "") or (
                "its motors are on and powering them off is a person's (the Addon's parameter powerOff): power the "
                "robot off on its node page" if work["of"].startswith("host") and labels.get(POWER_STATE) in ("powered", "ready")
                and os.environ["S1U_NODE_POWER_OFF"] == "attended" else "")
            where = "waiting"
            if not why:
                queued = mine.get("since") if mine.get("state") == "queued" else None
                why = "" if mode == "all" else place(name, queued, others, os.environ["S1U_NODE_ROLLOUT_PARALLEL"], change)
                where, again = "queued", queued is None and mode != "all"
            if not why:
                # Asked for, the marks read again, then held: a session let onto the robot before the power service
                # read the lease has its mark by now, and keeps the robot.
                terms = dict(kind=work["kind"], of=work["of"], power=CEILING[work["kind"]], why=work["why"], since=now)
                lease(**terms, held=False)
                marked = "; ".join(host("marked", quiet=True)[1].splitlines())
                if marked:
                    where, why = "waiting", "in use (%s)" % marked
                else:
                    lease(**terms, held=True)
                    held, holds = dict(terms, held=True), True
        if holds and where != "failed":
            lease(**{key: held[key] for key in ("kind", "of", "power", "why", "since")}, held=True)
            where, why = "acting", work["why"]
            if work["kind"] == "payload":
                if now - held["since"] > PAYLOAD_SECONDS:
                    where, why, retry = "failed", "the Orin did not switch within %d s" % PAYLOAD_SECONDS, now + FAILED_SECONDS
            else:
                up = lowered(work["kind"])
                if up and now - held["since"] > LOWER_SECONDS:
                    where, why, retry = "failed", "the robot did not come down within %d s: %s" % (LOWER_SECONDS, up), now + FAILED_SECONDS
                elif up:
                    where, why = "lowering", up
                elif work["kind"] == "clock" and now - held["since"] > CLOCK_SECONDS:
                    where, why, retry = "failed", "the clock was not jumped within %d s of the driver's stop" % CLOCK_SECONDS, now + FAILED_SECONDS
        if where != "acting":
            attempt = None  # the restart is asked for under a held lease, of a robot that is down
    if work and work["kind"] == "host" and where in ("waiting", "queued") and labels.get(POWER_STATE) not in ("powered", "ready"):
        # A restart of the robot is due and the robot is not up: it is not brought up meanwhile, only to be lowered
        # again for the restart (11-16, staging: a new robot queued behind another for its first restart was powered
        # and its driver started). The lease is asked for and not held: nothing is lowered, nothing raised.
        lease(kind=work["kind"], of=work["of"], power=CEILING[work["kind"]], why="%s: %s" % (work["why"], why), since=now, held=False)
    elif where in ("none", "waiting", "failed") or where == "queued" and not again:
        lease()  # no maintenance holds the robot: its power service keeps it at its desired state
    with open(f"{HOST}/rollout", "w") as told:  # for the Orin's container: its switch of a payload's version
        told.write("allowed\n" if where == "acting" and work["kind"] == "payload" else "waiting: %s\n" % (
            why if work and work["kind"] == "payload" else "the node tuner's maintenance"))
    if work:
        report = "%s: %s%s; %s" % (work["why"], where, " (%s)" % why if why and why != work["why"] else "", report)
    maintenance = dict(kind=work["kind"], of=work["of"], state=where, why=why, change=change, since=(
        before["since"] if (before.get("of"), before.get("state")) == (work["of"], where) else now),
        **(dict(retry=retry) if retry else {})) if work else {}
    # Where this robot is with the image, for the later waves and for people.
    rolled = json.loads(annotations.get(ROLLOUT, "{}"))
    stage = ("done" if value == "ok" and labels.get(node_spec.ORIN) == "ok" and where == "none" else
             "failed" if where == "failed" else "paused" if mode == "paused" else "waiting" if wave else "acting")
    rollout = dict(change=change, state=stage, why="" if stage == "done" else why if stage == "failed" else wave, since=(
        rolled["since"] if (rolled.get("change"), rolled.get("state")) == (change, stage) else now))
    # The whole node against its specification (node_spec.py), and which body the box drives.
    facts = node_spec.parse(host("facts", quiet=True)[1])
    failing, marks = node_spec.conformance(name, facts, labels, annotations, value, source)
    identity = dict(marks, **{
        SERIAL: facts["record"]["body serial"], PARAMETERS: facts["record"]["body parameters"],
        node_spec.SPEC: profile, node_spec.FACTS: json.dumps(facts, sort_keys=True),
        node_spec.REPORT: "; ".join(failing)[:2000]})
    checked, identity[SELF_CHECK] = self_check()
    conforms = {node_spec.CONFORMANCE: "not-ok" if failing else "ok", ROLLOUT_STATE: stage, SELF_CHECK_STATE: checked,
                MAINTENANCE_STATE: where}
    identity[ROLLOUT], identity[MAINTENANCE] = json.dumps(rollout), json.dumps(maintenance)
    if asked:
        identity[REQUESTED] = json.dumps(asked)
    # The attempt is on the Node before it is made: a restart of the host ends this Pod.
    publish(value, report, conforms, **dict(identity, **({work["record"]: json.dumps(attempt)} if attempt else {})))
    if attempt:
        # What the step said stays with the attempt: the reason of one that did not restart anything.
        said = host("reboot" if work["kind"] == "host" else "restart-kubelet")[1]
        publish(value, report, conforms, **dict(identity, **{work["record"]: json.dumps(dict(attempt, said=said[-300:]))}))
    return again


def main():
    for name in ("node_profile.sh", "node_profile.env", "boot_report.py", "bodies.json"):
        shutil.copy(f"{SOURCE}/{name}", f"{HOST}/{name}")
    if node()[0].get(CATEGORY) != "robot":
        # The profile rewrites a host's kernel command line, CPU affinities and kubelet: only a robot's (11-13: the
        # robot cluster's control plane is a workstation, and a selector by cluster would have reached it).
        print("this node is not a robot (%s): nothing is installed" % CATEGORY, flush=True)
        while True:
            time.sleep(3600)
    if os.environ["S1U_NODE_WANTED"] == "removed":
        code, removal = host("uninstall")
        publish("drift" if code else "removed", removal[-900:] if code else
                "the host is as before the node profile from its next boot")
        if code:
            raise SystemExit(code)
        while True:
            time.sleep(3600)
    code, install = host("install", f"{ON_HOST}/node_profile.env")
    if code:
        publish("drift", "install failed: " + install[-900:])
        raise SystemExit(code)
    profile = hashlib.sha256(b"".join(open(f"{SOURCE}/{name}", "rb").read() for name in (
        "node_profile.sh", "node_profile.env", "node_spec.py"))).hexdigest()[:16]
    # The Addon's timeSource: declared, the clock must be locked to it. An empty one comes as no variable at all (RLark
    # renders none; Fleet's v0.11.6 exited on that, 11-14).
    source = os.environ.get("S1U_SITE_TIME_SOURCE", "")
    # Off the SDK core and the device layer's CPUs, which kubelet's pool for Pods holds too: what this container runs
    # on the host every minute stays with it.
    kubernetes = next(line.split("=", 1)[1].strip() for line in open(f"{SOURCE}/node_profile.env")
                      if line.startswith("S1U_KUBERNETES_CPUS="))
    os.sched_setaffinity(0, {cpu for part in kubernetes.split(",")
                             for cpu in range(int(part.split("-")[0]), int(part.split("-")[-1]) + 1)})
    while True:
        host("apply")  # what needs no API server: a robot cut off from the site still follows its profile
        again = False
        try:
            again = follow(profile, source)
        except OSError as error:  # the API server is away (s1u-02-x86 11-13: its service address timed out for hours)
            print("the Node is not reached, again in a minute: %s" % error, flush=True)
        if "--once" in sys.argv:
            return
        time.sleep(10 if again else 60)

if __name__ == "__main__":
    main()
