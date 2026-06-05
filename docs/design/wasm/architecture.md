# Wasm Plugin Architecture

The Wasm plugin system extends aether-gateway's data plane with user-defined logic compiled to WebAssembly (WASM). Plugins run in a sandboxed WebAssembly runtime using [wasmtime](https://wasmtime.dev/), providing safe, portable, and deterministic extensions to the proxy pipeline.

## Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        Aether Gateway                         │
│                                                                 │
│  ┌──────────┐    ┌──────────┐    ┌──────────┐                 │
│  │  HTTP    │    │  Stream  │    │  gRPC    │                 │
│  │  Runtime │    │  Runtime │    │  Runtime │                 │
│  └────┬─────┘    └────┬─────┘    └────┬─────┘                 │
│       │               │               │                        │
│       ▼               ▼               ▼                        │
│  ┌─────────────────────────────────────────┐                   │
│  │            Hook Points                   │                   │
│  │  ┌──────────┐ ┌──────────┐ ┌─────────┐ │                   │
│  │  │on_request│ │on_response│ │on_stream│ │                   │
│  │  │          │ │          │ │_chunk   │ │                   │
│  │  └────┬─────┘ └────┬─────┘ └────┬────┘ │                   │
│  └───────┼─────────────┼────────────┼──────┘                   │
│          │             │            │                           │
│          ▼             ▼            ▼                           │
│  ┌─────────────────────────────────────────┐                   │
│  │          PluginManager (aeg-wasm)        │                   │
│  │  ┌──────────────────────────────────┐    │                   │
│  │  │  wasmtime Engine + Linker         │    │                   │
│  │  │  ┌────────┐  ┌────────┐          │    │                   │
│  │  │  │Plugin A│  │Plugin B│  ...     │    │                   │
│  │  │  │.wasm   │  │.wasm   │          │    │                   │
│  │  │  └────────┘  └────────┘          │    │                   │
│  │  └──────────────────────────────────┘    │                   │
│  │  Sandbox: memory limits, execution        │                   │
│  │  timeout, resource isolation              │                   │
│  └─────────────────────────────────────────┘                   │
│                                                                 │
│  ┌─────────────────────────────────────────┐                   │
│  │         Control Plane                    │                   │
│  │  ┌──────────────┐    ┌───────────────┐   │                   │
│  │  │ WasmPlugin   │───▶│ Translator    │   │                   │
│  │  │ CRD (k8s)    │    │ → IR → Proto  │   │                   │
│  │  └──────────────┘    └───────┬───────┘   │                   │
│  │                              │            │                   │
│  │                      ┌───────▼───────┐   │                   │
│  │                      │ gRPC xDS      │   │                   │
│  │                      │ Server        │───▶│── Data Plane     │
│  │                      └───────────────┘   │                   │
│  └─────────────────────────────────────────┘                   │
└─────────────────────────────────────────────────────────────────┘
```

## Crate Structure

The Wasm plugin system lives in `dataplane/crates/aeg-wasm/`:

```
aeg-wasm/
├── Cargo.toml
└── src/
    ├── lib.rs          # Crate root (#![forbid(unsafe_code)])
    ├── engine.rs       # wasmtime Engine, Linker, PluginContext, WasmEngine
    ├── plugin.rs       # PluginManager, LoadedPlugin, WasmHook, HookResult
    ├── host.rs         # Host functions (log, get_header, set_header)
    ├── sandbox.rs      # AISandbox for LLM operations
    ├── mem.rs          # Guest memory management
    └── error.rs        # WasmError type
