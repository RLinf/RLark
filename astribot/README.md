# astribot：本分叉加在 RLark 上的东西

本仓库 = 官方 RLark（基线提交 `9ad4a21`）+ 我们的场站层：场站节点和入网（Station）、机器人 Addon、站点镜像仓库、存储、发布工具。`git cat-file -e 9ad4a21:<路径>` 成功的是官方文件，否则是我们的（本仓库不带官方历史，先按 [upstream.md](docs/upstream.md) 取一次官方仓库）。官方文件尽量不动，合上游时冲突才少。

## 东西在哪

| 位置 | 内容 |
|---|---|
| `astribot/station/` | 场站节点：CRD 类型（`api/v1alpha1`）、station-controller、隧道拓扑计算（`network`）、节点安装器（`bootstrap`）、RBAC 和 DaemonSet 清单（`deploy`）、运维脚本（`hack`）。独立 Go module |
| `astribot/station-gateway/` | 每台节点的 WireGuard 隧道程序。独立 Go module |
| `astribot/station-forwarder/` | 到 NAS 的 SMB 转发。独立 Go module |
| `astribot/s1u/` | Addon `s1u-robot` 的镜像（tuner、orin、power、相机 video、clock）和测试 |
| `astribot/csi-rclone-ks3/` | rclone CSI 节点插件的 KS3 补丁和构建脚本 |
| `astribot/release/` | 构建（`build.sh`、`run.sh`）、发布文件（`releases/`）、环境（`environments/live.yaml`）、控制面 compose、镜像清单 |
| `astribot/docs/` | 本目录的文档；`swagger.yaml` 是我们加的接口（叠在官方 `apps/rlark/docs/api/swagger.yaml` 上，`apps/rlark/pkg/gateway/astribot_swagger_test.go` 检查） |
| `apps/rlark/astribot/` | 要和官方程序链接在一起的 Go 包：`release`、`rlarkctl`（release、station 命令）、`identity`、`idlerelay`、`inuse`、`nodeagent`、`nodepath`、`nodessh`、`resident`、`rollout`、`stationidentity`、`stationmember`、`storageref`；程序 `cmd/station-api`、`cmd/station-agent`、`cmd/csi-nodepath` |
| `apps/rlark-ui/src/astribot/` | 控制台我们的页面；测试 `apps/rlark-ui/tests/astribot-*.test.mjs` |
| `apps/rlark/pkg/addons/catalog/` | 我们加的 Addon：`csi-driver-nodepath`、`nvidia-gpu`、`package-proxy`、`registry-trust`、`s1u-robot`、`site-registry` |

官方 Go 包里我们加的文件都叫 `astribot_<名字>.go`，所以 `git ls-files '*astribot*'` 列出我们的代码。例外只有生成的代码和上面的 Addon 目录。改了哪些官方文件、为什么，见 [upstream.md](docs/upstream.md)。

## 不能随手改的字节

- **已发布的 Addon 版本不可变**。同一个“名字@版本”内容一变，Gateway 就回 409。改 `apps/rlark/pkg/addons/catalog/<名字>/` 下任何文件，同时升它 `addon.yaml` 的 `version`。
- **Orin 上联脚本**。`astribot/s1u/orin/orin.py --uplink-setup` 的输出必须逐字节等于 `apps/rlark/pkg/addons/catalog/s1u-robot/bootstrap/orin-uplink.sh`（`astribot/s1u/tests/test_s1u.py` 检查），所以那里的注释还写着旧路径 `extensions/s1u`。改 `orin.py` 的这段输出或 `s1u-orin-uplink.service`，就重新生成这个文件（`python3 astribot/s1u/orin/orin.py --uplink-setup > apps/rlark/pkg/addons/catalog/s1u-robot/bootstrap/orin-uplink.sh`）并升 `s1u-robot` 的版本。
- **生成物**（CRD、clientset、deepcopy、控制台的 `src/generated`、`reference/crd.md`）不手改：`make generate`，`make verify-generated` 检查。

## 构建和测试

在仓库根目录：

```bash
(cd api && go build ./... && go test ./...)
(cd apps/rlark && go build ./... && go vet ./... && go test ./...)
for m in astribot/station astribot/station-gateway astribot/station-forwarder; do (cd $m && GOWORK=off go vet ./... && GOWORK=off go test ./...); done
make verify-generated
(cd apps/rlark-ui && npm test && npx tsc -b)
python -m pytest astribot/s1u/tests -q
```

真实内核的网络测试只在一次性网络命名空间里跑：

```bash
(cd astribot/station-gateway && unshare -Urn env GOWORK=off STATION_NETNS_TEST=1 go test -run TestKernelReconciliation ./...)
(cd astribot/station-forwarder && unshare -Urn env GOWORK=off STATION_NETNS_TEST=1 go test -run TestFirewallIdempotence ./... && unshare -Urnm sh tests/network-namespace.sh)
```

`s1u-robot` 的镜像：`docker build -f astribot/s1u/Dockerfile .`；它的渲染测试 `cd apps/rlark && go test ./pkg/addons -run S1U`。

`astribot/station` 不在仓库的 Go workspace 里，改了类型后：

```bash
cd astribot/station
GOWORK=off ../../api/bin/controller-gen-v0.18.0 object paths=./api/v1alpha1/...
GOWORK=off ../../api/bin/controller-gen-v0.18.0 crd:crdVersions=v1 paths=./api/v1alpha1/... output:crd:artifacts:config=./config/crd/bases
```

## 文档

| 文档 | 内容 |
|---|---|
| [architecture.md](docs/architecture.md) | 各部件和它们怎么连：控制面、成员集群、Station、Addon、存储、镜像仓库、机器人占用、Fleet 怎么用 RLark |
| [install-join.md](docs/install-join.md) | 新建集群、机器入网、删除节点、重新入网、入网排障 |
| [release.md](docs/release.md) | 构建和上线、发布的 13 个阶段、快速上线、回退 |
| [upstream.md](docs/upstream.md) | 跟官方 RLark：改了哪些官方文件、怎么合入上游新版本 |
| [troubleshooting.md](docs/troubleshooting.md) | 日志在哪、怎么看集群、已知故障和修法 |
