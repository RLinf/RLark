# Isolated MediaMTX Deployment Fixture

This fixture validates the native MediaMTX HTTP/WHEP and Control API contracts for the camera streaming handoff. It is an isolated test deployment. It does not change the production Compose stack, Gateway, physical robots, or site networking.

## Pinned upstream image

The fixture uses upstream `bluenviron/mediamtx:1.21.4`, pinned to the Docker Hub OCI index digest `sha256:c730bccdbc8739f8f0c13cefe9bea8b61926f599b88fa4d0d229baa91846c7bc`. The upstream amd64 manifest is `sha256:456791fae0607e00d4f7105fd4048e1b9bb9c5cb43e0fe0cb55e3beeea023f15`; the arm64 manifest is `sha256:d98bbc8e7383de652e6479a2746eb6468a65ef54b7eecaaf5fdcacc63c969f7f`. The OCI index digest is the value pinned in Compose.

The synthetic publisher uses `jrottenberg/ffmpeg:7.1-alpine@sha256:8ec1ee1f6a0fcd37c97725827b6b7832795c9596e3439b8da56d7700d61ae778`. The smoke client uses `python:3.13-alpine@sha256:9ef1a599f863231c3d91e941389bfdc4a2d6070557243d54df1a4e57015b960e`.

## Run

Prerequisites are Docker Engine with Compose v2 and outbound access to pull the pinned images. From the repository root, run:

```sh
docker compose -f astribot/test/mediamtx/compose.yaml --profile test config --quiet
./astribot/test/mediamtx/smoke.sh
```

The smoke script starts only the fixture services and removes its containers and named volume on exit. It creates a synthetic JPEG and continuously publishes synthetic H.264 over RTSP/TCP. The test then negotiates a WHEP SDP answer, checks session creation and deletion, and uses the MediaMTX Control API to list, kick, and list the WHEP session. It confirms the RTSP publisher remains available after the reader session is kicked. No ICE host port is published because the test stops at SDP/session behavior and does not attempt an ICE media connection.

The current fixture path is `demo-agent/demo-node/front`, corresponding to `{agent}/{node}/{view}`. The private WHEP endpoint is `http://10.60.0.2:8889/demo-agent/demo-node/front/whep`. A Gateway on the fixture's media bridge would use the same native MediaMTX HTTP endpoint. The Control API listens only on `10.61.0.2:9997` on its separate internal bridge. Neither WHEP, RTSP, nor Control API ports are published on the host.

## Authentication and session behavior

Anonymous access is disabled. The synthetic publisher has `publish` permission only for the configured path, and the RTSP transport is TCP-only. The dedicated `whep-reader` account has `read` permission only for that path. The dedicated `media-controller` account has the `api` permission and no `read` or `publish` permission. All passwords in this fixture are test-only literals and must not be reused outside this isolated environment.

WHEP `POST` requires the dedicated Basic read credential. The smoke test verifies anonymous, wrong, publisher, and Control API credentials receive HTTP `401`. It sends the same read credential on WHEP `DELETE` and verifies deletion succeeds. In MediaMTX v1.21.4, `Location` contains a UUID session secret while the `ID` response header contains a different UUID used by the Control API. The session-specific `DELETE` handler accepts the Location secret without rechecking Basic authentication. The smoke test records this native behavior explicitly. Treat the `Location` value as a bearer capability, keep WHEP private, and send the read credential on both methods at the Gateway boundary.

The Control API smoke test verifies Basic authentication, rejects reader and publisher credentials, and exercises `GET /v3/webrtc/sessions/list`, `POST /v3/webrtc/sessions/kick/{id}`, then another list request. The API bind address is only on the dedicated control bridge and has no host port mapping. The fixture's API account can invoke the API surface supported by MediaMTX, so only a trusted Gateway should receive it.

The run on 2026-11-27 returned HTTP `201`, `ID=856ae823-befe-40a4-b83a-457c6963e1e5`, and `Location=/demo-agent/demo-node/front/whep/79cadae1-177c-4f4b-a3b8-16fad3a3791b`. The two UUID values differed as expected. The authorized WHEP `DELETE` returned HTTP `200`; the unauthenticated credential probe also returned `200`, confirming that the Location UUID secret is the native deletion capability.

The intended Gateway configuration names for the Control API are `RLARK_CAMERA_WHEP_CONTROL_ORIGIN`, `RLARK_CAMERA_WHEP_CONTROL_USER`, and `RLARK_CAMERA_WHEP_CONTROL_PASSWORD`. This fixture validates the upstream endpoint and credentials independently. It does not add or claim implementation of Gateway configuration.

## Recorded v1.21.4 contract

- The WHEP response is HTTP `201` with an SDP answer, `Location`, and `ID` headers.
- The `Location` suffix is a UUID secret and differs from the `ID` session UUID.
- WHEP `POST` authorization uses MediaMTX's `read` permission. WHEP `DELETE` uses the UUID secret in its Location path and does not perform a new Basic authentication check.
- The Control API provides `GET /v3/webrtc/sessions/list` and `POST /v3/webrtc/sessions/kick/{id}`. A kick removes the selected WebRTC session and leaves the RTSP publisher path available.
- The fixture is protocol/deployment smoke coverage. It does not establish browser ICE connectivity, production authorization, Gateway recovery behavior, or physical device compatibility.

Upstream references: [MediaMTX v1.21.4 release](https://github.com/bluenviron/mediamtx/releases/tag/v1.21.4), [v1.21.4 configuration](https://github.com/bluenviron/mediamtx/blob/v1.21.4/mediamtx.yml), and [v1.21.4 Control API](https://github.com/bluenviron/mediamtx/blob/v1.21.4/api/openapi.yaml).
