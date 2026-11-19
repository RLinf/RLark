# Addon 开发指南

## 目录结构

每个 addon 是 `catalog/` 下的一个子目录，包含 `addon.yaml` 和 `manifests/` 目录：

```
catalog/
└── my-addon/
    ├── addon.yaml          # addon 元信息 + 参数定义
    └── manifests/          # Go template YAML 文件
        ├── deployment.yaml
        └── service.yaml
```

无需修改任何 Go 代码，`catalog.go` 会通过 `//go:embed` 自动发现并注册 `catalog/*/addon.yaml`。

## addon.yaml 字段说明

| 字段 | 说明 |
|------|------|
| `name` | 唯一标识（与目录名一致） |
| `displayName` | 前端展示名称 |
| `category` | 分类：`storage` \| `monitoring` \| `device-plugin` \| `network` \| `other` |
| `version` | 版本号 |
| `description` | 功能描述 |
| `icon` | 图标标识（前端展示，如 `database`/`cpu`/`monitor`/`package`） |
| `parameters` | 可配置参数列表（可选） |

### parameter 字段

| 字段 | 说明 |
|------|------|
| `name` | 参数 key（模板中通过 `.Values.<name>` 引用） |
| `displayName` | 前端展示名称 |
| `description` | 参数说明 |
| `type` | `string` \| `text` \| `enum` \| `bool` \| `int` |
| `default` | 默认值 |
| `options` | `enum` 类型的可选值 |
| `required` | 是否必填 |

## 模板变量

manifests 目录下的 YAML 文件使用 Go template 语法，可用变量：

| 变量 | 说明 |
|------|------|
| `.Values.<param>` | addon.yaml 中定义的参数值 |
| `.Namespace` | Addon CR 所在命名空间（即集群 ID） |
| `.AddonName` | Addon CR 的 name |
| `.AddonUID` | Addon CR 的 UID |
| `.AddonLabels` | 需注入的 label map（通常无需手动使用） |

Agent 会自动为所有渲染出的顶层资源注入 label `rlark.io/addon-name` 和 `rlark.io/addon-uid`，无需在模板中手动添加。

## 自定义模板函数

| 函数 | 说明 |
|------|------|
| `splitList sep s` | 按分隔符分割字符串为列表 |
| `indent n s` | 对每一行添加 n 个空格缩进 |

## 新增 addon 步骤

1. 在 `catalog/` 下创建新目录（与 addon name 一致）
2. 编写 `addon.yaml`
3. 在 `manifests/` 下编写 Go template YAML 文件
4. 重新编译即可生效