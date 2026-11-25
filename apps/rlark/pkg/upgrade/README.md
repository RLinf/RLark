# rlark-upgrade

`rlark-upgrade` 是一个完全自治的升级工具，基于 `rlarkadm` 实现自动升级和智能回滚。

## 特性

- **基于 rlarkadm**: 复用 rlarkadm 的完整升级逻辑
- **多阶段健康检查**: 基础检查 + 深度检查 + 稳定性观察
- **智能回滚**: 多种触发条件，自动回滚
- **完全自治**: Upgrade Pod 独立完成所有操作
- **持续监控**: 定期健康检查，连续失败自动回滚

## 架构

```
rlark-upgrade
├── cmd/rlark-upgrade/main.go     # 主入口
└── pkg/upgrade/
    ├── config.go                 # 配置定义
    ├── health.go                 # 健康检查器
    └── upgrader.go               # 升级逻辑（包装 rlarkadm）
```

## 升级流程（5 个阶段）

```
Stage 1: Precheck (rlarkadm 内置)
├── 部署配置验证
└── ...

Stage 2: Upgrade (rlarkadm 内置)
├── RBAC 更新
├── PVC 扩容
├── Service 更新
└── Workload 更新

Stage 3: Basic Health Check (rlarkadm 内置)
└── 组件就绪检查

Stage 4: Deep Health Check (rlark-upgrade 实现)
├── 组件就绪比例检查 (>90%)
├── Pod 健康检查
├── Crash Loop 检查
├── 服务可访问性检查
└── 持续监控（连续 3 次失败触发回滚）

Stage 5: Stability Observation (rlark-upgrade 实现)
├── 观察 2 分钟
└── 最终健康检查
```

## 健康检查项

| 检查项 | 阈值 | 说明 |
|--------|------|------|
| 组件就绪比例 | ≥90% | Deployment ready replicas / desired |
| 容器重启次数 | ≤5 | 单个容器最大重启次数 |
| 不可用 Pod 数 | 0 | 不健康的 Pod 数量 |
| 连续失败次数 | <3 | 健康检查连续失败次数 |

## 使用方法

### 环境变量

| 变量 | 说明 | 默认值 |
|------|------|--------|
| `DEPLOY_CONFIG` | 部署配置路径 | `/etc/rlark/deploy-config.yaml` |
| `ROLLBACK_CONFIG` | 回滚配置路径 | `/etc/rlark/rollback-deploy-config.yaml` |
| `AUTO_ROLLBACK` | 是否自动回滚 | `true` |
| `HEALTH_CHECK_TIMEOUT` | 健康检查超时 | `5m` |
| `HEALTH_CHECK_INTERVAL` | 健康检查间隔 | `10s` |

### 退出码

| 退出码 | 说明 |
|--------|------|
| 0 | 升级成功 |
| 1 | 升级失败，无回滚 |
| 2 | 升级失败，回滚成功 |
| 3 | 升级失败，回滚也失败 |

### Docker 运行

```bash
docker run \
  -e DEPLOY_CONFIG=/etc/rlark/deploy-config.yaml \
  -e AUTO_ROLLBACK=true \
  -v /path/to/config:/etc/rlark \
  -v /path/to/kubeconfig:/etc/rlark/kubeconfig \
  registry.rlark.io/rlark-upgrade:v0.2.0
```

### Kubernetes Job

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: upgrade-job
spec:
  template:
    spec:
      containers:
      - name: upgrade
        image: registry.rlark.io/rlark-upgrade:v0.2.0
        env:
        - name: DEPLOY_CONFIG
          value: /etc/rlark/deploy-config.yaml
        - name: AUTO_ROLLBACK
          value: "true"
        volumeMounts:
        - name: config
          mountPath: /etc/rlark
      volumes:
      - name: config
        configMap:
          name: upgrade-config
      restartPolicy: Never
```

## 构建

```bash
cd apps/rlark/build/rlark-upgrade
make build VERSION=v0.2.0
make push VERSION=v0.2.0
```

## 依赖

- rlarkadm: RLark 部署工具（核心升级逻辑）
- client-go: Kubernetes 客户端