```

## Core Components

### PluginManager (`plugin.rs`)

The central orchestrator that manages the full lifecycle of Wasm plugins:

- **Load**: Compiles `.wasm` bytes into a `wasmtime::Module`, stores it keyed by name with its config, hooks, and sandbox settings.
- **Invoke**: On a hook trigger, creates a fresh `wasmtime::Store` with request context (headers, body, config), applies resource limits, sets epoch deadline for timeout, instantiates the module, and calls the hook export.
- **Unload**: Removes the compiled module from the registry.
- **Epoch Timer**: A background thread increments the engine epoch every 1ms for deterministic timeout enforcement.

### WasmEngine (`engine.rs`)

Configures and creates the shared `wasmtime::Engine` with:

- `epoch_interruption(true)` — enables timeout enforcement via epochs
- `wasm_multi_memory(true)` — supports multi-memory WASM proposals
- `wasm_component_model(true)` — future support for component model
- `cranelift_opt_level(OptLevel::Speed)` — optimized native code generation

### PluginContext (`engine.rs`)

The per-invocation state passed into each WASM instance:

| Field | Purpose |
|---|---|
| `config` | Plugin-specific JSON configuration |
| `request_headers` | HTTP request headers for the current call |
| `response_headers` | Mutable response headers (set by plugin) |
| `body` | Request/response body bytes |
| `memory_limit` | Maximum heap memory in bytes |

Implements `wasmtime::ResourceLimiter` to enforce `memory_limit`.

### WasmHook (`plugin.rs`)

Three hook points where plugins can execute:

| Hook | Export Name | Trigger |
|---|---|---|
| `OnRequest` | `on_request` | When an HTTP request is received, before upstream proxy |
| `OnResponse` | `on_response` | When a response is received from upstream, before client |
| `OnStreamChunk` | `on_stream_chunk` | For each chunk in a streaming response |

### HookResult (`plugin.rs`)

Return value from hook invocation:

- `Continue` — allow the request/response to proceed normally
- `Reject(i32)` — reject with the given HTTP status code

### WasmSandboxConfig (`plugin.rs`)

Sandbox limits applied per plugin:

| Field | Default | Description |
|---|---|---|
| `max_memory_bytes` | 16 MiB | Maximum heap memory |
| `max_execution_ms` | 10 ms | Maximum execution time |

## Security Model

The WASM runtime provides defense-in-depth sandboxing:

1. **Memory isolation**: Each plugin instance gets its own linear memory with a hard limit enforced by `PluginContext::memory_growing()`. Plugins cannot access host memory directly.

2. **Execution timeout**: The epoch interruption mechanism kills plugins that exceed `max_execution_ms`. A background thread increments the engine epoch every 1ms; each store is created with a deadline of `current_epoch + max_execution_ms`.

3. **No host access**: Plugins can only interact with the host through explicitly registered host functions. There is no filesystem, network, or system call access unless explicitly granted.

4. **Fresh store per invocation**: Each hook call creates a new `wasmtime::Store`, preventing state leakage between requests.

5. **Compilation safety**: wasmtime validates all WASM modules at load time. Malformed or malicious bytecode is rejected before any guest code executes.

## Host Functions API

Host functions are registered under the `aether` module and provide controlled interaction with the proxy environment.

### `aether::log(level: i32, msg_ptr: i32, msg_len: i32)`

Log a message from the guest plugin. Level 0 = debug, 1-2 = warn, 3+ = debug.

### `aether::get_header(name_ptr: i32, name_len: i32) -> i64`

Read a request header by name. Returns a packed `(ptr << 32) | len` value in guest memory, or 0 if not found.

### `aether::set_header(name_ptr: i32, name_len: i32, val_ptr: i32, val_len: i32)`

Set a response header. The modified headers are applied after the plugin returns.

## Plugin Lifecycle

```
┌──────────┐     ┌──────────┐     ┌──────────┐     ┌──────────┐
│  LOAD    │────▶│  COMPILE │────▶│  REGISTER│────▶│  ACTIVE  │
│ .wasm    │     │ wasmtime │     │ in hash  │     │          │
│ bytes    │     │ Module   │     │ map      │     │          │
└──────────┘     └──────────┘     └──────────┘     └─────┬────┘
                                                          │
                                              ┌───────────▼──────────┐
                                              │    INVOKE on hook     │
                                              │  ┌─────────────────┐  │
                                              │  │ Create Store     │  │
                                              │  │ Set resource lim │  │
                                              │  │ Set epoch deadln │  │
                                              │  │ Instantiate mod  │  │
                                              │  │ Call export func │  │
                                              │  │ Return result    │  │
                                              │  └─────────────────┘  │
                                              └───────────┬──────────┘
                                                          │
                                              ┌───────────▼──────────┐
                                              │      UNLOAD          │
                                              │  Remove from map     │
                                              │  Drop Module         │
                                              └──────────────────────┘
```

## Deployment Flow

```
Kubernetes Cluster
    │
    ▼
WasmPlugin CRD
  ├── metadata (name, namespace)
  ├── spec.wasm (url | configMap | inline base64)
  ├── spec.hooks ([on_request, on_response, ...])
  ├── spec.config (JSON)
  └── spec.sandbox (max_memory, max_execution)
    │
    ▼
Go Translator (controlplane)
  ├── Reads WasmPlugin from informer cache
  ├── Validates hook list
  ├── Builds IR representation
  └── Encodes into proto snapshot
    │
    ▼
gRPC xDS Stream
    │
    ▼
Rust xDS Client (aeg-xds)
  ├── Deserializes proto snapshot
  └── Passes to PluginManager
    │
    ▼
PluginManager::load_plugin()
  ├── Compiles WASM bytes
  ├── Registers hooks
  ├── Sets sandbox config
  └── Ready for invocation
```

## AISandbox (`sandbox.rs`)

A separate sandbox environment for AI/LLM operations. Currently a placeholder with module loading support. Future functionality includes tokenization and embedding via guest SDKs. Separated from the main PluginManager to allow different security policies for AI workloads.
