# Wasm Plugin Developer Guide

This guide walks through building, testing, and deploying WebAssembly plugins for nantian-gw.

## Prerequisites

- Rust toolchain (stable, 1.70+)
- `wasm32-wasi` target: `rustup target add wasm32-wasi`
- `wasm32-unknown-unknown` target (optional, for bare-metal wasm)

## Quick Start: Hello Plugin

Create a new Rust project:

```bash
cargo new --lib hello-plugin
cd hello-plugin
```

Edit `Cargo.toml`:

```toml
[package]
name = "hello-plugin"
version = "0.1.0"
edition = "2021"

[lib]
crate-type = ["cdylib"]

[profile.release]
opt-level = "s"       # Optimize for size
lto = true            # Link-time optimization
strip = true          # Strip debug symbols
```

Edit `src/lib.rs`:

```rust
// Simple hello-world plugin that logs on every request.
// Must export `on_request`, `on_response`, or `on_stream_chunk`.

static mut BUFFER: [u8; 65536] = [0; 65536];
static mut OFFSET: usize = 0;

/// Allocate memory in the guest buffer. Returns a pointer (offset)
/// or -1 if out of memory.
#[no_mangle]
pub extern "C" fn alloc(len: i32) -> i32 {
    unsafe {
        if OFFSET + (len as usize) > BUFFER.len() {
            return -1;
        }
        let offset = OFFSET;
        OFFSET += len as usize;
        offset as i32
    }
}

#[no_mangle]
pub extern "C" fn dealloc(_ptr: i32, _len: i32) {}

/// Hook: called when a request arrives (before upstream proxy).
/// Return 0 to continue, non-zero to reject with that status code.
#[no_mangle]
pub extern "C" fn on_request() -> i32 {
    // Log a message via the host
    let msg = b"Hello from wasm plugin!\0";
    let ptr = alloc(msg.len() as i32);
    if ptr >= 0 {
        // Copy message to guest memory
        unsafe {
            std::ptr::copy_nonoverlapping(
                msg.as_ptr(),
                BUFFER.as_mut_ptr().offset(ptr as isize),
                msg.len(),
            );
        }
        // Call host::log (level=0 for debug)
        unsafe { nantian_log(0, ptr, msg.len() as i32 - 1) };
    }
    0 // Continue the request
}

// Host function declarations
extern "C" {
    fn nantian_log(level: i32, msg_ptr: i32, msg_len: i32);
}
```

Build:

```bash
cargo build --release --target wasm32-unknown-unknown
ls -la target/wasm32-unknown-unknown/release/hello_plugin.wasm
```

## Plugin Structure

### Required Exports

A plugin must export at least one hook function. Each returns `i32`:

| Function | Hook | Return |
|---|---|---|
| `on_request` | Before upstream proxy | 0 = continue, N = reject with HTTP status N |
| `on_response` | Before sending to client | 0 = continue, N = reject |
| `on_stream_chunk` | Per stream chunk | 0 = continue, N = abort stream |

### Required Guest Functions

The host calls these to manage guest memory:

```rust
#[no_mangle]
pub extern "C" fn alloc(len: i32) -> i32;
    // Allocate `len` bytes in guest linear memory
    // Returns: byte offset (>= 0) or -1 on failure

#[no_mangle]
pub extern "C" fn dealloc(ptr: i32, len: i32) {}
    // Free previously allocated memory (no-op is fine)
```

### Available Host Functions

Imported from the `nantian` module:

#### `nantian_log(level: i32, msg_ptr: i32, msg_len: i32)`

Log a message to the data plane log.
- `level`: 0 = debug, 1 = info, 2 = warn, 3+ = error
- `msg_ptr`, `msg_len`: Pointer and length in guest memory

```rust
extern "C" {
    fn nantian_log(level: i32, msg_ptr: i32, msg_len: i32);
}

fn log_warn(msg: &str) {
    let bytes = msg.as_bytes();
    let ptr = alloc(bytes.len() as i32);
    if ptr >= 0 {
        unsafe {
            std::ptr::copy_nonoverlapping(
                bytes.as_ptr(),
                BUFFER.as_mut_ptr().offset(ptr as isize),
                bytes.len(),
            );
            nantian_log(2, ptr, bytes.len() as i32);
        }
    }
}
```

#### `nantian_get_header(name_ptr: i32, name_len: i32) -> i64`

Read a request header. Returns `(ptr << 32) | len` packed into an i64, or 0 if not found.

