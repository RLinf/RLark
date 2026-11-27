"""Each S1U robot's x86 keeps its own Orin (user 11-12/13: nobody logs into an Orin, nothing is done per machine): a
container of the node tuner's Pod (Addon s1u-robot), on every S1U robot x86. Every minute it logs into the robot's
Orin over the robot's internal network (S1U_ORIN_IP_ADDRESS, the same on every robot) and makes it what the Addon
declares:

- the x86's uplink through the Orin (s1u-orin-uplink.service: forwarding and masquerade for the x86's address, there
  from the Orin's boot), as this image has it; the Orin's uplink addresses written for the node profile's rules
  (none is the x86's, none is of the robot's own network). Units the Addon retires (orinRetiredUnits: the same
  uplink under an earlier name) are stopped and removed, and this one started again at once, since stopping such a
  unit deletes the forwarding rules both of them hold;
- the cameras' USB rule (the devices of the cameras' vendors the account's), and the cameras for this Pod's camera
  service (device/cameras.py) as the Orin's account, through the directory the two share (/camera). Each camera is
  found by its serial number in the Orin's /etc/config/camera_info.json (the vendor's record of which camera is
  which: name -> serial). Every minute, where each camera is on the USB (usb: {name: {path, device, speed, hubs}}).
  On request ({"id", "cameras": the vendor's names}), each id done once, also across a restart of this container, the
  result beside it: reset-request, when it restarted the vendor's camera driver and still had no frames, the cameras'
  USB devices reset (reset-result {"id", "ok", "detail", "reset", "missing", "finished_at"}: the names reset and
  those without a USB device, both left out when the reset did not run); diagnose-request, when it gave a camera up,
  what the Orin shows of each (diagnose-result {"id", "cameras": {name: diagnosis}, "finished_at"}): on the USB or
  not and where, whether the vendor's camera process runs, and for an Orbbec camera the SDK opens (usbfs) whether its
  log has the colour and depth streams (s1u-03 11-21: colour stopped, depth went on, no USB error);
- a vendor process that must not run there (the profile's S1U_ORIN_STOPPED) ended at every pass, and the line that
  starts it in the vendor's start script (S1U_ORIN_STOPPED_LAUNCHER) commented out;
- the site's adb key the headsets trust (the secret's adb_key, if it holds one) as ~/.android/adbkey_rlark;
- the payloads the Addon declares (orinPayloads, "NAME=IMAGE,..."): each image's /payload directory, copied into
  this Pod by an init container, as ~/rl/NAME.<hash of its files> on the Orin with ~/rl/NAME a link to it (the hash
  is the version). A payload's own script ``install`` places what only root can (its units, its USB rules) and
  starts it: run through sudo as ``install`` after a new version is unpacked, ``install restart`` when the link is
  switched to it, ``install remove`` when the payload is no longer declared. It finds the robot's addresses and the
  account in rlark.env beside it. A new version is switched to only when the node tuner says so
  (/run/rlark/s1u/rollout): a restart of a payload may end what it serves, so the tuner holds the robot's maintenance
  lease for it, taken while nobody uses the robot, in its wave and its place among the site's robots (node_tuner.py).
RLark depends on a payload's image reference only, never on what it is made of.

The login is one site-level set in the Addon's secret (orinAccessSecret), mounted in memory at /orin-access:
ssh_key (the x86's key to the Orin), password (the Orin account's, also sudo's) and, optionally, adb_key. A new Orin
does not have the key yet: the first login is by password (ssh's own SSH_ASKPASS, whose helper reads the mounted
file) and adds the key's public line to authorized_keys. The password is written to sudo's standard input and is in
no argument, environment variable, log or file of this container's making.

The Orin's ssh host key is trusted on first use and kept on this robot's Node (annotation s1u.rlark.io/orin-host-key,
written here, kept by the cluster across a reinstalled x86). Once it differs, nothing more is sent to that address,
the state says identity_changed with both fingerprints, and a person confirms the new one by setting the Node's
annotation s1u.rlark.io/orin-host-key-confirm to the presented fingerprint (through RLark: whoever may write the
robot's Node may confirm). The key RLark gave the Orin when the robot joined (a NodeClass with rotateHostKey; the
join writes its fingerprint to the Node's annotation rlark.io/entry-host-key before the Orin shows it) needs no person:
it is trusted as "rotated by RLark". Every trust is recorded: the annotation s1u.rlark.io/orin-host-key-trust and an
Event.

The state is on the Node: label s1u.rlark.io/orin-state, one word: ok | updating | no_access (the secret is not in the
cluster yet) | unreachable | login_failed | identity_changed | failed; with the Addon's profile=removed, removing (the
host is marked in use) | removed (remove: nothing of the Addon's is left there); annotation s1u.rlark.io/orin, the
whole state as JSON (reachable, identity, trust, fingerprints, serial, payloads, the last camera reset and diagnosis,
failures). The camera service tells the Node of each reset and diagnosis (Events CameraUsbReset, CameraStuck).

Standard library and the image's ssh only."""
import hashlib
import io
import json
import os
import ssl
import subprocess
import sys
import tarfile
import tempfile
import threading
import time
import urllib.request
from pathlib import Path

