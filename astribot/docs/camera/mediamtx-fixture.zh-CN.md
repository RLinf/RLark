# 隔离的 MediaMTX 部署测试夹具

此测试夹具用于验证相机媒体交接所依赖的 MediaMTX 原生 HTTP/WHEP 与 Control API 契约。它只用于隔离测试，不修改生产 Compose、Gateway、实体机器人或站点网络。

## 固定的上游镜像

夹具使用上游 `bluenviron/mediamtx:1.21.4`，固定到 Docker Hub OCI index digest `sha256:c730bccdbc8739f8f0c13cefe9bea8b61926f599b88fa4d0d229baa91846c7bc`。上游 amd64 manifest 为 `sha256:456791fae0607e00d4f7105fd4048e1b9bb9c5cb43e0fe0cb55e3beeea023f15`，arm64 manifest 为 `sha256:d98bbc8e7383de652e6479a2746eb6468a65ef54b7eecaaf5fdcacc63c969f7f`。Compose 固定使用 OCI index digest。

合成媒体发布器使用 `jrottenberg/ffmpeg:7.1-alpine@sha256:8ec1ee1f6a0fcd37c97725827b6b7832795c9596e3439b8da56d7700d61ae778`。Smoke 客户端使用 `python:3.13-alpine@sha256:9ef1a599f863231c3d91e941389bfdc4a2d6070557243d54df1a4e57015b960e`。

## 运行

前置条件为 Docker Engine、Docker Compose v2，以及拉取固定镜像的网络访问。从仓库根目录运行：

```sh
docker compose -f astribot/test/mediamtx/compose.yaml --profile test config --quiet
./astribot/test/mediamtx/smoke.sh
```

Smoke 脚本只启动夹具服务，并在退出时删除其容器和具名卷。它生成合成 JPEG，并持续通过 RTSP/TCP 发布合成 H.264。随后测试完成 WHEP SDP 应答协商、会话创建与删除，并调用 MediaMTX Control API 对 WHEP 会话执行 list、kick、list。测试还会确认踢除读者会话后 RTSP 发布器仍可用。由于测试只验证 SDP 和会话行为，不建立 ICE 媒体连接，因此不向宿主机发布 ICE 端口。

夹具路径为 `demo-agent/demo-node/front`，对应 `{agent}/{node}/{view}`。私有 WHEP 地址为 `http://10.60.0.2:8889/demo-agent/demo-node/front/whep`。接入夹具媒体网桥的 Gateway 应访问同一个 MediaMTX 原生 HTTP endpoint。Control API 仅监听独立内部网桥上的 `10.61.0.2:9997`。WHEP、RTSP 和 Control API 端口均不映射到宿主机。

## 认证与会话行为

夹具禁用匿名访问。合成发布器只对配置路径拥有 `publish` 权限，且 RTSP 仅启用 TCP。独立 `whep-reader` 账号只对该路径拥有 `read` 权限。独立 `media-controller` 账号只拥有 `api` 权限，没有 `read` 或 `publish` 权限。夹具中的密码是测试专用明文，不得在该隔离环境以外复用。

WHEP `POST` 需要专用 Basic read 凭证。Smoke 测试确认匿名、错误、发布器和 Control API 凭证均收到 HTTP `401`。测试在 WHEP `DELETE` 请求中也发送同一 read 凭证并确认删除成功。MediaMTX v1.21.4 的 `Location` 中包含一个 UUID session secret，而 `ID` 响应头包含另一个供 Control API 使用的 UUID。该版本针对会话的 `DELETE` handler 使用 Location 中的 secret，不会重新检查 Basic 认证。Smoke 测试会显式记录这一上游行为。应将 `Location` 视为 bearer capability，保持 WHEP 私有，并由 Gateway 在两个方法上都发送 read 凭证。

Control API smoke 测试验证 Basic 认证、拒绝 reader 和 publisher 凭证，并依次执行 `GET /v3/webrtc/sessions/list`、`POST /v3/webrtc/sessions/kick/{id}` 和再次 list。API 仅绑定到专用 control 网桥，没有宿主机端口映射。夹具 API 账号可调用 MediaMTX 所支持的 API，因此只能交给受信任的 Gateway。

2026-11-27 的测试返回 HTTP `201`、`ID=856ae823-befe-40a4-b83a-457c6963e1e5` 和 `Location=/demo-agent/demo-node/front/whep/79cadae1-177c-4f4b-a3b8-16fad3a3791b`。两个 UUID 按预期不同。带凭证的 WHEP `DELETE` 返回 HTTP `200`；无凭证探测也返回 `200`，确认 Location UUID secret 是原生删除 capability。

Gateway 预期的 Control API 配置项为 `RLARK_CAMERA_WHEP_CONTROL_ORIGIN`、`RLARK_CAMERA_WHEP_CONTROL_USER` 和 `RLARK_CAMERA_WHEP_CONTROL_PASSWORD`。本夹具独立验证上游 endpoint 与凭证，不增加也不声称 Gateway 已实现这些配置。

## v1.21.4 契约记录

- WHEP 响应为 HTTP `201`，包含 SDP answer、`Location` 和 `ID` 响应头。
- `Location` 末尾是 UUID secret，与 `ID` session UUID 不同。
- WHEP `POST` 使用 MediaMTX 的 `read` 权限进行授权。WHEP `DELETE` 使用 Location 路径中的 UUID secret，不会重新执行 Basic 认证。
- Control API 提供 `GET /v3/webrtc/sessions/list` 和 `POST /v3/webrtc/sessions/kick/{id}`。kick 会移除指定 WebRTC 会话，并保留 RTSP 发布器路径。
- 该夹具覆盖协议与部署 smoke，不证明浏览器 ICE 连通性、生产授权、Gateway 恢复行为或实体设备兼容性。

上游参考：[MediaMTX v1.21.4 release](https://github.com/bluenviron/mediamtx/releases/tag/v1.21.4)、[v1.21.4 配置](https://github.com/bluenviron/mediamtx/blob/v1.21.4/mediamtx.yml) 和 [v1.21.4 Control API](https://github.com/bluenviron/mediamtx/blob/v1.21.4/api/openapi.yaml)。
