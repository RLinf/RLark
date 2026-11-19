"""The robot's clock held to the site's time (user 11-13): a container of the node tuner's Pod (Addon
s1u-robot), on every S1U robot x86.

The vendor has its own time chain, started by its boot scripts and left exactly as it is: ptp4l makes the x86 the PTP
master of the robot's internal network, phc2sys makes the x86's system clock follow that NIC's hardware clock, and
the Orin follows over PTP. The chain's root is the NIC's oscillator, which nothing ties to real time: measured 11-12,
s1u_01 was 1.0 s and s1u_02 0.3 s behind the site. (chrony on the x86 would be a second servo on the system clock
phc2sys already steers.)

This trims the root: every PERIOD_SECONDS it measures the system clock against the site's time source by SNTP
(S1U_SITE_TIME_SOURCE, the Addon's parameter timeSource: the site's, not the robot model's; the best
of a few exchanges) and sets the frequency of the hardware clock phc2sys reads (a
PI servo, within LIMIT_PPM). The vendor's chain carries the correction to the system clock and to the Orin. It only
changes the rate: 1 s is made up in about 35 minutes. A jump is made only while the vendor's driver is not running
and the clock is more than STEP_SECONDS off. Without a time source in the profile, or while none answers, it changes
nothing.

The state is on the Node: label rlark.io/clock-state (locked | slewing | no_source | unreachable |
no_hardware_clock) and annotation rlark.io/clock-report (offset_ms, frequency_ppm, source, at), and in
/run/rlark/s1u/clock.json on the host, which node_profile.sh status notes.

Standard library only (/usr/bin/python3); privileged for the hardware clock's device, in the host's PID namespace."""
import ctypes
import glob
import json
import os
import socket
import ssl
import struct
import time
import urllib.request

ACCOUNT = "/var/run/secrets/kubernetes.io/serviceaccount"
STATE, REPORT = "rlark.io/clock-state", "rlark.io/clock-report"
PERIOD_SECONDS, LONGEST_SECONDS = 16, 900
LIMIT_PPM = 500.0
LOCKED_SECONDS = 0.05  # the profile's promise: within 50 ms of the site
STEP_SECONDS = 0.1
NTP_EPOCH = 2208988800  # seconds from 1900 to 1970
ADJ_FREQUENCY, ADJ_SETOFFSET, ADJ_NANO = 0x0002, 0x0100, 0x2000


class Timex(ctypes.Structure):
    """struct timex of clock_adjtime(2)."""
    _fields_ = [("modes", ctypes.c_uint), ("offset", ctypes.c_long), ("freq", ctypes.c_long),
                ("maxerror", ctypes.c_long), ("esterror", ctypes.c_long), ("status", ctypes.c_int),
                ("constant", ctypes.c_long), ("precision", ctypes.c_long), ("tolerance", ctypes.c_long),
                ("seconds", ctypes.c_long), ("fraction", ctypes.c_long), ("tick", ctypes.c_long),
                ("ppsfreq", ctypes.c_long), ("jitter", ctypes.c_long), ("shift", ctypes.c_int),
                ("stabil", ctypes.c_long), ("jitcnt", ctypes.c_long), ("calcnt", ctypes.c_long),
                ("errcnt", ctypes.c_long), ("stbcnt", ctypes.c_long), ("tai", ctypes.c_int),
                ("padding", ctypes.c_int * 11)]


def offset_of(reply, sent, received):
    """(seconds this clock is ahead of the server, round trip) from an NTP reply and this clock's send and receive
    times."""
    fields = struct.unpack("!12I", reply[:48])
    server_received = fields[8] - NTP_EPOCH + fields[9] / 2 ** 32
    server_sent = fields[10] - NTP_EPOCH + fields[11] / 2 ** 32
    return ((sent - server_received) + (received - server_sent)) / 2, (received - sent) - (server_sent - server_received)


def measure(source, exchanges=4):
    """This system clock's offset to ``source`` (seconds ahead), from the exchange with the shortest round trip."""
    samples = []
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as connection:
        connection.settimeout(1.0)
        for _ in range(exchanges):
            sent = time.time()
            connection.sendto(b"\x23" + bytes(47), (source, 123))  # version 4, client
            reply = connection.recv(64)
            samples.append(offset_of(reply, sent, time.time()))
    return min(samples, key=lambda sample: sample[1])[0]


class Servo:
    """A PI servo on the offset (seconds ahead): the hardware clock's frequency in ppm, within LIMIT_PPM. The gains
    are linuxptp's for this period (0.7 and 0.3 per period)."""

    def __init__(self, period=PERIOD_SECONDS):
        self.proportional, self.integral, self.sum = 0.7 / period, 0.3 / period, 0.0

    def frequency(self, offset):
        ppm = -(self.proportional * offset + self.integral * self.sum) * 1e6
        if abs(ppm) < LIMIT_PPM:  # no wind-up while the rate is at its limit
            self.sum += offset
            ppm = -(self.proportional * offset + self.integral * self.sum) * 1e6
        return max(-LIMIT_PPM, min(LIMIT_PPM, ppm))