```rust
extern "C" {
    fn nantian_get_header(name_ptr: i32, name_len: i32) -> i64;
}

fn get_header(name: &str) -> Option<String> {
    let name_bytes = name.as_bytes();
    let ptr = alloc(name_bytes.len() as i32);
    if ptr < 0 { return None; }
    unsafe {
        std::ptr::copy_nonoverlapping(
            name_bytes.as_ptr(),
            BUFFER.as_mut_ptr().offset(ptr as isize),
            name_bytes.len(),
        );
        let packed = nantian_get_header(ptr, name_bytes.len() as i32);
        if packed == 0 { return None; }
        let val_ptr = (packed >> 32) as i32;
        let val_len = (packed & 0xFFFFFFFF) as usize;
        let val_bytes = std::slice::from_raw_parts(
            BUFFER.as_ptr().offset(val_ptr as isize),
            val_len,
        );
        Some(String::from_utf8_lossy(val_bytes).to_string())
    }
}
```

#### `nantian_set_header(name_ptr: i32, name_len: i32, val_ptr: i32, val_len: i32)`

Set a response header. Applied after the plugin returns `Continue`.

```rust
extern "C" {
    fn nantian_set_header(
        name_ptr: i32, name_len: i32,
        val_ptr: i32, val_len: i32
    );
}
```

## Writing a Plugin: Step by Step

### 1. Authentication Plugin Example

```rust
// auth-plugin.rs — Reject requests without a valid API key

static mut BUFFER: [u8; 65536] = [0; 65536];
static mut OFFSET: usize = 0;

#[no_mangle]
pub extern "C" fn alloc(len: i32) -> i32 {
    unsafe {
        if OFFSET + (len as usize) > BUFFER.len() { return -1; }
        let offset = OFFSET;
        OFFSET += len as usize;
        offset as i32
    }
}

#[no_mangle]
pub extern "C" fn dealloc(_ptr: i32, _len: i32) {}

extern "C" {
    fn nantian_log(level: i32, msg_ptr: i32, msg_len: i32);
    fn nantian_get_header(name_ptr: i32, name_len: i32) -> i64;
}

#[no_mangle]
pub extern "C" fn on_request() -> i32 {
    let header_name = b"x-api-key\0";
    let ptr = alloc(header_name.len() as i32);
    if ptr < 0 { return 0; } // out of memory, allow through

    unsafe {
        std::ptr::copy_nonoverlapping(
            header_name.as_ptr(),
            BUFFER.as_mut_ptr().offset(ptr as isize),
            header_name.len(),
        );
    }

    let packed = unsafe {
        nantian_get_header(ptr, header_name.len() as i32 - 1)
    };

    if packed == 0 {
        // No API key header — reject with 401
        return 401;
    }

    0 // Allow the request
}
```

### 2. Response Header Injection

```rust
#[no_mangle]
pub extern "C" fn on_response() -> i32 {
    let name = b"x-powered-by\0";
    let value = b"nantian-gw\0";

    let n_ptr = alloc(name.len() as i32);
    let v_ptr = alloc(value.len() as i32);

    if n_ptr < 0 || v_ptr < 0 { return 0; }

    unsafe {
        std::ptr::copy_nonoverlapping(
            name.as_ptr(),
            BUFFER.as_mut_ptr().offset(n_ptr as isize),
            name.len(),
        );
        std::ptr::copy_nonoverlapping(
            value.as_ptr(),
            BUFFER.as_mut_ptr().offset(v_ptr as isize),
            value.len(),
        );
        nantian_set_header(
            n_ptr, name.len() as i32 - 1,
            v_ptr, value.len() as i32 - 1
        );
    }

    0
}
```

### 3. Rate Limiting with Config

Plugins receive their config as JSON. Access it through the `PluginContext` (future SDK will provide a direct host function).

```rust
// Conceptual: config-driven behavior
// In practice, config is passed via PluginContext.config (serde_json::Value)
// and will be accessible through a nantian::get_config host function.

#[no_mangle]
pub extern "C" fn on_request() -> i32 {
    // Future: let max_rps: i32 = nantian_get_config_int("max_rps");
    // Future: check rate counter, reject if exceeded
    0
}
```

## Building

### For release (optimized, small binary):

```bash
cargo build --release --target wasm32-unknown-unknown
```

Recommended profile settings in `Cargo.toml`:

