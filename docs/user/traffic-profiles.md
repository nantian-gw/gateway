# Traffic Profile Examples

This document supplements the [Install Profile Matrix](install-profiles.md) with traffic access examples that were not expanded there, helping deployers distinguish between north-south and east-west usage patterns.

Install profiles answer "how to install the control plane, data plane, and observability components"; traffic profiles answer "where business traffic enters, and which Gateway API object receives it." Do not conflate the two: a single `single-cluster-prod` installation can simultaneously carry both north-south and east-west traffic.

## Profile Overview

| Profile | Entry Object | Typical Resources | Default Recommended Exposure | Current Support Boundary |
| --- | --- | --- | --- | --- |
| North-south HTTP / gRPC | `Gateway` | `HTTPRoute`, `GRPCRoute` | per-Gateway Service, adjustable to `LoadBalancer` / `NodePort` via `Gateway.spec.infrastructure.parametersRef` | Recommended production entry |
| North-south TCP / UDP | `Gateway` | `TCPRoute`, `UDPRoute` | per-Gateway Service, exposed per listener port | Runtime and smoke / conformance subset exists, still lacks production-grade throughput and long-stability evidence |
| East-west service parent | `Service` parentRef | `HTTPRoute`, `GRPCRoute`, `TCPRoute`, `UDPRoute`, `TLSRoute` | Control plane maintains mesh frontend / shadow Service | Repo extension capability, not equivalent to a complete Gateway API standard profile declaration |

## North-South HTTP Example

This example routes external HTTP traffic through `Gateway/edge/public`, then forwards it via `HTTPRoute/app/orders` to `Service/app/orders:8080`.

For production environments, it is recommended to control per-Gateway Service exposure via `Gateway.spec.infrastructure.parametersRef` rather than directly modifying the control-plane-derived Service.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: edge
---
apiVersion: v1
kind: Namespace
metadata:
  name: app
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: public-gateway-infra
  namespace: edge
data:
  service.yaml: |
    type: LoadBalancer
    externalTrafficPolicy: Local
    loadBalancerSourceRanges:
      - 198.51.100.0/24
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public
  namespace: edge
spec:
  gatewayClassName: nantian
  infrastructure:
    labels:
      example.com/traffic-profile: north-south
    annotations:
      example.com/exposure: internet
    parametersRef:
      group: ""
      kind: ConfigMap
      name: public-gateway-infra
  listeners:
    - name: http
      protocol: HTTP
      port: 80
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orders
  namespace: app
spec:
  parentRefs:
    - name: public
      namespace: edge
      sectionName: http
  hostnames:
    - orders.example.com
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: orders
          port: 8080
```

Minimum checks:

```bash
kubectl -n edge describe gateway public
kubectl -n edge get svc nantian-gw-public -o wide
kubectl -n app describe httproute orders
```

If `Gateway.status.addresses` does not have an external address, first check the `type`, `externalIPs`, and `LoadBalancer ingress` of the `nantian-gw-public` Service, and whether `/v1/infrastructure` shows that Service as still in `missing` or `drifted` state.

gRPC can reuse the same HTTP / HTTPS listener — just replace `HTTPRoute` with `GRPCRoute` and point the backend to the gRPC Service port:

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GRPCRoute
metadata:
  name: inventory
  namespace: app
spec:
  parentRefs:
    - name: public
      namespace: edge
      sectionName: http
  hostnames:
    - inventory.example.com
  rules:
    - matches:
        - method:
            service: inventory.v1.InventoryService
      backendRefs:
        - name: inventory
          port: 50051
```

## North-South TCP / UDP Example

This example demonstrates a single north-south Gateway exposing both TCP and UDP listeners. In production, whether a single `LoadBalancer` Service can mix protocols depends on the cluster's LB controller; if the controller does not support it, consider splitting into different Gateways or different exposure strategies.

```yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: public-l4
  namespace: edge
spec:
  gatewayClassName: nantian
  infrastructure:
    parametersRef:
      group: ""
      kind: ConfigMap
      name: public-gateway-infra
  listeners:
    - name: tcp-postgres
      protocol: TCP
      port: 5432
      allowedRoutes:
        namespaces:
          from: All
    - name: udp-dns
      protocol: UDP
      port: 53
      allowedRoutes:
        namespaces:
          from: All
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: TCPRoute
metadata:
  name: postgres
  namespace: db
spec:
  parentRefs:
    - name: public-l4
      namespace: edge
      sectionName: tcp-postgres
  rules:
    - backendRefs:
        - name: postgres
          port: 5432
---
apiVersion: gateway.networking.k8s.io/v1alpha2
kind: UDPRoute
metadata:
  name: dns
  namespace: dns
spec:
  parentRefs:
    - name: public-l4
      namespace: edge
      sectionName: udp-dns
  rules:
    - backendRefs:
        - name: dns
          port: 53
```

Minimum checks:

```bash
kubectl -n edge describe gateway public-l4
kubectl -n db describe tcproute postgres
kubectl -n dns describe udproute dns
kubectl -n edge get svc nantian-gw-public-l4 -o yaml
```

If the Route and Gateway both show accepted but are unreachable externally, first confirm that the external LB actually has the corresponding protocol and port enabled, then check dataplane logs, `/v1/listeners`, `/v1/routes`, and per-pod scrape results for `nantian_gateway_dataplane_ready`.

## East-West Service Parent Example

In east-west scenarios, Routes can point `parentRefs` to a `Service`. The control plane maintains a mesh frontend / shadow Service based on this Service, handing traffic entering that Service to the dataplane, which then selects backends according to Route rules.

This is an extension capability of the current repo's service parent / mesh frontend implementation and should not be described as a complete Gateway API official profile declaration.

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: app
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: orders-mesh
  namespace: app
spec:
  parentRefs:
    - group: ""
      kind: Service
      name: orders
      port: 80
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /
      backendRefs:
        - name: orders-v1
          port: 8080
          weight: 90
        - name: orders-v2
          port: 8080
          weight: 10
```

Minimum checks:

```bash
kubectl -n app describe httproute orders-mesh
kubectl -n app get svc -l nantian.dev/service-role=mesh-frontend
kubectl -n app get svc -l nantian.dev/service-role=mesh-backend-shadow
```

Troubleshooting order:

1. First check whether `HTTPRoute` shows `Accepted=True` and the parent reference is resolved.
2. Then check whether the mesh frontend Service exists, and whether the original Service is marked as mesh frontend by the control plane.
3. Finally check controlplane `/v1/infrastructure` to confirm whether the mesh frontend / shadow Service / frontend EndpointSlice are `ready`.

## Selection Recommendations

- For public internet or cross-cluster entry, prefer north-south `Gateway` and control per-Gateway Service type and address via `parametersRef`.
- For intra-cluster service governance or progressive migration, use east-west service parent, but treat it as a repo extension capability and retain dedicated e2e or business validation.
- TCP / UDP entries must have their LB controller, NetworkPolicy, node ports, security groups, and actual dataplane forwarding success rate independently verified before going live.
- controlplane/dataplane admin API should only be accessed via controlled entry points, port-forward, or internal network operations channels, and should be troubleshot separately from business traffic Gateways.