# Mihomo Control Plane (mihomo-cp)

[mihomo](https://github.com/MetaCubeX/mihomo) 代理的 Web 管理面板。通过结构化 UI 或原始 YAML 编辑器修改配置，使用 mihomo 二进制校验，一键发布变更。

[English](./README.md)

## 功能特性

- **代理节点管理** — 创建、编辑、删除、复制、排序代理节点（ss、vmess、vless、trojan、hysteria2、tuic 等）
- **策略组管理** — Selector、URLTest、Fallback、LoadBalance、Relay 类型策略组，支持拖拽排序成员
- **规则与规则集** — 完整的规则 CRUD（类型/匹配值/目标），规则集提供者管理
- **系统配置** — 通用设置、TUN、DNS、外部控制器等结构化配置界面
- **配置编辑器** — 直接编辑原始 YAML，与发布流程无缝集成
- **发布中心** — 差异预览、校验（结构化检查 + mihomo 二进制验证）、发布、回滚、带日期前缀的版本历史（如 `20260324-1`）
- **运行状态监控** — 实时连接查看/搜索/关闭，策略组延迟测试与切换，运行中的规则/规则集，支持从连接快捷添加规则
- **中英文切换** — 右上角下拉菜单一键切换语言
- **认证与权限** — JWT 登录，admin/readonly 角色，用户管理，修改密码
- **单文件部署** — 前端通过 `embed.FS` 嵌入，无需单独的 Web 服务器

## 技术栈

| 层级   | 技术                                                |
|--------|-----------------------------------------------------|
| 后端   | Go 1.22+，`net/http` 标准库路由，SQLite (modernc.org/sqlite) |
| 前端   | React 19，TypeScript，Vite，Tailwind CSS，shadcn/ui |
| 状态管理 | Zustand（认证、草稿、国际化）                        |
| HTTP   | ky（前端），标准库（后端）                            |
| 认证   | JWT（access + refresh token），bcrypt 密码加密       |

## 快速开始

### 前置依赖

- Go 1.22+
- Node.js 20+ 和 npm（或 pnpm）
- mihomo 二进制文件（用于配置校验）

### 构建

```bash
# 一键构建前后端
make build

# 或分步构建：
cd web && npm install && npm run build
cd .. && go build -o mihomo-cp ./cmd/mihomo-cp
```

### 交叉编译（Linux）

```bash
cd web && npm run build && cd ..
GOOS=linux GOARCH=amd64 go build -o mihomo-cp ./cmd/mihomo-cp
```

### 运行

```bash
./mihomo-cp -host 0.0.0.0 -port 8080 -db ./data/mihomo-cp.db
```

首次启动时通过 UI 创建管理员账户。

### 配置

登录后前往 **设置** 页面配置以下参数：

| 配置项           | 说明                        | 示例                       |
|------------------|-----------------------------|----------------------------|
| 配置文件路径     | mihomo 的 config.yaml 路径  | `/etc/mihomo/config.yaml`  |
| 工作目录         | mihomo 工作目录             | `/etc/mihomo`              |
| 二进制文件路径   | mihomo 可执行文件路径       | `/usr/local/bin/mihomo`    |
| 控制器地址       | mihomo 外部控制器端点       | `http://127.0.0.1:9090`   |
| 控制器密钥       | mihomo 控制器 API 密钥      | `your-secret`              |

### Docker

```bash
docker build -t mihomo-cp .
docker run -d -p 8080:8080 -v ./data:/app/data mihomo-cp
```

## 项目结构

```
cmd/mihomo-cp/          # 程序入口
internal/
  handler/              # HTTP 处理器（认证、代理、策略组、规则、发布、运行时、设置）
  middleware/           # JWT 认证、管理员权限、请求日志
  model/               # 数据模型（User、Proxy、ProxyGroup、Rule、RuleProvider 等）
  server/              # HTTP 服务器启动与路由注册
  service/             # 业务逻辑（认证、配置渲染、校验、发布、mihomo 客户端）
  store/               # SQLite 数据访问层，自动迁移
web/
  src/
    api/               # API 客户端（基于 ky，类型化接口）
    components/        # React 组件（布局、代理、规则、发布、运行时、公共）
    i18n/              # 国际化（en.ts、zh.ts）
    pages/             # 路由页面（概览、代理、规则、设置、发布、运行时）
    stores/            # Zustand 状态管理（认证、草稿）
```

## 工作流程

1. **编辑** — 通过 UI 修改代理节点、策略组、规则、系统配置，或直接编辑 YAML
2. **预览** — 前往发布中心查看草稿与当前运行配置的差异
3. **校验** — 结构化校验（悬空引用、循环依赖检测）+ mihomo 二进制验证
4. **发布** — 写入配置文件并重载 mihomo；失败时自动回滚
5. **监控** — 查看实时连接、代理延迟、运行中的规则

## API 参考

所有接口以 `/api/` 为前缀，除特殊说明外均需认证。

| 方法   | 路径                              | 说明                      |
|--------|-----------------------------------|---------------------------|
| POST   | `/auth/login`                     | 登录（公开）              |
| POST   | `/auth/refresh`                   | 刷新令牌（公开）          |
| GET    | `/auth/me`                        | 当前用户信息              |
| PUT    | `/auth/change-password`           | 修改密码                  |
| GET    | `/proxies`                        | 代理节点列表              |
| POST   | `/proxies`                        | 创建代理节点              |
| GET    | `/proxy-groups`                   | 策略组列表                |
| POST   | `/proxy-groups`                   | 创建策略组                |
| GET    | `/rules`                          | 规则列表                  |
| POST   | `/rules`                          | 创建规则                  |
| GET    | `/rule-providers`                 | 规则集列表                |
| GET    | `/system-config`                  | 获取系统配置              |
| PUT    | `/system-config`                  | 更新系统配置              |
| GET    | `/publish/status`                 | 草稿变更状态              |
| GET    | `/publish/preview`                | 预览草稿 YAML 及差异      |
| POST   | `/publish/validate`               | 校验草稿                  |
| POST   | `/publish`                        | 发布草稿                  |
| POST   | `/publish/rollback`               | 回滚到上次成功版本        |
| GET    | `/publish/history`                | 发布历史记录              |
| GET    | `/runtime/connections`            | 实时连接列表              |
| GET    | `/runtime/proxies`                | 运行中的代理状态          |
| GET    | `/runtime/rules`                  | 运行中的规则              |
| GET    | `/runtime/providers`              | 运行中的规则集            |
| GET    | `/settings`                       | 应用设置                  |
| PUT    | `/settings`                       | 更新应用设置              |
| GET    | `/settings/config-yaml`           | 获取配置 YAML（草稿或文件）|
| PUT    | `/settings/config-yaml`           | 保存配置 YAML 草稿        |
| DELETE | `/settings/config-yaml`           | 清除配置 YAML 草稿        |
| GET    | `/settings/users`                 | 用户列表（仅管理员）      |
| POST   | `/settings/users`                 | 创建用户（仅管理员）      |

## 许可证

MIT
