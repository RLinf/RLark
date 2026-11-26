# Astribot IAM 后端

`astribot-iam` 是控制台主入口背后的身份服务（见 `astribot/docs/iam/deploy.zh-CN.md`）。它将人类用户、不透明会话和审计事件存入 PostgreSQL。它不负责创建 Gateway service account，也不提供控制台 UI。

## 运行

运行 `astribot/cmd/astribot-iam`，通过 `--db-config` 指向现有 RLark PostgreSQL 配置。服务启动时使用 advisory transaction lock 执行版本化 `astribot_iam_*` 迁移，并幂等引导 `admin` / `astribot`（角色 `admin`，ID `u_admin`）和 `user` / `123`（角色 `operator`，ID `u_user`）。引导密码使用 bcrypt cost 12 哈希。首次登录后应修改默认密码。新建和重置密码至少为 8 个字符，且不超过 bcrypt 的 72 字节输入上限。

默认监听地址为 `:8081`，`--session-ttl` 默认 `168h`。登录签发 32 字节不透明 token，仅保存其 SHA-256 摘要，并设置 HttpOnly、SameSite=Lax 的 `astribot_session` Cookie。TLS 在服务前终止时应设置 `--cookie-secure=true`。

`--upstream-tokens` 读取含 `admin`、`operator`、`user` 字符串字段的 JSON 文件，权限必须为 0600 或更严格。缺少该文件时登录和 IAM 管理接口仍可用，`/internal/authorize` 对业务调用返回 503。服务不会读取 `rlark-ui-auth`。

设置 `--submission-signing-key` 指向权限为 0600 或更严格的 PKCS#8 Ed25519 私钥 PEM，即可为任务和工作流创建请求签发身份声明。声明经 nginx 传给 Gateway，由 Gateway 使用配对公钥验证。

## HTTP API

- `POST /api/iam/login` 和 `POST /api/v1/auth/login` 接收 `{ "username": "...", "password": "..." }`，返回不透明 token、角色、过期时间和不含密码哈希的用户 DTO。
- `/api/iam/logout`、`/api/iam/me`、`/api/iam/password`、`/api/iam/sessions`、`/api/iam/sessions/revoke-others` 提供当前用户的会话和资料操作。撤销其他会话时会保留当前请求所用会话。
- `GET /api/iam/session` 对带有会话（cookie 或 Bearer）的请求返回该会话的 token、角色和用户，应答带 `Cache-Control: no-store`：只有 cookie、没有 token 的控制台标签页用它重新进入。
- `/api/iam/users` 和 `/api/iam/audit` 仅管理员可访问。重置密码会撤销目标用户的全部会话。
- `GET /internal/admit` 为已认证会话放行控制台页面的 GET 或 HEAD 请求。页面访问不记审计。
- `GET /internal/authorize` 回答 nginx 的 `/api/` location 匹配到的请求。所有地址都按接口调用进行方法和路径判定；不是有效接口路径的地址一律拒绝。GET/HEAD 可以只带 cookie，其余方法必须带 Bearer。未知路由 fail closed，授权成功时只通过 `X-IAM-Upstream` 返回所选上游 token。

所有 IAM 写接口都要求 Bearer 会话 token。授权接口允许 GET/HEAD 使用会话 Cookie，以支持浏览器终端握手。登录和 IAM 管理接口不会返回上游 token。

## 验证范围

包内测试通过测试 Store 覆盖权限、密码哈希、不透明登录 token、过期、撤销、会话恢复和用户名唯一性。迁移及 bootstrap 需要 PostgreSQL，应在具备 PostgreSQL 的环境中执行集成验证。
