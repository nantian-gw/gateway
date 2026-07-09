# Kubernetes 最佳实践审查报告

审查范围:`Dockerfile`、`deploy/kubernetes/**`、RBAC、NetworkPolicy、CRD、`cmd/manager` 启动逻辑。
已忽略文档(README / AGENTS.md / docs/,内容可能与代码不符)。
审查日期:2026-07-09

> 说明:#1、#5 涉及运行时行为推断;#1 已通过阅读 `internal/infrastructure/services.go` 确认(控制面为数据面动态创建 Service/EndpointSlice,静态 dataplane 即共享数据面)。

---

## 实现进度(2026-07-09)

已实现并通过 `kustomize build`(全 overlay)+ `go build -mod=readonly` 验证:
- [x] #1 dataplane NetworkPolicy 放行 80/443
- [x] #2 dataplane allowPrivilegeEscalation → false
- [x] #3 三个 CRD 补 status 子资源 + 结构化 conditions
- [x] #4 namespace 加 restricted PSA 标签
- [x] #6 HPA 加 behavior + 生产 overlay 删除静态 replicas
- [x] #8 运行时镜像换 distroless static:nonroot(去 apt / WORKDIR)
- [x] #9 RBAC:secrets 集群级只读 + 命名空间级写 Role
- [x] #12 删除空的 `sysctls:` 键
- [x] #14 构建改 `-mod=readonly`
- [x] #16 新增 PriorityClass 并在两个 plane 引用
- [x] #18 dataplane PDB 改 maxUnavailable: 1
- [x] #5 探针对齐(方案 B):liveness/startup 指向 :18083,readiness 保留 admin
- [x] #11 :18083 现由 kubelet 探针使用(不再是"暴露但无人用")

已推迟(见下方各条 "状态"):
- [ ] #7 dataplane 资源配比 —— 属环境相关容量决策,建议你按压测结果定
- [ ] #10 NetworkPolicy egress / default-deny —— 依赖集群 apiserver 端点,易误伤,建议单独处理
- [ ] #13 镜像 digest 固定 —— 与镜像仓库/发布流程相关
- [ ] #15 dataplane metrics 独立端口 —— 需改数据面代码(不在本仓库)
- [ ] #17 topologySpread 改 DoNotSchedule —— 小集群可能排不下,可选

---

## 高优先级(High)

### [x] 1. dataplane 的 NetworkPolicy 会挡掉数据面流量
文件:`deploy/kubernetes/base/services-networkpolicy.yaml`
**状态:已修复。** 新增 ingress 放行 80/443(from 0.0.0.0/0 + ::/0),admin 19080 仍限命名空间内。
若 Gateway 使用非标准监听端口,需在此追加对应端口。

### [x] 2. dataplane `allowPrivilegeEscalation: true`
文件:`deploy/kubernetes/base/dataplane.yaml`
**状态:已修复。** 改为 `false`,保留 `NET_BIND_SERVICE`(足以绑定 80/443),现满足 restricted PSS。

### [x] 3. 三个 CRD 都缺 `status` 子资源
文件:`aiservice-crd.yaml` / `tokenpolicy-crd.yaml` / `wasmplugin-crd.yaml`
**状态:已修复。** 每个 version 补 `subresources.status: {}`;`status.conditions` 改为标准
`metav1.Condition` 结构化 schema + `x-kubernetes-list-type: map` / `list-map-keys: [type]`。
已确认 `internal/status/reconciler_*.go` 用 `.Status().Patch`,与子资源语义一致。

---

## 中优先级(Medium)

### [x] 4. namespace 缺 Pod Security Admission 标签
文件:`deploy/kubernetes/base/namespace-gatewayclass.yaml`
**状态:已修复。** 加 `enforce/audit/warn: restricted`(+ `enforce-version: latest`)。
两个 plane 均已满足 restricted。

