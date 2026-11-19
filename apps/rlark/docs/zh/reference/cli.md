# CLI 参考

RLark 提供以下命令行工具。

## rlarkadm

部署工具。用于安装和卸载 RLark 控制面和数据面组件。

### rlarkadm install

安装 RLark 组件。

| 参数 | 缩写 | 类型 | 默认值 | 说明 |
|------|------|------|--------|------|
| `--install-conf` | `-f` | string | `""` | 安装配置文件路径（必填） |

**示例：**
```bash
rlarkadm install -f deploy-control-plane.yaml
```

### rlarkadm uninstall

卸载 RLark 组件。

| 参数 | 缩写 | 类型 | 默认值 | 说明 |
|------|------|------|--------|------|
| `--uninstall-conf` | `-f` | string | `""` | 卸载配置文件路径（必填） |
| `--purge` | | bool | `false` | 同时删除 namespace 和数据目录 |
| `--yes` | `-y` | bool | `false` | 跳过确认提示 |

!!! warning "`--purge` 操作不可逆"
    使用 `--purge` 将永久删除 namespace 和所有相关数据。

**示例：**
```bash
rlarkadm uninstall -f deploy-control-plane.yaml --purge -y
```

### 全局参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--log-level` | string | `info` | 日志级别：debug、info、warn、error |

## rlarkctl (rlark-server-cli)

Server 命令行工具。用于证书管理和代理访问。

### 全局参数

| 参数 | 类型 | 默认值 | 说明 |
|------|------|--------|------|
| `--server-address` | string | `https://localhost:8443` | Server 地址 |
| `--server-hostname` | string | `""` | 服务器 TLS 预期主机名 |
| `--client-cert` | string | `""` | 客户端 TLS 证书路径 |
| `--client-key` | string | `""` | 客户端 TLS 私钥路径 |
| `--ca-cert` | string | `""` | CA 证书路径 |
| `--insecure-skip-tls-verify` | bool | `false` | 跳过 TLS 证书验证 |

### rlarkctl sign

签发 Agent 证书。

| 参数 | 缩写 | 类型 | 默认值 | 说明 |
|------|------|------|--------|------|
| `--role` | `-r` | string | `agent` | 证书角色：admin、peer、agent |
| `--client-id` | `-c` | string | `example-client-id` | Agent 角色的 Client ID |
| `--output` | `-o` | string | `""` | 证书和私钥输出目录 |

**示例：**
```bash
rlarkctl sign \
  --role=agent \
  --client-id=agent-my-cluster-1 \
  --output=/tmp/agent-certs
```

### rlarkctl revoke

吊销证书。

| 参数 | 缩写 | 类型 | 默认值 | 说明 |
|------|------|------|--------|------|
| `--cert-type` | `-t` | string | `""` | 证书类型：x509、ssh（必填） |
| `--serial-number` | `-s` | string | `""` | 证书序列号（必填） |
| `--subject-key-id` | `-k` | string | `""` | 证书 Subject Key ID（必填） |
| `--reason` | `-r` | string | `""` | 吊销原因（可选） |

**示例：**
```bash
rlarkctl revoke \
  --cert-type=x509 \
  --serial-number=12345 \
  --subject-key-id=abc:def:123
```

### rlarkctl proxy-curl

通过 Server 代理端点发送 HTTP 请求。

**示例：**
```bash
rlarkctl proxy-curl https://internal-service:8080/api/status
```