HERE = Path(__file__).resolve().parent
ACCESS = "/orin-access"
PAYLOADS = Path("/payloads")  # one directory per declared payload, filled by the init containers
IN_USE = Path("/in-use")  # the host's /run/rlark/in-use
# The trusted host key and serial as the Node last had them (public), on the robot's disk in a directory of root's:
# this container runs as root without any capability and writes only what root owns (11-14: the robot's data
# directory is the vendor account's, the write was refused and the pass ended there).
KEPT = "/state/orin_trust.json"
# The robot's cameras' USB vendors (Orbbec: head and torso; Intel: the wrists' RealSense), whose devices the account
# may reset; how often the camera service's request is looked for, and where (the Pod's shared directory).
CAMERA_VENDORS, WATCH_SECONDS, CAMERAS = ("2bc5", "8086"), 5, "/camera"
# Run on the Orin as its account with the vendor's names of the cameras to reset: each one's serial number from the
# vendor's /etc/config/camera_info.json (camera name -> serial), the USB device of that serial by its sysfs entry,
# reset through usbfs (USBDEVFS_RESET); prints "NAME bus BBB dev DDD" for each device reset.
USB_RESET = ("import fcntl, glob, json, os, sys\n"
             "info = json.load(open(\"/etc/config/camera_info.json\"))\n"
             "for name in sys.argv[1:]:\n"
             "    for d in glob.glob(\"/sys/bus/usb/devices/*\"):\n"
             "        s = os.path.join(d, \"serial\")\n"
             "        if os.path.exists(s) and open(s).read().strip() == info.get(name):\n"
             "            r = lambda n: int(open(os.path.join(d, n)).read())\n"
             "            with open(\"/dev/bus/usb/%03d/%03d\" % (r(\"busnum\"), r(\"devnum\")), \"wb\") as f:\n"
             "                fcntl.ioctl(f, 0x5514)\n"
             "            print(\"%s bus %03d dev %03d\" % (name, r(\"busnum\"), r(\"devnum\")))\n")
# Run on the Orin as its account (python3 -, the script on standard input): each camera of the vendor's
# /etc/config/camera_info.json on the USB by its serial (its sysfs entry: path, device number, speed, vendor, the hubs
# from the root port down, its first interface's driver), whether the vendor's camera process runs, and in the Orbbec
# SDK's log (None without one) the seconds since its last COLOR and DEPTH stream lines and the frame timeouts after the
# last COLOR one; one line of JSON. The log's lines begin "[MM/DD HH:MM:SS.ffffff]", without the year.
CAMERA_FACTS = r"""import json, os, re, subprocess, time
USB = "/sys/bus/usb/devices"
names = {serial: name for name, serial in json.load(open("/etc/config/camera_info.json")).items()}
read = lambda device, key: open(os.path.join(USB, device, key)).read().strip()
cameras = {}
for device in os.listdir(USB):
    if os.path.exists(os.path.join(USB, device, "serial")) and read(device, "serial") in names:
        parts, driver = device.split("."), os.path.join(USB, device + ":1.0", "driver")
        cameras[names[read(device, "serial")]] = dict(
            path=device, device=int(read(device, "devnum")), speed=int(read(device, "speed")), vendor=read(device, "idVendor"),
            hubs=["%s:%s" % (read(hub, "idVendor"), read(hub, "idProduct")) for hub in (".".join(parts[:i]) for i in range(1, len(parts)))],
            driver=os.path.basename(os.readlink(driver)) if os.path.exists(driver) else None)
sdk, log = None, os.path.expanduser("~/Log/OrbbecSDK.log.txt")
if os.path.exists(log):
    sdk, now = dict(color=None, depth=None, timeouts=0), time.time()
    with open(log, "rb") as file:
        start = max(0, os.path.getsize(log) - 400000)
        file.seek(start)
        lines = file.read().decode(errors="replace").splitlines()[1 if start else 0:]  # from the first whole line
    for line in reversed(lines):
        stamp = re.match(r"\[(\d\d/\d\d \d\d:\d\d:\d\d)", line)
        if not stamp:
            continue
        at = lambda year: time.mktime(time.strptime("%d/%s" % (year, stamp[1]), "%Y/%m/%d %H:%M:%S"))
        for stream in ("color", "depth"):
            if sdk[stream] is None and "OB_SENSOR_%s Streaming" % stream.upper() in line:
                year = time.localtime(now).tm_year
                sdk[stream] = round(now - (at(year) if at(year) <= now else at(year - 1)))  # 31 December's on 1 January
        sdk["timeouts"] += sdk["color"] is None and "Wait for frame timeout" in line
process = subprocess.run(["pgrep", "-f", "astribot_camera_main"], stdout=subprocess.DEVNULL).returncode == 0
print(json.dumps(dict(cameras=cameras, process=process, sdk=sdk)))
"""
CAMERA_RULE = '/etc/udev/rules.d/70-s1u-camera.rules'
HOST = "/host"  # the host's /run/rlark/s1u, the node profile's state
ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"
HOST_KEY, TRUST, CONFIRM = ("s1u.rlark.io/orin-host-key", "s1u.rlark.io/orin-host-key-trust",
                            "s1u.rlark.io/orin-host-key-confirm")
