import base64
import json
import re
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid


PATH = "demo-agent/demo-node/front"
WHEP_URL = f"http://10.60.0.2:8889/{PATH}/whep"
API_URL = "http://10.61.0.2:9997"
READ_CREDENTIALS = ("whep-reader", "read-test-only")
PUBLISH_CREDENTIALS = ("synthetic-publisher", "publish-test-only")
CONTROL_CREDENTIALS = ("media-controller", "control-test-only")

OFFER = """v=0
o=- 10000 1 IN IP4 127.0.0.1
s=-
t=0 0
a=group:BUNDLE 0
a=msid-semantic: WMS *
m=video 9 UDP/TLS/RTP/SAVPF 96
c=IN IP4 0.0.0.0
a=rtcp:9 IN IP4 0.0.0.0
a=ice-ufrag:rlarktest
a=ice-pwd:012345678901234567890123
a=ice-options:trickle
a=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF
a=setup:actpass
a=mid:0
a=recvonly
a=rtcp-mux
a=rtcp-rsize
a=rtpmap:96 H264/90000
a=fmtp:96 packetization-mode=1;level-asymmetry-allowed=1;profile-level-id=42e01f
"""


def request(url, method="GET", credentials=None, body=None, content_type=None):
    headers = {}
    if credentials:
        raw = f"{credentials[0]}:{credentials[1]}".encode()
        headers["Authorization"] = "Basic " + base64.b64encode(raw).decode()
    if content_type:
        headers["Content-Type"] = content_type
    req = urllib.request.Request(url, data=body, headers=headers, method=method)
    try:
        with urllib.request.urlopen(req, timeout=5) as response:
            return response.status, response.headers, response.read()
    except urllib.error.HTTPError as error:
        return error.code, error.headers, error.read()


def assert_status(label, result, expected):
    status, _, body = result
    if status != expected:
        raise RuntimeError(f"{label}: expected HTTP {expected}, got {status}: {body.decode(errors='replace')}")


def post_whep(credentials, retries=1):
    last = None
    for _ in range(retries):
        last = request(
            WHEP_URL,
            method="POST",
            credentials=credentials,
            body=OFFER.encode(),
            content_type="application/sdp",
        )
        if last[0] == 201:
            return last
        if last[0] not in (400, 404, 503):
            break
        time.sleep(1)
    return last


def location_and_session_id(headers):
    location = headers.get("Location")
    session_id = headers.get("ID")
    if not location or not session_id:
        raise RuntimeError("WHEP response must include Location and ID headers")
    absolute = urllib.parse.urljoin(WHEP_URL, location)
    parsed = urllib.parse.urlparse(absolute)
    secret = parsed.path.rsplit("/", 1)[-1]
    if not re.fullmatch(r"/" + re.escape(PATH) + r"/whep/[0-9a-f-]{36}", parsed.path):
        raise RuntimeError(f"unexpected WHEP Location path: {parsed.path}")
    uuid.UUID(secret)
    uuid.UUID(session_id)
    if secret == session_id:
        raise RuntimeError("v1.21.4 is expected to use distinct Location secret and ID UUIDs")
    return absolute, session_id, secret


def control_request(path, method="GET", credentials=CONTROL_CREDENTIALS):
    return request(API_URL + path, method=method, credentials=credentials)


def list_sessions(credentials=CONTROL_CREDENTIALS):
    status, _, body = control_request("/v3/webrtc/sessions/list", credentials=credentials)
    assert_status("Control API session list", (status, {}, body), 200)
    return json.loads(body).get("items", [])


def wait_for_session(session_id, present):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        found = any(item.get("id") == session_id for item in list_sessions())
        if found == present:
            return
        time.sleep(0.2)
    raise RuntimeError(f"session {session_id} did not reach present={present}")


