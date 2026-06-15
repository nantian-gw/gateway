# Nantian Gateway

<p align="center">
  <strong>面向生产的现代化 Kubernetes Gateway API 实现，采用分离式平面架构。</strong>
</p>

<p align="center">
  <a href="https://github.com/nantian-gw/gateway/blob/main/LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-blue.svg" alt="License"></a>
  <a href="https://go.dev/"><img src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go" alt="Go"></a>
  <a href="https://gateway-api.sigs.k8s.io/"><img src="https://img.shields.io/badge/Gateway_API-v1.5.1-326ce5?logo=kubernetes" alt="Gateway API"></a>
  <a href="https://nantian.dev"><img src="https://img.shields.io/badge/docs-nantian.dev-7c3aed" alt="Docs"></a>
</p>

> 📖 [English](README.md)

---

## 什么是 Nantian Gateway？

Nantian Gateway 是 [Kubernetes Gateway API](https://gateway-api.sigs.k8s.io/) 的一种实现，用于处理入口流量、API 路由和 AI 网关功能 —— 全部通过标准 Kubernetes 资源管理。没有自定义 CRD 用于路由，没有私有配置语言，只有 Gateway API。

**如果你用过 nginx ingress 或 Envoy Gateway** —— Nantian Gateway 做同样的事，但采用 Go 控制面 + Rust 数据面，目标是完整的 Gateway API v1.5.1 兼容性，支持 55 项特性。

### 为什么选择 Nantian Gateway？

| 痛点 | Nantian Gateway 的解决方案 |
|---|---|
| **厂商锁定** | 标准 Gateway API —— 无需修改路由定义即可切换实现 |
| **复杂的 AI 路由** | 内置 AI Gateway：多提供商代理、API 密钥、速率限制、PII 脱敏 |
| **可观测性不足** | Prometheus 指标 + Grafana 仪表板 + Admin API，开箱即用 |
| **大规模性能** | Rust 数据面 + xDS 推送 —— 亚毫秒级配置下发 |
| **自定义逻辑** | Wasm 插件系统，无需重新编译即可实现请求/响应钩子 |

### 架构总览

```
  用户 / 客户端
        │
        ▼
  ┌───────────┐
  │  数据面     │  ◄── Rust 代理（HTTP、gRPC、UDP、TLS）
  │  (Rust)    │      处理实时流量
  └─────┬─────┘
        │ gRPC xDS（双向流）
  ┌─────┴─────┐
  │  控制面     │  ◄── Go 进程
  │  (Go)      │      监听 Gateway API 资源
  └─────┬─────┘      转换 → xDS 配置 → 推送到数据面
        │
  ┌─────┴─────┐
  │ Kubernetes │  Gateway、HTTPRoute、GRPCRoute、TLSRoute…
  │    API     │
  └───────────┘
```

## 快速开始

使用本地 Kind 集群，5 分钟即可体验：

```bash
# 克隆并部署
git clone https://github.com/nantian-gw/gateway.git
cd gateway

# 启动 Kind 集群并部署 Nantian Gateway
./test/e2e/smoke/run.sh
```

然后创建你的第一条路由：

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: my-first-route
spec:
  parentRefs:
    - name: nantian-gateway
  rules:
    - backendRefs:
        - name: my-service
          port: 8080
```

完整教程请查看[快速开始指南](https://nantian.dev)。

## 安装

### Helm（推荐用于生产环境）

```bash
helm repo add nantian-gw https://chart.nantian.dev
helm install nantian-gw nantian-gw/nantian-gw \
  --namespace nantian-gw \
  --create-namespace
```

自定义配置请参考 [Helm Chart 文档](https://github.com/nantian-gw/helm-charts)。

### Kustomize

```bash
kubectl apply -k deploy/kubernetes/overlays/production
```

### 环境要求

- Kubernetes 1.28+
- 集群中已安装 [Gateway API CRDs](https://gateway-api.sigs.k8s.io/guides/#installing-gateway-api)

## 核心特性

### Gateway API v1.5.1 支持

| 路由类型 | 状态 |
|---|---|
| HTTPRoute | ✅ 完全支持 |
| GRPCRoute | ✅ 完全支持 |
| TCPRoute | ✅ 完全支持 |
| UDPRoute | ✅ 完全支持 |
| TLSRoute | ✅ 透传模式 |
| BackendTLSPolicy | ✅ 完全支持 |
| BackendLBPolicy | ✅ 完全支持 |

该表仅作能力级概览，不表示默认运行时已启用全部特性；默认运行时对应 `enableExperimentalGateway=false`。`ListenerSet`、`TLSRouteModeTerminate`、`TLSRouteModeMixed`、`UDPRoute` 需要启用 `enableExperimentalGateway=true`，精确状态请查看 [Gateway API 支持矩阵](docs/gateway-api-feature-support.md)。

### AI Gateway

通过单一端点将 AI 流量路由到多个提供商：

- **统一代理** —— 一个端点支持 OpenAI、Anthropic、Ollama
- **Token 计数与速率限制** —— 按用户、按模型的配额管理
- **API 密钥管理** —— 通过 Kubernetes Secret 集中存储凭证
- **PII 脱敏** —— 自动检测并遮蔽敏感字段
- **A/B 测试** —— 在不同模型或提供商之间分流流量

```yaml
apiVersion: gateway.nantian.dev/v1alpha1
kind: AIBackend
metadata:
  name: my-llm
spec:
  provider: openai
  model: gpt-4o
  apiKeySecretRef:
    name: openai-credentials
```

→ [AI Gateway 文档](docs/design/ai-gateway/)

### Wasm 插件系统

无需重新编译或重启即可扩展数据面逻辑：

- **请求/响应钩子** —— 修改请求头、响应体或状态码
- **wasmtime 运行时** —— 快速、沙箱化执行
- **支持多种语言** —— 用 Rust、Go、C 或 JavaScript 编译为 Wasm

→ [Wasm 插件文档](docs/design/wasm/)

### 可观测性

- **Prometheus 指标** —— 请求速率、延迟、按路由和后端的错误统计
- **Grafana 仪表板** —— 预置模板位于 `deploy/observability/`
- **Admin API** —— 运行时配置、健康检查、诊断接口

## 文档

| 你想… | 看这里 |
|---|---|
| 快速入门 | [快速开始](https://nantian.dev/getting-started/quick-start/) |
| 生产环境安装 | [安装指南](https://nantian.dev/installation/helm/) |
| 理解核心概念 | [概念](https://nantian.dev/concepts/) |
| 配置 AI Gateway | [AI Gateway 文档](docs/design/ai-gateway/) |
| 编写 Wasm 插件 | [Wasm SDK 文档](docs/design/wasm/) |
| 查看支持的特性 | [Gateway API 支持矩阵](docs/gateway-api-feature-support.md) |
| 参与贡献 | [CONTRIBUTING.md](CONTRIBUTING.md) |
| 报告 Bug | [Issues](https://github.com/nantian-gw/gateway/issues) |
| 查看路线图 | [ROADMAP.md](ROADMAP.md) |

[完整文档站点 →](https://nantian.dev)

## 开发

```bash
# 环境要求：Go 1.26+、Rust（数据面）、Kind（端到端测试）

# 生成 protobuf
make proto

# 构建
make build

# 运行单元测试
make test

# 运行性能测试
make benchmarks

# 运行合规性测试（需要 Kind）
make conformance
```

完整开发流程请参考 [CONTRIBUTING.md](CONTRIBUTING.md)。

## 相关项目

| 项目 | 描述 |
|---|---|
| [nantian-gw/dataplane](https://github.com/nantian-gw/dataplane) | Rust 数据面（HTTP 代理、xDS 客户端、AI Gateway、Wasm 运行时） |
| [nantian-gw/dashboard](https://github.com/nantian-gw/dashboard) | Next.js 管理控制台 |
| [nantian-gw/website](https://github.com/nantian-gw/website) | 文档站点 ([nantian.dev](https://nantian.dev)) |
| [nantian-gw/helm-charts](https://github.com/nantian-gw/helm-charts) | Kubernetes 部署 Helm Charts |
| [nantian-gw/proto](https://github.com/nantian-gw/proto) | 控制面与数据面共享的 protobuf 协议 |

## 项目状态

Nantian Gateway 正在积极开发中。目前已有可工作的控制面、数据面、管理接口、Kind 冒烟测试、合规性测试流程和生产环境部署覆盖。尚未被认定为官方 Gateway API 实现。

## 许可证

Apache 2.0 —— 详见 [LICENSE](LICENSE)。