def processes():
    """Each process of the host as its argument list (this Pod is in the host's PID namespace)."""
    for path in glob.glob("/proc/[0-9]*/cmdline"):
        try:
            yield open(path).read().split("\0")[:-1]
        except OSError:  # it ended meanwhile
            pass


def hardware_clock(commands):
    """The device of the hardware clock the vendor's phc2sys makes the system clock follow, or None while it does
    not run: its -s interface's PTP clock."""
    for command in commands:
        if command and os.path.basename(command[0]) == "phc2sys" and "-s" in command:
            devices = glob.glob("/sys/class/net/%s/device/ptp/ptp*" % command[command.index("-s") + 1])
            return "/dev/" + os.path.basename(devices[0]) if devices else None
    return None


def adjust(device, ppm=None, step=None):
    """Sets the hardware clock's frequency (``ppm``) or moves it by ``step`` seconds."""
    libc = ctypes.CDLL("libc.so.6", use_errno=True)
    descriptor = os.open(device, os.O_RDWR)
    try:
        timex = Timex()
        if step is None:
            timex.modes, timex.freq = ADJ_FREQUENCY, int(ppm * 65536)
        else:
            whole = int(step // 1)
            timex.modes, timex.seconds, timex.fraction = ADJ_SETOFFSET | ADJ_NANO, whole, int((step - whole) * 1e9)
        # A file descriptor's dynamic clock: ((~fd) << 3) | 3.
        if libc.clock_adjtime(ctypes.c_int((~descriptor << 3) | 3), ctypes.byref(timex)) < 0:
            raise OSError(ctypes.get_errno(), "clock_adjtime on %s" % device)
    finally:
        os.close(descriptor)


def publish(node, label, report=None):
    annotations = {} if report is None else {REPORT: json.dumps(report)}
    request = urllib.request.Request(
        "https://%s:%s/api/v1/nodes/%s" % (os.environ["KUBERNETES_SERVICE_HOST"],
                                            os.environ["KUBERNETES_SERVICE_PORT"], node),
        data=json.dumps({"metadata": {"labels": {STATE: label}, "annotations": annotations}}).encode(),
        method="PATCH", headers={"Authorization": "Bearer " + open(ACCOUNT + "/token").read(),
                                 "Content-Type": "application/merge-patch+json"})
    urllib.request.urlopen(request, context=ssl.create_default_context(cafile=ACCOUNT + "/ca.crt"), timeout=10)


def main():
    lines = (line.split("=", 1) for line in open(os.environ["S1U_NODE_PROFILE"]).read().splitlines()
             if "=" in line and line[0] != "#")
    profile = {key: value.strip('"') for key, value in lines}
    # Off the SDK core and the device layer's CPUs, which kubelet's pool for Pods holds too (device_pod.yaml).
    os.sched_setaffinity(0, {cpu for part in profile["S1U_KUBERNETES_CPUS"].split(",")
                             for cpu in range(int(part.split("-")[0]), int(part.split("-")[-1]) + 1)})
    source, driver, node = os.environ.get("S1U_SITE_TIME_SOURCE", ""), profile["S1U_VENDOR_DRIVER"], os.environ["NODE_NAME"]
    servo, label, failures, published = Servo(), None, 0, 0.0
    print("Clock of %s against %s" % (node, source or "no site time source"), flush=True)
    if os.environ["S1U_NODE_WANTED"] == "removed":
        device = hardware_clock(list(processes()))
        if device and os.path.exists("/host/clock.json"):  # it was trimmed: back to its own rate
            adjust(device, ppm=0.0)
            os.remove("/host/clock.json")
        publish(node, "removed")
        while True:
            time.sleep(3600)
    while True:
        report, wait = None, PERIOD_SECONDS
        commands = list(processes())
        device = hardware_clock(commands)
        if not source:
            now = "no_source"
        elif device is None:
            now = "no_hardware_clock"  # the vendor's phc2sys is not up (yet)
        else:
            try:
                offset = measure(source)
            except OSError:
                now, failures = "unreachable", failures + 1
                wait = min(PERIOD_SECONDS * 2 ** failures, LONGEST_SECONDS)
            else:
                failures = 0
                runs = any(command and os.path.basename(command[0]) == driver for command in commands)
                if abs(offset) > STEP_SECONDS and not runs:
                    adjust(device, step=-offset)
                    servo = Servo()
                    print("Stepped %s by %+.3f s (the driver is not running)" % (device, -offset), flush=True)
                ppm = servo.frequency(offset)
                adjust(device, ppm=ppm)
                now = "locked" if abs(offset) <= LOCKED_SECONDS else "slewing"
                report = dict(offset_ms=round(offset * 1e3, 2), frequency_ppm=round(ppm, 2), source=source,
                              device=device, at=time.time())
                with open("/host/clock.json.new", "w") as target:
                    json.dump(report, target)
                os.replace("/host/clock.json.new", "/host/clock.json")
        if now != label or (report and time.time() - published > 60):
            if now != label:  # one line per change
                print("Clock: %s %s" % (now, report or ""), flush=True)
            try:
                publish(node, now, report)
                label, published = now, time.time()
            except OSError:
                pass  # the API server is away: again at the next pass
        time.sleep(wait)


if __name__ == "__main__":
    main()