def main():
    jpeg_path = "/media/synthetic.jpg"
    deadline = time.monotonic() + 30
    while time.monotonic() < deadline:
        try:
            with open(jpeg_path, "rb") as image:
                header = image.read(3)
            if header == b"\xff\xd8\xff":
                break
        except FileNotFoundError:
            pass
        time.sleep(0.25)
    else:
        raise RuntimeError("synthetic JPEG was not generated")
    print(f"synthetic JPEG present at {jpeg_path}")

    assert_status("anonymous Control API", control_request("/v3/webrtc/sessions/list", credentials=None), 401)
    assert_status("reader credential on Control API", control_request("/v3/webrtc/sessions/list", credentials=READ_CREDENTIALS), 401)
    assert_status("publisher credential on Control API", control_request("/v3/webrtc/sessions/list", credentials=PUBLISH_CREDENTIALS), 401)
    assert_status("anonymous WHEP POST", post_whep(None), 401)
    assert_status("wrong WHEP Basic credential", post_whep(("whep-reader", "wrong")), 401)
    assert_status("Control API credential cannot read", post_whep(CONTROL_CREDENTIALS), 401)
    assert_status("publish credential cannot read", post_whep(PUBLISH_CREDENTIALS), 401)
    print("credential separation verified for Control API, WHEP read, and RTSP publish roles")

    status, headers, answer = post_whep(READ_CREDENTIALS, retries=20)
    assert_status("authorized WHEP POST", (status, headers, answer), 201)
    if headers.get_content_type() != "application/sdp" or b"m=video" not in answer or b"H264" not in answer:
        raise RuntimeError("WHEP response did not contain an H.264 SDP answer")
    location, session_id, location_secret = location_and_session_id(headers)
    print(f"WHEP POST returned HTTP 201, ID={session_id}, Location={location}")
    print(f"Location UUID secret={location_secret}; ID is a distinct session UUID")

    listed = list_sessions()
    session = next((item for item in listed if item.get("id") == session_id), None)
    if not session or session.get("path") != PATH or session.get("state") != "read":
        raise RuntimeError(f"WHEP read session was not listed with the expected path and state: {session}")
    kick_status, _, kick_body = control_request(f"/v3/webrtc/sessions/kick/{session_id}", method="POST")
    assert_status("Control API session kick", (kick_status, {}, kick_body), 200)
    wait_for_session(session_id, present=False)
    assert_status("WHEP DELETE after API kick", request(location, method="DELETE", credentials=READ_CREDENTIALS), 404)
    path_status, _, path_body = control_request(f"/v3/paths/get/{urllib.parse.quote(PATH, safe='/')}")
    assert_status("publisher path after reader kick", (path_status, {}, path_body), 200)
    publisher_path = json.loads(path_body)
    if not publisher_path.get("available") or not publisher_path.get("source"):
        raise RuntimeError(f"reader kick affected the RTSP publisher path: {publisher_path}")
    print("Control API list/kick/list removed the WHEP reader and retained the RTSP publisher")

    status, headers, answer = post_whep(READ_CREDENTIALS, retries=20)
    assert_status("second authorized WHEP POST", (status, headers, answer), 201)
    location, session_id, _ = location_and_session_id(headers)
    delete_status, _, delete_body = request(location, method="DELETE", credentials=READ_CREDENTIALS)
    assert_status("WHEP DELETE with dedicated Basic credential", (delete_status, {}, delete_body), 200)
    wait_for_session(session_id, present=False)
    print("WHEP DELETE with dedicated Basic credential returned HTTP 200")

    status, headers, answer = post_whep(READ_CREDENTIALS, retries=20)
    assert_status("third authorized WHEP POST", (status, headers, answer), 201)
    location, _, _ = location_and_session_id(headers)
    delete_status, _, delete_body = request(location, method="DELETE")
    assert_status("native WHEP DELETE credential probe", (delete_status, {}, delete_body), 200)
    print("native v1.21.4 also accepted DELETE without Basic because Location carries a session secret")


if __name__ == "__main__":
    main()
