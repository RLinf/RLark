# Astribot IAM 部署与运维

Astribot IAM（`astribot-iam`）是 RLark 控制台的身份层：人类账号、会话和审计存在控制面 PostgreSQL 的 `astribot_iam_*` 表里，密码只存 bcrypt 哈希，没有人再用安装时随机生成、明文放进 `rlark-ui-auth` 的共享密码登录（它们只用来供给网关 service account，见下文）。RLark 原生代码未改动；本功能全部在 `astribot/`、`apps/rlark/astribot/` 与 `apps/rlark-ui/src/astribot/` 里。

## 当前形态：IAM 是主入口

自 2026-11-25 起，控制台的 nginx（`astribot/release/rlark-ui.nginx.conf`）只放行通过 IAM 的页面请求和业务 `/api/*` 调用（`auth_request` → `/internal/authorize`），人们在 `/login` 登录：

- `/login` 是控制台镜像里的静态页（`astribot/release/login.html`）。没有会话时访问页面，得到 `302 /login?next=<原地址>`；登录后浏览器回到该地址。`/admin` 下的地址只有 admin 角色才会被带回。`next` 读取的是查询串余下的全部内容，因此它必须是最后一个参数。
- 由 `astribot/release/rlark-ui.Dockerfile` 构建的控制台镜像设置了 `VITE_IAM_PORTAL=1`，由此启用门户守卫（`apps/rlark-ui/src/astribot/iam/portal.ts`）。守卫会：
  - 在标签页只有 cookie、没有 token 时（新标签页、书签）通过 `GET /api/iam/session` 恢复会话，不再要求重新输入密码；
  - 接管控制台自己的「退出登录」（每个页面右上角的头像菜单）：撤销会话（`POST /api/iam/logout`）并回到 `/login`；
  - 在调用返回 401 时（会话过期，或因改密、重置、停用而失效）把控制台带到 `/login`；
  - 从不显示 RLark 原生登录页。
- 以其他方式构建的控制台（原生的 `apps/rlark-ui` 镜像）不设置该变量，保留原生登录。
- 业务调用到达网关时带的是该角色的 service account token，由 nginx 注入；浏览器持有的 token 本身不授权任何操作。
- `/api/iam/*` 直接转给 IAM。`POST /api/v1/auth/login` 仍不经会话检查直接转给网关（原生登录交换）；原生账号已打不开控制台，因为控制台的页面需要 IAM 会话。

## 组件与文件

| 组件 | 位置 |
| --- | --- |
| IAM 服务（Go） | `apps/rlark/astribot/iam/`，入口 `apps/rlark/astribot/cmd/astribot-iam` |
| 门户守卫与控制台页面 | `apps/rlark-ui/src/astribot/iam/`（`portal.ts`；管理员控制台的「身份与访问」页，`/admin/iam`） |
| 登录页 | `astribot/release/login.html`（复制进控制台镜像） |
| 发布接线 | `astribot/release/rlark.Dockerfile`、`astribot/release/astribot-iam.Dockerfile`、`astribot/release/rlark-ui.Dockerfile`、`astribot/release/images`、`astribot/release/control-plane/rlark.compose.yaml`、`astribot/release/rlark-ui.nginx.conf` |
| 服务账号供给 | `astribot/release/iam-provision.sh`（创建 `astribot-iam-admin/operator/user` 三个网关 service account，明文 token 只写进 `${R}/iam/upstream-tokens.json`，0600） |

compose 里 `astribot-iam` 只在内部网络监听 8081，不发布宿主端口；`rlark-ui` 通过 `ASTRIBOT_IAM_URL` 转发。服务的镜像取 `ASTRIBOT_IAM_IMAGE`，缺省回落到发布里 `rlark-server` 的镜像（其中带有同一个二进制）。网关的 `19000` 端口保持原样（Fleet 依赖），后续如需收敛用网络策略限制来源。

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

4. 在管理机上验证：没有会话时页面被送往登录页，登录接口有应答。

   ```bash
   curl -s -o /dev/null -D - http://127.0.0.1/ | grep -iE '^(HTTP|location)'   # 302，Location: /login?next=/
   curl -fsS -X POST -H 'Content-Type: application/json' \
     -d '{"username":"admin","password":"astribot"}' -o /dev/null http://127.0.0.1/api/iam/login
   ```

   服务自己的 `/healthz` 只在容器网络内可达（`docker exec rlark-ui wget -qO- http://astribot-iam:8081/healthz`）。然后浏览器打开 `http://<控制面>/`：登录页；「身份与访问」（`/admin/iam`）属于管理员，用 `admin / astribot` 登录。

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
- **单独滚动 IAM、登录页或门户守卫**：`astribot-iam` 另有独立镜像（`astribot/release/astribot-iam.Dockerfile`；服务只有一个二进制），控制台镜像由 `astribot/release/rlark-ui.Dockerfile` 构建。把镜像按摘要发布到控制面的拉取仓库，写进发布的 `images`（`astribot-iam`、`rlark-ui`），再执行 `rlarkctl release apply`：console 阶段先重建 `astribot-iam`，再重建 `rlark-ui`（nginx 启动时才解析地址）。滚动 IAM 不会让任何人掉线：会话存在 PostgreSQL 里。
- **上游 token 轮换**：删除或泄露 `${R}/iam/upstream-tokens.json` 后重跑 `iam-provision.sh`（对已存在且角色一致的账号走轮换接口），再重启 `astribot-iam`。IAM 只存哈希不存明文的只有它自己的会话；上游 token 文件是唯一明文落盘点，备份与权限按 0600 管理。

## 备份与恢复

控制面 PostgreSQL 的常规备份自动覆盖 `astribot_iam_*` 三张表；恢复数据库即恢复全部账号与会话（会话按 `expires_at` 自然过期）。`${R}/iam/upstream-tokens.json` 不在数据库里，需要单独纳入备份或按上文轮换。

## 回滚

`rlarkctl release apply --back` 按阶段逆序回退，`astribot-iam` 与 `rlark-ui` 一起回到上一版；`astribot_iam_*` 表保留不删。回到主入口切换之前的控制台后，重新使用原生登录（`rlark-ui-auth` 的账号）。

## 已知限制

- 所有人类账号都在 IAM 里：原生登录页不可达，原生 admin/user 账号只用于供给网关 service account。
- IAM 不可用时，控制台的页面和业务调用由 nginx 应答 5xx；登录页是静态页仍能打开，但 IAM 恢复前登录会失败。
- IAM 不改变 RLark 的租户/资源归属模型；同角色用户看到的业务资源范围与原生一致。
- 持有网关 service account token 或能直连 19000 的主体仍可绕过 IAM（机器接口按计划保留给 Fleet）。
- 不接 SSO/OIDC；登录失败按「账号+IP」退避（5 次锁 30 秒）。