### [x] 5. startup gate 没接到 kubelet 探针
文件:`deploy/kubernetes/base/controlplane.yaml`
**状态:已修复(方案 B)。**
- startupProbe → `:18083`(healthz)`/readyz`:controller-runtime 的 /readyz 含 `startupGate.Check`,
  现在 startup gate 真正门控 rollout。
- livenessProbe → `:18083`(healthz)`/healthz`:controller-runtime ping。
- readinessProbe → 保留 admin(`:18081`)`/readyz`:数据面 snapshot 就绪语义。
- 注意:controller-runtime 只提供 `/healthz`、`/readyz`(无 `/livez`),故路径一并改正。

### [x] 6. HPA 与 Deployment 静态 `replicas` 冲突
文件:`addons/dataplane-hpa/hpa.yaml`、`overlays/production/kustomization.yaml`
**状态:已修复。** HPA 加 `behavior`(scaleUp/Down 稳定窗口);生产 overlay 用 JSON6902
`remove /spec/replicas`(注:该 patch 必须放在同时包含 base Deployment 与 HPA 的 overlay,
放在 addon 内无目标可匹配、会被静默跳过)。

### [ ] 7. dataplane 资源配比
文件:`dataplane.yaml`、`overlays/production/patch-dataplane-deployment.yaml`
**状态:推迟。** 属环境相关容量决策(request/limit 比例、是否 Guaranteed、HPA 触发点),
建议按压测结果调整,而非拍脑袋改。

### [x] 8. 运行时镜像用 debian-slim
文件:`Dockerfile`
**状态:已修复。** 换 `gcr.io/distroless/static:nonroot`,删除 apt 层与 `WORKDIR /app`,
`USER 65532:65532`。已验证 `go build -mod=readonly ./cmd/manager` 通过。

### [x] 9. RBAC 中 cluster-wide secrets 写权限过宽
文件:`deploy/kubernetes/base/rbac.yaml`
**状态:已修复。** ClusterRole 的 secrets 收窄为 `get/list/watch`(跨命名空间读 TLS + informer watch);
新增命名空间级 Role + RoleBinding 提供 `create/get/patch/update`(chatbot-config、metrics-config
均写在 `nantian-gw`,已核实常量)。

---

## 低优先级(Low)

- [ ] 10. **NetworkPolicy 只有 Ingress**(推迟):补 egress 需明确 apiserver 端点/DNS,易误伤,建议单独处理。
- [x] 11. **controlplane `:18083` 暴露 `0.0.0.0/0`**:方案 B 下 :18083 现由 kubelet liveness/startup 探针使用,不再是"暴露但无人用";健康端点(/healthz、/readyz)敏感度低,保留可达以适配 kubelet 源 IP。如需进一步收敛可锁定到节点 CIDR(环境相关)。
- [x] 12. **dataplane `sysctls:` 为空键**:已删除。
- [ ] 13. **镜像 `:dev` + Always 且无 digest**(推迟):与仓库/发布流程相关。另:本仓库 Dockerfile 只构建 controlplane,`cmd/` 无 dataplane 入口,清单引用 `nantian-dataplane:dev`,来源在仓库外,需确认构建产物一致性。
- [x] 14. **构建可复现性**:已改 `-mod=readonly`,build 通过。
- [ ] 15. **dataplane metrics 复用 admin 端口 19080**(推迟):需改数据面代码(不在本仓库)。
- [x] 16. **PriorityClass 缺失**:已新增 `nantian-gw-critical`(value 1e9)并在两个 plane 引用。
- [ ] 17. **topologySpread 用 `ScheduleAnyway`**(可选):改 `DoNotSchedule` 在小集群可能排不下。
- [x] 18. **PDB `minAvailable: 1` 固定值**:dataplane 改 `maxUnavailable: 1`,随 HPA 伸缩保持可用性。

---

## 备注
- dashboard Deployment(`replicas: 1`)未加 PriorityClass;如需与两个 plane 一致可补,但它是 UI,重要性较低。
- 所有改动已通过 `kustomize build`(base + 6 overlay,`--load-restrictor LoadRestrictionsNone`)。