# The fingerprint of the host key RLark gave the Orin when the robot joined (EntryHostKeyAnnotation of extensions/station).
ROTATED = "rlark.io/entry-host-key"
STATE, STATE_ALL, SERIAL = "s1u.rlark.io/orin-state", "s1u.rlark.io/orin", "s1u.rlark.io/orin-serial"
PERIOD_SECONDS, LONGEST_SECONDS = 60, 900
UPLINK = "s1u-orin-uplink.service"


def uplink_setup():
    """The shell script that places the x86's uplink on an Orin, made of the unit and nothing else: what each pass
    runs, and what RLark hands to a new robot's Orin when it first joins (the S1U NodeClass's uplink.setup, the Addon's
    bootstrap/orin-uplink.sh, which ``orin.py --uplink-setup`` prints). Run as root, any number of times."""
    unit = (HERE / UPLINK).read_text()
    return """#!/bin/sh
# s1u-orin-uplink %s: generated from extensions/s1u/orin/%s (orin.py --uplink-setup).
set -eu
unit=/etc/systemd/system/%s
new=$(mktemp)
cat > "$new" <<'S1U_ORIN_UPLINK'
%sS1U_ORIN_UPLINK
if ! cmp -s "$new" "$unit"; then
  install -m 644 "$new" "$unit"
  systemctl daemon-reload
  systemctl restart %s
fi
rm -f "$new"
systemctl enable --quiet %s
# Stopped, or forwarding switched off since: started again (its rules are added only where missing).
[ "$(systemctl is-active %s)" = active ] && [ "$(cat /proc/sys/net/ipv4/ip_forward)" = 1 ] \\
  || systemctl restart %s
""" % (hashlib.sha256(unit.encode()).hexdigest()[:16], UPLINK, UPLINK, unit, UPLINK, UPLINK, UPLINK, UPLINK)


def payload(directory, environment):
    """(version, tar.gz) of a payload directory with rlark.env, the robot's addresses for its install script. The
    version is the hash of the names and contents of the payload's own files."""
    files = {str(path.relative_to(directory)): path.read_bytes()
             for path in sorted(directory.rglob("*")) if path.is_file()}
    digest = hashlib.sha256()
    for name in sorted(files):
        digest.update(name.encode() + b"\0" + files[name] + b"\0")
    version = digest.hexdigest()[:16]
    files["rlark.env"] = "".join("%s=%s\n" % item for item in environment.items()).encode()
    files["VERSION"] = version.encode() + b"\n"
    archive = io.BytesIO()
    with tarfile.open(fileobj=archive, mode="w:gz") as tar:
        for name, content in files.items():
            member = tarfile.TarInfo(name)
            member.size = len(content)
            member.mode = 0o755 if name == "install" else 0o644
            tar.addfile(member, io.BytesIO(content))
    return version, archive.getvalue()


def delay(failures):
    """Seconds to the next attempt: every minute, doubling with each consecutive failure up to 15 minutes."""
    return min(PERIOD_SECONDS * 2 ** failures, LONGEST_SECONDS)


def verdict(known, presented):
    """What to do with the host key the Orin presents: trust it (the first seen), go on, or stop."""
    if known is None:
        return "first"
    return "trusted" if known == presented else "changed"


def in_use():
    """Why the host is in use now (each component's mark under /run/rlark/in-use: a Station that serves a Run, a
    session attached to the device layer), or ""."""
    marks = sorted(IN_USE.glob("*")) if IN_USE.is_dir() else []
    return "; ".join("%s: %s" % (path.name, path.read_text().strip()[:200]) for path in marks)


