#!/usr/bin/env python3
"""How long this boot of the robot x86 took, milestone by milestone (user 11-12: "每次重启都要10分钟太慢了"):
a host service the node profile installs (rlark-s1u-boot-report.service, started early in every boot). It notes the
second each milestone is first reached, counted from the power button (firmware and boot loader included, as
systemd measured them), and keeps the result where the node tuner publishes it on the Node (annotation
rlark.io/boot-report) and an operator reads it: /run/rlark/s1u/boot_report.json, with every boot's
copy under /var/lib/rlark/s1u/boots/.

    ssh            sshd listens
    network        the uplink is up (network-online.target)
    registry       the site's registry answers (images are pulled from it)
    ros_master     the vendor's roscore answers
    tunnel         RLark's management plane answers a ping through the tunnel
    kubelet        kubelet runs
    node_ready     the API server has the node Ready (kubelet reached it and its network is up)
    driver         the vendor's driver runs (S1U_VENDOR_DRIVER)

What the robot's device layer reaches (its control endpoint, a Station attached) is the device layer's to time.

Standard library only (the host's python3). It ends once every milestone is reached, or after LIMIT_SECONDS."""
import json
import os
import re
import socket
import ssl
import subprocess
import time
import urllib.request
from pathlib import Path

PROFILE = "/etc/rlark/s1u/node_profile.env"
LIVE = Path("/run/rlark/s1u/boot_report.json")
BOOTS = Path("/var/lib/rlark/s1u/boots")
LIMIT_SECONDS = 1800
ORDER = ("ssh", "network", "registry", "ros_master", "tunnel", "kubelet", "node_ready", "driver")


def profile():
    """The node profile's values (KEY=VALUE lines, values possibly quoted)."""
    lines = (line.split("=", 1) for line in open(PROFILE).read().splitlines() if "=" in line and line[0] != "#")
    return {key: value.strip('"') for key, value in lines}


def uptime():
    return float(open("/proc/uptime").read().split()[0])


def before_kernel():
    """Seconds from the power button to the kernel's start: firmware and boot loader, as systemd read them from
    the firmware (0.0 where it gave none)."""
    def microseconds(name):
        value = subprocess.run(["systemctl", "show", "-p", name, "--value"], capture_output=True, text=True).stdout
        return int(value.strip() or 0)
    # Both count back from the kernel's start; the firmware's includes the loader's.
    return max(microseconds("FirmwareTimestampMonotonic"), microseconds("LoaderTimestampMonotonic")) / 1e6


def listens(host, port):
    try:
        socket.create_connection((host, port), 0.5).close()
        return True
    except OSError:
        return False


def active(unit):
    return subprocess.run(["systemctl", "is-active", "-q", unit]).returncode == 0


def node_name():
    """This host's Node, as kubelet registers it: its --hostname-override, else the host name."""
    flags = Path("/var/lib/kubelet/kubeadm-flags.env").read_text() if Path("/var/lib/kubelet/kubeadm-flags.env").exists() else ""
    named = re.search(r"--hostname-override=([^ \"]+)", flags)
    return named[1] if named else socket.gethostname().lower()


def checks(values, node):
    """Each milestone's check: reached when it returns something true."""
    ros, gateway = values["S1U_ROS_IP_ADDRESS"], values["S1U_SITE_GATEWAY"]
    registry, registry_port = values["S1U_IMAGE_REGISTRY"].split(":")

    def node_ready():
        # As kubelet itself asks: its own Node at the API server it is configured for, with its client certificate.
        server = re.search(r"server:\s*(\S+)", open("/etc/kubernetes/kubelet.conf").read())[1]
        context = ssl.create_default_context(cafile="/etc/kubernetes/pki/ca.crt")
        context.load_cert_chain("/var/lib/kubelet/pki/kubelet-client-current.pem")
        try:
            with urllib.request.urlopen("%s/api/v1/nodes/%s" % (server, node), context=context, timeout=2) as reply:
                conditions = json.load(reply)["status"]["conditions"]
        except OSError:
            return False
        return any(condition["type"] == "Ready" and condition["status"] == "True" for condition in conditions)

    return {
        "ssh": lambda: listens("127.0.0.1", 22),
        "network": lambda: active("network-online.target"),
        "registry": lambda: listens(registry, int(registry_port)),
        "ros_master": lambda: listens(ros, 11311),
        "tunnel": lambda: subprocess.run(["ping", "-c1", "-W1", "-q", gateway], capture_output=True).returncode == 0,
        "kubelet": lambda: active("kubelet"),
        "node_ready": node_ready,
        "driver": lambda: subprocess.run(["pgrep", "-x", values["S1U_VENDOR_DRIVER"]],
                                         capture_output=True).returncode == 0,
    }


def main():
    values = profile()
    node = node_name()
    offset, pending, reached = None, checks(values, node), {}
    boot = open("/proc/sys/kernel/random/boot_id").read().strip()
    LIVE.parent.mkdir(parents=True, exist_ok=True)
    BOOTS.mkdir(parents=True, exist_ok=True)
    while pending and uptime() < LIMIT_SECONDS:
        for name in [name for name, check in pending.items() if check()]:
            reached[name] = round(uptime(), 1)
            del pending[name]
        if offset is None and active("multi-user.target"):
            offset = before_kernel()  # known once systemd finished starting up
        before = offset or 0.0
        report = dict(boot_id=boot, node=node, booted_unix_s=round(time.time() - uptime()),
                      before_kernel_s=round(before, 1), complete=not pending,
                      seconds_from_power_button={name: round(reached[name] + before, 1)
                                                 for name in ORDER if name in reached},
                      not_reached=[name for name in ORDER if name in pending])
        temporary = LIVE.with_suffix(".new")
        temporary.write_text(json.dumps(report, indent=1) + "\n")
        os.replace(temporary, LIVE)
        time.sleep(0.5)
    (BOOTS / (boot + ".json")).write_text(LIVE.read_text())


if __name__ == "__main__":
    main()
