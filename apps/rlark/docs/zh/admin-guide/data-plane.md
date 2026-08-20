# GPU 集群接入

## 概述

接入 Kubernetes 数据面需要 Agent 证书及供 `rlarkadm` 使用的 `DeployConfig`。当前管理表单用于签发证书，不要求填写集群类型或区域，也不会生成 Shell 安装命令。

## 通过 UI 获取证书

### 第一步：签发集群证书

1. 登录管理平台 `http://<host>:5173/admin`。
2. 打开**集群管理** → **创建集群**。
3. 只需输入集群名称，例如 `my-cluster-01`。
4. 选择**签发证书**。

![创建集群](../../images/ui/admin-create-cluster.jpg)

签发后，页面会显示集群名称、Server 地址和完整部署 YAML，并将名称加入**已签发集群**；之后仍可展开该记录并再次复制 YAML。

!!! warning "保护 YAML"
    页面显示的 `agent-key` 是私钥。请安全保存复制的 YAML，且不要在其他集群复用。

### 第二步：完善部署 YAML

UI 输出与 `rlarkadm` 接受的 `DeployConfig` 一致：

```yaml
apiVersion: rlark.io/v1alpha1
kind: DeployConfig
plane: data
control-plane-address: https://<control-plane>:8443

cert:
  ca-cert: |
    -----BEGIN CERTIFICATE-----
    <CA 证书>
    -----END CERTIFICATE-----
  agent-cert: |
    -----BEGIN CERTIFICATE-----
    <Agent 证书>
    -----END CERTIFICATE-----
  agent-key: |
    -----BEGIN PRIVATE KEY-----
    <Agent 私钥>
    -----END PRIVATE KEY-----

kubernetes:
  kubeconfig: /path/to/kubeconfig.yaml
  agent-image: rlark-agent:latest
```

将 `kubernetes.kubeconfig` 替换为可向目标集群部署资源的 kubeconfig，并设置可用的 Agent 镜像。仅在启用需要共享 RLark 镜像的组件时添加可选的 `kubernetes.image`。所有可用字段参见[配置参考](../reference/configuration.md)。

### 第三步：安装数据面

将完善后的 YAML 保存为 `deploy-data-plane.yaml`，再从受信任的管理主机运行：

```bash
rlarkadm install -f deploy-data-plane.yaml
```

UI 不会生成此命令，也不会代为执行。

### 第四步：验证注册

1. 返回**集群管理** → **集群列表**或**节点管理**。
2. 等待 Agent 连接并同步节点。
3. 确认集群在线，且预期 Worker 节点已经出现。

![验证集群节点](../../images/ui/admin-clusters-nodes.jpg)

## 通过 CLI 获取证书

自动化场景可签发 Agent 证书并使用仓库维护的部署示例：

```bash
rlarkctl sign \
  --role=agent \
  --client-id=agent-my-cluster-1 \
  --output=/tmp/agent-certs

rlarkadm install -f apps/rlark/docs/examples/deploy-data-plane.yaml
```

安装前请更新示例中的 Server 地址、证书内容或路径、kubeconfig 和镜像。

## 添加调度元数据

使用节点管理的批量编辑器设置城市、一个或多个云算力/端算力/真机分类、GPU 型号或设备型号，也可批量 Cordon/Uncordon。字段存储在控制面的 Node CR 上，并会在 Agent 刷新 Kubernetes 发现状态时保留。

## 运行冒烟测试

从业务平台创建一个小型 Job，为 Worker 选择刚接入的集群并填写可用镜像后提交。确认 Worker 进入 Running、日志可查看；如果镜像提供 Shell，再确认 WebTerminal 可以打开。

## 故障排查

| 症状 | 排查方向 |
|------|---------|
| Agent 无法连接 | Server 地址、CA/Agent 证书是否匹配、出站网络 |
| 集群显示离线 | Agent Deployment 日志和证书有效期 |
| 节点未出现 | Agent 模式、本地 RBAC、节点控制器日志 |
| Job 无法调度 | 所选集群、节点选择器、资源申请、镜像拉取状态 |

## 注册管理

- 每个数据面集群分别签发 Agent 证书。
- 复制的部署 YAML 含有 `agent-key`，必须按 Secret 保护。
- 使用 `rlarkctl sign` 轮换证书，并重新部署受影响的 Agent。