class Orin:
    def __init__(self, address, user, robot_address, node, stopped, launcher, retired):
        self.address, self.user, self.node, self.stopped, self.launcher = address, user, node, stopped, launcher
        self.retired = retired
        environment = dict(S1U_ROS_IP_ADDRESS=robot_address, S1U_ORIN_IP_ADDRESS=address, S1U_ORIN_USER=user)
        self.payloads = {path.name: payload(path, environment)
                         for path in sorted(PAYLOADS.iterdir()) if path.is_dir()} if PAYLOADS.is_dir() else {}
        self.state = dict(reachable=False, identity=None, trust=None, failures=0,
                          payloads={name: dict(wanted=version) for name, (version, _) in self.payloads.items()})
        # The camera reset and diagnosis last done, as their results in the Pod's shared directory say: a request done
        # before this container restarted is not done again. The camera thread's own: each pass copies it to the state.
        self.camera = {action: json.loads(Path(CAMERAS + "/%s-result" % action).read_text())
                       for action in ("reset", "diagnose") if os.path.exists(CAMERAS + "/%s-result" % action)}
        self.wake = threading.Event()
        self.serial = None  # the Orin's hardware serial as the Node records it (known)
        self.kept = None  # the Node's record of the Orin as last written to KEPT
        self.confirm = None  # the fingerprint a person confirmed on the Node
        self.rotated = None  # the fingerprint of the key RLark gave the Orin, as the Node says
        self.directory = tempfile.mkdtemp(dir="/dev/shm")
        self.askpass = self.directory + "/askpass"
        Path(self.askpass).write_text("#!/bin/sh\nexec cat %s/password\n" % ACCESS)
        os.chmod(self.askpass, 0o700)

    # The cluster: this robot's Node holds the trusted host key and shows the state.
    def api(self, method, path, body=None, kind="application/merge-patch+json"):
        request = urllib.request.Request(
            "https://%s:%s%s" % (os.environ["KUBERNETES_SERVICE_HOST"], os.environ["KUBERNETES_SERVICE_PORT"], path),
            data=None if body is None else json.dumps(body).encode(), method=method,
            headers={"Authorization": "Bearer " + open(ACCOUNT + "/token").read(), "Content-Type": kind})
        context = ssl.create_default_context(cafile=ACCOUNT + "/ca.crt")
        with urllib.request.urlopen(request, context=context, timeout=10) as reply:
            return json.load(reply)

    def event(self, reason, message):
        at = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())
        self.api("POST", "/api/v1/namespaces/default/events", kind="application/json", body=dict(
            metadata=dict(generateName="s1u-orin-"), type="Normal", reason=reason, message=message,
            involvedObject=dict(kind="Node", name=self.node), source=dict(component="s1u-orin"),
            firstTimestamp=at, lastTimestamp=at, count=1))

    def known(self):
        """(the trusted host key, the record of its trust), each None before the first."""
        # The Node is the record. Its last reading stays on the robot's disk for the time the cluster is away: the
        # x86's uplink is this Orin, and it is then that the Orin must still be kept (11-13: 110 minutes without
        # the tunnel and nobody ended the LED script; s1u_02 cut off by its Orin and no pass reached the Orin).
        try:
            annotations = self.api("GET", "/api/v1/nodes/" + self.node)["metadata"].get("annotations", {})
        except OSError:
            if not os.path.exists(KEPT):
                raise
            kept = json.loads(Path(KEPT).read_text())
        else:
            self.confirm, self.rotated = annotations.get(CONFIRM), annotations.get(ROTATED)
            kept = dict(key=annotations.get(HOST_KEY), trust=json.loads(annotations.get(TRUST, "null")),
                        serial=annotations.get(SERIAL))
            if kept != self.kept:
                try:
                    Path(KEPT).write_text(json.dumps(kept))
                except OSError as error:  # the pass goes on with the Node's record; only a pass without the cluster needs the copy
                    print("The Orin's record is not kept on the robot (no pass while the cluster is away): %s" % error,
                          flush=True)
        self.kept, self.serial = kept, kept["serial"]
        return kept["key"], kept["trust"]

    def trust(self, key, fingerprint, how):
        """The Orin's host key from now on, with who trusted it and when on the Node and as an Event."""
        record = dict(fingerprint=fingerprint, at=time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()), how=how)
        self.api("PATCH", "/api/v1/nodes/" + self.node,
                 {"metadata": {"annotations": {HOST_KEY: key, TRUST: json.dumps(record), CONFIRM: None}}})
        # The robot's copy follows at once: a pass while the cluster is away must not hold the earlier key.
        self.kept = dict(key=key, trust=record, serial=self.serial)
        try:
            Path(KEPT).write_text(json.dumps(self.kept))
        except OSError as error:
            print("The Orin's record is not kept on the robot: %s" % error, flush=True)
        self.event("OrinHostKeyTrusted", "%s: the Orin's ssh host key %s trusted (%s)" % (self.node, fingerprint, how))
        print("Trusted the Orin's host key %s (%s)" % (fingerprint, how), flush=True)
        return record

    # The Orin.
    def scan(self):
        """(host key, fingerprint) the address presents now, or None."""
        line = subprocess.run(["ssh-keyscan", "-T", "5", "-t", "ed25519", self.address],
                              capture_output=True, text=True).stdout.strip()
        if not line:
            return None
        key = line.split(None, 1)[1]
        listing = subprocess.run(["ssh-keygen", "-lf", "-"], input=line, capture_output=True, text=True).stdout
        return key, listing.split()[1]

    def ssh(self, key, remote, data=b"", password=False):
        """``remote`` on the Orin, which must present ``key``; ``data`` to its standard input."""
        known = self.directory + "/known_hosts"
        # Replaced whole: the camera thread's ssh reads it while a pass writes it.
        staged = "%s.%d" % (known, threading.get_ident())
        Path(staged).write_text("%s %s\n" % (self.address, key))
        os.replace(staged, known)
        command = ["ssh", "-o", "UserKnownHostsFile=" + known, "-o", "StrictHostKeyChecking=yes",
                   "-o", "ConnectTimeout=5", "-o", "LogLevel=ERROR"]
        environment = dict(os.environ)
        if password:
            # Without a terminal ssh asks SSH_ASKPASS, which prints the mounted password to ssh's pipe.
            command += ["-o", "PreferredAuthentications=password", "-o", "PubkeyAuthentication=no",
                        "-o", "NumberOfPasswordPrompts=1"]
            environment.update(DISPLAY="none", SSH_ASKPASS=self.askpass)
        else:
            command += ["-i", ACCESS + "/ssh_key", "-o", "BatchMode=yes"]
        return subprocess.run(command + ["%s@%s" % (self.user, self.address), remote], input=data,
                              capture_output=True, timeout=180, env=environment, start_new_session=password)

    def sudo(self, key, remote):
        """``remote`` as root on the Orin, the password on sudo's standard input and nothing else there (-k: sudo
        asks for it every time, so it never reaches the command)."""
        return self.ssh(key, "sudo -k -S -p '' " + remote, Path(ACCESS + "/password").read_bytes().rstrip(b"\n") + b"\n")

    def upload(self, key, path, data):
        """``data`` as the account's file ``path`` on the Orin (what root then takes from there)."""
        return self.ssh(key, "umask 022; mkdir -p ~/rl && cat > %s" % path, data)

    def read(self, key, questions):
        """Each answer on the Orin under its name: one that prints nothing does not move the others up a line
        (s1u_01 11-14, first pass: the host route's address was read as the Orin's own, the sysctl file as a
        route)."""
        script = "; ".join("echo %s=$(%s)" % item for item in questions.items())
        return dict(line.split("=", 1) for line in self.ssh(key, script).stdout.decode().splitlines() if "=" in line)

    def reconcile(self):
        """One pass; the state's new fields, ``label`` among them."""
        if not os.path.exists(ACCESS + "/password"):
            # Without the login nothing was ever placed on an Orin whose host key was never trusted.
            if os.environ["S1U_NODE_WANTED"] == "removed" and self.known()[0] is None:
                return dict(label="removed", error=None, waiting=None)
            return dict(label="no_access", reachable=False, error="the Addon's orinAccessSecret is not in the cluster")
        scanned = self.scan()
        if scanned is None:
            return dict(label="unreachable", reachable=False, error="no ssh at %s" % self.address)
        key, fingerprint = scanned
        known, trust = self.known()
        identity = verdict(known, key)
        if identity == "changed" and fingerprint in (self.confirm, self.rotated):
            trust, identity = self.trust(key, fingerprint, "confirmed" if self.confirm == fingerprint else "rotated by RLark"), "trusted"
        if identity == "changed":
            # Nothing more goes to this address, the password least of all, until a person confirms.
            listing = subprocess.run(["ssh-keygen", "-lf", "-"], input="%s %s" % (self.address, known),
                                     capture_output=True, text=True).stdout
            return dict(label="identity_changed", reachable=True, identity="changed", trust=trust,
                        trusted_fingerprint=listing.split()[1], presented_fingerprint=fingerprint,
                        error="the Orin's ssh host key changed: confirm the new one (annotation %s)" % CONFIRM)
        if os.environ["S1U_NODE_WANTED"] == "removed":
            return self.remove(key if identity == "trusted" else None)
        if identity == "first":
            trust = self.trust(key, fingerprint, "first use")
        result = dict(reachable=True, identity="trusted", trust=trust, trusted_fingerprint=fingerprint,
                      presented_fingerprint=None)
        if self.ssh(key, "true").returncode:
            public = subprocess.run(["ssh-keygen", "-y", "-f", ACCESS + "/ssh_key"], capture_output=True,
                                    text=True, check=True).stdout.strip() + " s1u-robot"
            login = self.ssh(key, 'umask 077; mkdir -p ~/.ssh; line=$(cat); grep -qxF "$line" ~/.ssh/authorized_keys '
                                  '2>/dev/null || echo "$line" >> ~/.ssh/authorized_keys', public.encode(), True)
            if login.returncode:
                return dict(result, label="login_failed", error=login.stderr.decode(errors="replace")[-300:])
            print("First login to the Orin by password: the site's ssh key is authorized there now", flush=True)
        # Which Orin this is: its hardware serial (the ssh host key is the vendor image's, the same on every Orin).
        serial = self.ssh(key, "tr -d '\\0' < /proc/device-tree/serial-number").stdout.decode().strip()
        if serial and serial != self.serial:
            if self.serial:
                self.event("OrinReplaced", "%s: another Orin answers (%s, was %s)" % (self.node, serial, self.serial))
            self.api("PATCH", "/api/v1/nodes/" + self.node, {"metadata": {"annotations": {SERIAL: serial}}})
            self.serial = serial
        result["serial"] = serial
        # The vendor process that must not run there (node_profile.env S1U_ORIN_STOPPED): its line in the vendor's
        # start script is commented out, so that an Orin booting without the cluster does not start it either (s1u_01
        # 11-13: 110 minutes without the tunnel left 617 registrations on the ROS master), and it is ended where it runs.
        ended = self.ssh(key, "sed -i -E '\\#%s# s/^[^#]/#s1u-stopped# &/' ~/%s; pkill -e -f -- '%s' || true"
                         % (self.stopped, self.launcher, self.stopped)).stdout.decode().strip()
        if ended:
            print("Ended on the Orin: %s" % ended.replace("\n", "; "), flush=True)
        steps = self.root(key, result)
        if os.path.exists(ACCESS + "/adb_key"):
            held = self.read(key, dict(adb_key="sha256sum ~/.android/adbkey_rlark 2>/dev/null | cut -c1-64"))
            if hashlib.sha256(Path(ACCESS + "/adb_key").read_bytes()).hexdigest() != held["adb_key"]:
                steps.append(self.ssh(key, "umask 077; mkdir -p ~/.android; cat > ~/.android/adbkey_rlark",
                                      Path(ACCESS + "/adb_key").read_bytes()))
        waiting = self.place(key, steps, result)
        failed = next((step for step in steps if step.returncode), None)
        if failed:
            return dict(result, label="failed", error=failed.stderr.decode(errors="replace")[-300:])
        return dict(result, label="updating" if waiting else "ok", error=None, waiting=waiting or None)

    def root(self, key, result):
        """What only root places: the x86's uplink, the units the Addon retires, the cameras' USB rule. The steps
        taken."""
        read = self.read(key, dict(
            unit="sha256sum /etc/systemd/system/%s 2>/dev/null | cut -c1-64" % UPLINK,
            active="systemctl is-active %s" % UPLINK,
            forwarding="cat /proc/sys/net/ipv4/ip_forward",
            addresses="for device in $(ip -4 route show default | grep -o 'dev [^ ]*' | cut -d' ' -f2 | sort -u); do "
                      "ip -4 -o addr show dev $device | awk '{print $4}' | cut -d/ -f1; done",
            retired="for unit in %s; do [ ! -e /etc/systemd/system/$unit.service ] || echo $unit; done" % " ".join(self.retired),
            camera="cat %s 2>/dev/null" % CAMERA_RULE,
            # Not this Addon's and left alone (a colleague's forwarding script of 10-28, root's): said, for a person.
            found="ls ~/orin_gateway.sh /run/orin-fleet-gateway-* 2>/dev/null"))
        if read["found"] != self.state.get("found"):
            print("On the Orin, not this Addon's and left alone: %s" % (read["found"] or "nothing"), flush=True)
        result["found"] = read["found"]
        Path(HOST + "/orin_addresses").write_text(read["addresses"] + "\n")
        result["uplink_addresses"] = read["addresses"].split()
        steps = []
        placed = (read["unit"] == hashlib.sha256((HERE / UPLINK).read_bytes()).hexdigest()
                  and read["active"] == "active" and read["forwarding"] == "1")
        if not placed or read["retired"]:
            # A retired unit's stop deletes the forwarding rules this one holds too: this one starts again at once.
            retire = "".join("systemctl disable --now --quiet %s.service; rm -f /etc/systemd/system/%s.service; " % (unit, unit)
                             for unit in read["retired"].split())
            steps.append(self.upload(key, "~/rl/.s1u-orin-uplink.sh", uplink_setup().encode()))
            # The setup script starts the unit when it changed or does not run; a restart beyond that would run its
            # stop step and drop the x86's forwarding for a moment. Only after a retired unit's stop, which deleted
            # the rules both hold, is it started again.
            again = " && systemctl restart %s" % UPLINK if read["retired"] else ""
            steps.append(self.sudo(key, "sh -c '%ssh ~%s/rl/.s1u-orin-uplink.sh%s'" % (retire, self.user, again)))
            print("The x86's uplink placed and started on the Orin%s" % (
                " (retired: %s)" % read["retired"] if read["retired"] else ""), flush=True)
        uid = self.ssh(key, "id -u").stdout.decode().strip()
        rules = ['SUBSYSTEM=="usb", ATTR{idVendor}=="%s", OWNER="%s", MODE="0660"' % (vendor, uid) for vendor in CAMERA_VENDORS]
        if read["camera"] != " ".join(rules):  # as read: the file's lines on one
            steps.append(self.upload(key, "~/rl/.s1u-camera.rules", "".join(rule + "\n" for rule in rules).encode()))
            # Taken out again when udev did not take it: the next pass sees no rule and tries again.
            steps.append(self.sudo(key, "sh -c 'install -m 644 ~%s/rl/.s1u-camera.rules %s && udevadm control --reload-rules%s "
                                        "|| { rm -f %s; exit 1; }'" % (self.user, CAMERA_RULE, "".join(
                                            " && udevadm trigger --subsystem-match=usb --attr-match=idVendor=%s" % vendor
                                            for vendor in CAMERA_VENDORS), CAMERA_RULE)))
        return steps

    def place(self, key, steps, result):
        """The declared payloads at their versions, those no longer declared removed. What waits, or ""."""
        held = Path(HOST + "/rollout")
        held = "" if held.exists() and held.read_text().startswith("allowed") else "the robot's maintenance"
        on = self.read(key, dict(declared="cat ~/rl/.rlark-payloads 2>/dev/null | xargs", **{
            name: "cat ~/rl/%s/VERSION 2>/dev/null" % name for name in self.payloads}))
        states, waiting = {}, []
        for name, (version, archive) in self.payloads.items():
            running = on[name] or "none"
            directory = "~/rl/%s.%s" % (name, version)
            if running != version:
                steps.append(self.ssh(key, "rm -rf {0} && mkdir -p {0} && tar -xz -C {0}".format(directory), archive))
                steps.append(self.sudo(key, "%s/install" % directory))
                if held:
                    waiting.append("%s %s is on the Orin; it starts with %s" % (name, version, held))
                    states[name] = dict(wanted=version, running=running)
                    continue
                # A directory there before the versions is kept aside; older versions go.
                steps.append(self.ssh(key, (
                    "cd ~/rl && {{ [ -L {1} ] || [ ! -e {1} ] || mv {1} {1}.before_{0}; }} && ln -sfn {1}.{0} {1} "
                    "&& ls -d {1}.* | grep -v -e '{0}$' -e before_ | xargs -r rm -rf").format(version, name)))
                steps.append(self.sudo(key, "%s/install restart" % directory))
                print("Payload %s %s started on the Orin (was %s)" % (name, version, running), flush=True)
            states[name] = dict(wanted=version, running=version)
        for name in set(on["declared"].split()) - set(self.payloads):
            steps += self.drop(key, name)
            print("Payload %s removed from the Orin: no longer declared" % name, flush=True)
        steps.append(self.ssh(key, "mkdir -p ~/rl && echo %s > ~/rl/.rlark-payloads" % " ".join(sorted(self.payloads))))
        result["payloads"] = states
        return "; ".join(waiting)

    def drop(self, key, name):
        """The steps that take a payload off the Orin: its own removal, then its files."""
        return [self.sudo(key, "sh -c '[ ! -x ~%s/rl/%s/install ] || ~%s/rl/%s/install remove'" % (
                    self.user, name, self.user, name)),
                self.ssh(key, "cd ~/rl && rm -rf {0} {0}.????????????????".format(name))]

    def remove(self, key):
        """Takes back what reconcile placed on the Orin, the x86's login last; ``key`` is the trusted host key, None
        where this Orin was never trusted (nothing of the Addon's went there). The uplink stays: it is the robot's way
        to the site, there since the robot joined, and the x86 that asks for the removal would cut itself off."""
        if key is None or self.ssh(key, "true").returncode:
            return dict(label="removed", error=None, waiting=None)
        busy = in_use()
        if busy:
            return dict(label="removing", error=None, waiting="the payloads stop when nothing marks the host in use (%s)" % busy)
        declared = self.read(key, dict(declared="cat ~/rl/.rlark-payloads 2>/dev/null | xargs"))["declared"].split()
        steps = [step for name in declared for step in self.drop(key, name)]
        steps += [self.sudo(key, "sh -c 'rm -f %s && udevadm control --reload-rules'" % CAMERA_RULE), self.ssh(key, (
            "rm -f ~/rl/.rlark-payloads ~/.android/adbkey_rlark; sed -i 's/^#s1u-stopped# //' ~/%s; "
            "sed -i '/ s1u-robot$/d' ~/.ssh/authorized_keys" % self.launcher))]
        failed = next((step for step in steps if step.returncode), None)
        if failed:
            return dict(label="failed", error=failed.stderr.decode(errors="replace")[-300:])
        self.api("PATCH", "/api/v1/nodes/" + self.node, {"metadata": {"annotations": {HOST_KEY: None, TRUST: None, SERIAL: None}}})
        Path(KEPT).unlink(missing_ok=True)
        print("Removed from the Orin: the payloads, the cameras' USB rule, the adb key and this x86's login", flush=True)
        return dict(label="removed", error=None, waiting=None)

    def follow(self):
        while True:
            try:
                result = self.reconcile()
            except Exception as error:  # the API server or ssh itself: reported like any other failure
                result = dict(label="failed", error="%s: %s" % (type(error).__name__, error))
            before = self.state.get("label")
            # A host key that changed waits for a person, not for a retry: its confirmation is read every minute.
            good = result["label"] in ("ok", "updating", "removing", "removed", "identity_changed")
            self.state = dict(self.state, **result, failures=0 if good else self.state["failures"] + 1, since=(
                self.state.get("since") if before == result["label"] else time.time()), checked_at=time.time(),
                **{"camera_" + action: done for action, done in self.camera.items()})
            if before != result["label"]:  # one line per change, none while an Orin stays off
                print("Orin: %s%s" % (result["label"], ": %s" % result["error"] if result.get("error") else ""),
                      flush=True)
            try:
                self.api("PATCH", "/api/v1/nodes/" + self.node, {"metadata": {
                    "labels": {STATE: result["label"]}, "annotations": {STATE_ALL: json.dumps(self.state)}}})
            except OSError:
                pass  # written at the next pass
            self.wake.wait(delay(self.state["failures"]))
            self.wake.clear()

    def watch_cameras(self):
        """The camera service's requests, and every minute where the cameras are on the USB."""
        located = -PERIOD_SECONDS
        while True:
            time.sleep(WATCH_SECONDS)
            try:
                self.camera_pass()
                if time.monotonic() - located >= PERIOD_SECONDS:
                    located = time.monotonic()
                    self.camera_facts()
            except Exception as error:  # the Orin or the cluster away: looked at again
                print("Cameras on the Orin: %s: %s" % (type(error).__name__, error), flush=True)

    def camera_pass(self):
        """One look at the camera service's requests, each done once whatever comes of it, its result beside it."""
        for action, do in (("reset", self.reset_cameras), ("diagnose", self.diagnose_cameras)):
            try:
                request = json.loads(Path(CAMERAS + "/%s-request" % action).read_text())
            except (OSError, ValueError):  # none, or being written
                continue
            if request["id"] == self.camera.get(action, {}).get("id"):
                continue
            try:
                done = do(request["cameras"])
            except Exception as error:  # ssh timed out (a reset hung on a wedged device), the cluster away
                failed = "%s: %s" % (type(error).__name__, error)
                done = dict(ok=False, detail=failed) if action == "reset" else dict(
                    cameras=dict.fromkeys(request["cameras"], "Orin 没有回答（%s）" % failed))
            done = self.camera[action] = dict(done, id=request["id"], finished_at=time.time())
            Path(CAMERAS + "/%s-result.new" % action).write_text(json.dumps(done, ensure_ascii=False) + "\n")
            os.replace(CAMERAS + "/%s-result.new" % action, CAMERAS + "/%s-result" % action)
            print("Camera %s on the Orin (%s): %s" % (action, ", ".join(request["cameras"]), done.get("detail") or done["cameras"]), flush=True)

    def reset_cameras(self, names):
        """{ok: every one done, detail: one line, reset: the names reset, missing: the names without a USB device}:
        the USB devices of the cameras ``names`` reset through usbfs as the Orin's account, whose they are by the
        cameras' USB rule; no sudo. Without reset and missing when the reset did not run."""
        known, _ = self.known()
        if known is None:
            return dict(ok=False, detail="the Orin's host key is not trusted yet")
        done = self.ssh(known, "python3 -c '%s' %s" % (USB_RESET, " ".join(names)))
        if done.returncode:
            return dict(ok=False, detail=done.stderr.decode(errors="replace").strip().splitlines()[-1][-200:])
        devices = done.stdout.decode().splitlines()
        reset = [name for name in names if any(line.startswith(name + " ") for line in devices)]
        missing = [name for name in names if name not in reset]
        return dict(ok=not missing, reset=reset, missing=missing, detail="; ".join(
            (["reset ok " + ", ".join(devices)] if devices else []) + (["no USB device of " + ", ".join(missing)] if missing else [])))

    def camera_facts(self):
        """The cameras' facts on the Orin (CAMERA_FACTS), each one's place on the USB written for the camera service;
        None when the Orin does not answer."""
        if not self.kept or not self.kept["key"]:
            return None
        done = self.ssh(self.kept["key"], "python3 -", CAMERA_FACTS.encode())
        if done.returncode:
            return None
        facts = json.loads(done.stdout)
        Path(CAMERAS + "/usb.new").write_text(json.dumps({name: {key: camera[key] for key in ("path", "device", "speed", "hubs")}
                                                          for name, camera in facts["cameras"].items()}) + "\n")
        os.replace(CAMERAS + "/usb.new", CAMERAS + "/usb")
        return facts

    def diagnose_cameras(self, names):
        """{cameras: {name: what the Orin shows of it, in a line}}."""
        facts = self.camera_facts()
        if facts is None:
            return dict(cameras=dict.fromkeys(names, "Orin 没有回答"))
        said = {}
        for name in names:
            camera, sdk = facts["cameras"].get(name), facts["sdk"]
            lines = ["在 USB %s（%dM，经 %d 级 hub）" % (camera["path"], camera["speed"], len(camera["hubs"])) if camera
                     else "USB 上没有它（camera_info.json 的序列号）", "厂商相机进程%s" % ("在跑" if facts["process"] else "不在")]
            # The SDK's own log: a stream line every 5 s while it streams.
            orbbec = camera and camera["vendor"] == "2bc5" and camera["driver"] == "usbfs"
            if orbbec and sdk is None:
                lines.append("Orbbec SDK：没有日志")
            elif orbbec:
                color, depth = (sdk[stream] is not None and sdk[stream] <= 10 for stream in ("color", "depth"))
                if depth and not color:
                    lines.append("Orbbec SDK：彩色停了（%s），深度仍在，之后等帧超时 %d 次" % (
                        "%d 秒前" % sdk["color"] if sdk["color"] is not None else "日志尾部没有它", sdk["timeouts"]))
                else:
                    lines.append("Orbbec SDK：" + ("彩色和深度都有帧" if color and depth else "深度停了，彩色仍在" if color else "彩色和深度都停了"))
            said[name] = "；".join(lines)
        return dict(cameras=said)


