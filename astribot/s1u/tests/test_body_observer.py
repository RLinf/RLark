import json
import math
import struct
import sys
from pathlib import Path

import pytest

S1U = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(S1U / "device"))

from body_observer import Latest, decode  # noqa: E402


def test_catalog_camera_variants_match_camera_service():
    payload = S1U.parents[1] / "apps/rlark/pkg/gateway/astribot_s1u_capabilities.json"
    catalog = json.loads(payload.read_text())
    assert {v["id"]: v["cameras"] for v in catalog["variants"]} == json.loads((S1U / "tuner/bodies.json").read_text())


def test_joint_position_can_survive_invalid_velocity():
    # Header, mode, empty names, then three vectors (last two unused here).
    prefix = struct.pack("<4IbI", 2, 123, 250000000, 0, 1, 0)
    vector = lambda *xs: struct.pack("<I", len(xs)) + struct.pack("<%dd" % len(xs), *xs)
    sample = decode(prefix + vector(1.0, 0.0) + vector(math.nan, 0.0), 2)
    assert sample == {"position": (1.0, 0.0), "sourceUnixSeconds": 123.25}
    with pytest.raises(ValueError, match="wrong joint vector size"):
        decode(prefix + vector(1.0) + vector(0.0, 0.0), 2)


def test_pose_preserves_missing_frame_and_rejects_invalid_quaternion():
    header = struct.pack("<4I", 1, 123, 0, 0)
    pose = (1, 2, 3, 0, 0, 0, 1)
    assert decode(header + struct.pack("<7d", *pose), None) == {
        "sourceUnixSeconds": 123.0, "position": (1.0, 2.0, 3.0), "quaternion": (0.0, 0.0, 0.0, 1.0),
    }
    with pytest.raises(ValueError, match="invalid pose"):
        decode(header + struct.pack("<7d", *pose[:-1], 0), None)


def test_stale_and_recovery_are_independent_per_topic():
    latest = Latest()
    joint, endpoint = ("joint", "torso"), ("endpoint", "arm_left")
    assert latest.read(joint, 10)["state"] == "waiting"
    latest.record(joint, {"sourceUnixSeconds": 100, "position": (1,)}, 10)
    latest.record(joint, {"sourceUnixSeconds": 99, "position": (2,)}, 11)
    latest.record(endpoint, {"sourceUnixSeconds": 102}, 12)
    latest.record(joint, {"sourceUnixSeconds": 100, "position": (3,)}, 13)
    assert latest.read(joint, 13) == {"id": "torso", "state": "stale", "ageSeconds": 3}
    assert latest.read(endpoint, 13)["state"] == "live"
    latest.record(joint, {"sourceUnixSeconds": 101, "position": (4,)}, 14)
    assert latest.read(joint, 14)["position"] == (4,)