```toml
[profile.release]
opt-level = "s"        # Optimize for size
lto = true             # Link-time optimization
strip = true           # Strip debug symbols
codegen-units = 1      # Better optimization
panic = "abort"        # Smaller binary
```

### Check binary size:

```bash
ls -lh target/wasm32-unknown-unknown/release/your_plugin.wasm
```

Target: under 100KB for simple plugins. Use `wasm-opt` from [binaryen](https://github.com/WebAssembly/binaryen) for further optimization:

```bash
wasm-opt -Os your_plugin.wasm -o your_plugin.opt.wasm
```

## Deploying

Create a `WasmPlugin` custom resource:

```yaml
apiVersion: gateway.nantian.dev/v1alpha1
kind: WasmPlugin
metadata:
  name: auth-plugin
  namespace: default
spec:
  wasm:
    url: "https://example.com/plugins/auth-plugin.wasm"
    # Or use ConfigMap:
    # configMap:
    #   namespace: default
    #   name: wasm-plugins
    #   key: auth-plugin.wasm
    # Or inline (base64):
    # inline: "AGFzbQEAAAAB..."
    sha256: "sha256:abc123def456..."
  hooks:
    - on_request
    - on_response
  config:
    apiKeys:
      - "key-001"
      - "key-002"
    rateLimit: 100
  sandbox:
    maxMemory: 16777216      # 16 MiB
    maxExecutionTime: 10     # 10 ms
```

Apply:

```bash
kubectl apply -f wasmplugin-auth.yaml
```

The control plane will:
1. Detect the new WasmPlugin
2. Translate it to IR
3. Send it via xDS to the data plane
4. The data plane's `PluginManager` will download, compile, and register it

## Testing

### Unit Tests (Rust)

Run aeg-wasm tests:

```bash
cargo test -p aeg-wasm --manifest-path dataplane/Cargo.toml
```

### Writing Plugin Unit Tests

Test your plugin logic in isolation:

```rust
#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_on_request_no_header() {
        let result = on_request();
        assert_eq!(result, 401); // Should reject without API key
    }
}
```

Run with: `cargo test --target wasm32-unknown-unknown`

### Integration Tests

See `dataplane/crates/aeg-wasm/tests/` for integration test patterns:

```bash
# Engine tests
cargo test -p aeg-wasm --manifest-path dataplane/Cargo.toml -- engine_tests

# Plugin manager tests
cargo test -p aeg-wasm --manifest-path dataplane/Cargo.toml -- plugin_tests
```

### Testing with Dashboard

After deploying a WasmPlugin, verify in the dashboard:

1. Navigate to **Wasm Plugins** in the sidebar
2. Confirm your plugin appears in the table
3. Click through to the detail page
4. Verify hooks, config, sandbox settings, and status conditions

## Memory Model

Plugins use a linear memory buffer with a simple bump allocator:

```rust
static mut BUFFER: [u8; 65536] = [0; 65536]; // 64KB buffer
static mut OFFSET: usize = 0;                 // Current allocation offset
```

Key rules:
- The host calls `alloc()` to reserve memory before passing pointers to host functions
- The buffer is fixed size (64KB default, adjustable)
- No garbage collection — memory is reclaimed when the store is dropped after each invocation
- Each hook invocation gets a fresh store, so memory is clean between requests

For larger memory needs, increase `BUFFER` size and set `max_memory_bytes` in `WasmSandboxConfig` accordingly.

## Troubleshooting

### Plugin fails to load

- Check `.wasm` file is valid: `wasm-validate your_plugin.wasm`
- Check SHA256 matches
- Verify the `wasm32-unknown-unknown` target was used (not `wasm32-wasi`)

### Plugin compiles but returns errors

- Check that all exported hook functions exist and match signatures
- Verify `alloc` and `dealloc` are exported
- Ensure no panics in guest code — use defensive programming

### Execution timeout

- Increase `maxExecutionTime` in sandbox config
- Profile plugin execution time with `console.time()` equivalent logging
- Consider splitting complex logic across multiple hook invocations

### Out of memory

- Increase `maxMemory` in sandbox config
- Increase `BUFFER` size in guest code
- Avoid large allocations in hot paths

### Debugging

Use `nantian_log` for printf-style debugging:

```rust
fn debug_state() {
    let msg = format!("Processing request, state: {:?}", state);
    // Write msg to BUFFER, then:
    unsafe { nantian_log(0, ptr, msg.len() as i32) };
}
```

Logs appear in the data plane output with the `aeg_wasm` target.