def main():
    if "--uplink-setup" in sys.argv:
        print(uplink_setup(), end="")
        return
    lines = (line.split("=", 1) for line in open(os.environ["S1U_NODE_PROFILE"]).read().splitlines()
             if "=" in line and line[0] != "#")
    profile = {key: value.strip('"') for key, value in lines}
    # Off the SDK core and the device layer's CPUs, which kubelet's pool for Pods holds too.
    os.sched_setaffinity(0, {cpu for part in profile["S1U_KUBERNETES_CPUS"].split(",")
                             for cpu in range(int(part.split("-")[0]), int(part.split("-")[-1]) + 1)})
    orin = Orin(profile["S1U_ORIN_IP_ADDRESS"], profile["S1U_ORIN_USER"], profile["S1U_ROS_IP_ADDRESS"],
                os.environ["NODE_NAME"], profile["S1U_ORIN_STOPPED"], profile["S1U_ORIN_STOPPED_LAUNCHER"],
                os.environ.get("S1U_ORIN_RETIRED_UNITS", "").replace(",", " ").split())
    print("Orin of %s at %s: payloads %s" % (orin.node, orin.address, ", ".join(
        "%s %s" % (name, version) for name, (version, _) in orin.payloads.items()) or "none"), flush=True)
    threading.Thread(target=orin.watch_cameras, daemon=True).start()
    orin.follow()


if __name__ == "__main__":
    main()
