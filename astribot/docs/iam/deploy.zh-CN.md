# Astribot IAM 部署与运维

Astribot IAM（`astribot-iam`）是 RLark 控制台之上的一层独立身份与访问管理：人类账号、会话和审计存在控制面 PostgreSQL 的 `astribot_iam_*` 表里，密码只存 bcrypt 哈希，不再使用安装时随机生成、明文放进 `rlark-ui-auth` 的共享密码登录身份与访问页。RLark 原生代码未改动；本功能全部在 `astribot/` 目录与 Astribot 发布文件里。

## 当前形态（影子上线）

本次发布是**影子上线**：nginx 只把 `/api/iam/*` 转给 `astribot-iam`，`/`、`/admin` 与其余 `/api/*` 的登录和鉴权仍是 RLark 原生网关的。也就是说：

- 控制台主入口仍是原生登录页（`rlark-ui-auth` 的 admin/user）。
- 管理员先原生登录，再打开 **身份与访问**（`/admin/iam`），用 IAM 账号在该页内登录一次（见下）。这一步的二次登录是影子阶段的已知限制，前置切主入口（nginx `auth_request` + `/login`）之后消除。
- `/internal/authorize` 已就绪但还没有调用方；在 nginx 接入 `auth_request` 前，它返回的判定不影响现网。

## 组件与文件

| 组件 | 位置 |
| --- | --- |
| IAM 服务（Go） | `apps/rlark/astribot/iam/`，入口 `apps/rlark/astribot/cmd/astribot-iam` |
| 控制台页面 | `apps/rlark-ui/src/astribot/iam/`（管理员控制台的「身份与访问」页，`/admin/iam`） |
| 发布接线 | `astribot/release/rlark.Dockerfile`、`astribot/release/images`、`astribot/release/control-plane/rlark.compose.yaml`、`astribot/release/rlark-ui.nginx.conf`、`astribot/release/iam-provision.sh` |
| 服务账号供给 | `astribot/release/iam-provision.sh`（创建 `astribot-iam-admin/operator/user` 三个网关 service account，明文 token 只写进 `${R}/iam/upstream-tokens.json`，0600） |

compose 里 `astribot-iam` 只在内部网络监听 8081，不发布宿主端口；`rlark-ui` 通过 `ASTRIBOT_IAM_URL` 转发。网关的 `19000` 端口保持原样（Fleet 依赖），后续如需收敛用网络策略限制来源。

## 首次部署

1. 正常走发布（`astribot/release/build.sh` + `rlarkctl release apply`）。apply 的 console 阶段会按顺序启动 `astribot-iam`、`rlark-ui`；首次启动自动建表（`astribot_iam_users/sessions/audit/schema_migrations`，advisory lock 保护）并引导两个默认账号。
2. 在管理机上以 root 供给上游 service account（登录网关需要它；脚本幂等，token 已存在则直接退出）：

   ```bash
   sudo env R=/home/cailiang/.local/share/rlark-runtime \
     astribot/release/iam-provision.sh
   ```

3. 供给后重启一次 IAM 让它读到 token 文件，并连带重启控制台（nginx 启动时才解析地址）：

   ```bash
   sudo docker compose -p rlark -f /var/lib/rlark/rlark.compose.yaml \
     --env-file /var/lib/rlark/control-plane.env up -d --no-deps astribot-iam rlark-ui
   ```

4. 验证：`curl -fsS http://127.0.0.1:80/api/iam/healthz`（经 nginx）；浏览器打开 `http://<控制面>/admin/iam`，用 `admin / astribot` 登录 IAM 页。

## 默认账号与首次改密

| 账号 | 初始密码 | 角色 |
| --- | --- | --- |
| `admin` | `astribot` | admin |
| `user` | `123` | operator |

两个默认密码都是弱口令：首次登录后立即在 **个人资料 → 修改密码** 改掉（新密码至少 8 位）。管理员重置他人密码不需要旧密码，重置会撤销目标用户的全部会话。

## 日常操作

- **用户/角色**：管理员在「身份与访问 → 用户」新建（admin/operator/viewer）、编辑显示名与角色、停用/启用、删除、重置密码。operator 只能读写业务面，viewer 只读；IAM 管理接口仅 admin。
- **审计**：「审计」页记录登录成败、登出、用户增删改、重置密码、改密、授权拒绝；按操作者/动作/结果/时间过滤。
- **会话**：会话有效期 7 天；改密、重置、停用、撤销都会立即失效相关会话。已建立的 WebSocket 终端不受影响，随下一次握手失效。
- **上游 token 轮换**：删除或泄露 `${R}/iam/upstream-tokens.json` 后重跑 `iam-provision.sh`（对已存在且角色一致的账号走轮换接口），再重启 `astribot-iam`。IAM 只存哈希不存明文的只有它自己的会话；上游 token 文件是唯一明文落盘点，备份与权限按 0600 管理。

## 备份与恢复

控制面 PostgreSQL 的常规备份自动覆盖 `astribot_iam_*` 三张表；恢复数据库即恢复全部账号与会话（会话按 `expires_at` 自然过期）。`${R}/iam/upstream-tokens.json` 不在数据库里，需要单独纳入备份或按上文轮换。

## 回滚

`rlarkctl release --back` 走 console 阶段回退，`astribot-iam` 与 `rlark-ui` 一起回到上一版；`astribot_iam_*` 表保留不删，回滚后原生登录照常工作。

## 已知限制

- 影子阶段存在双重登录（原生控制台 + IAM 页）；前置切主入口后消除。
- 原生控制台的「退出登录」只清它自己的令牌，不撤销 IAM 会话；在「身份与访问」页内退出才走 IAM 登出。
- IAM 不改变 RLark 的租户/资源归属模型；同角色用户看到的业务资源范围与原生一致。
- 持有网关 service account token 或能直连 19000 的主体仍可绕过 IAM（机器接口按计划保留给 Fleet）。
- 不接 SSO/OIDC；登录失败按「账号+IP」退避（5 次锁 30 秒）。
