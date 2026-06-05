#!/usr/bin/env python3

import argparse
import asyncio
import json
import math
import statistics
import time
from collections import Counter
from pathlib import Path
from typing import Any


DEFAULT_PAYLOAD = (
    "GET / HTTP/1.1\r\n"
    "Host: {host}\r\n"
    "Connection: close\r\n"
    "User-Agent: pgw-tcp-concurrency-client/1.0\r\n"
    "\r\n"
)


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run lightweight concurrent TCP request/response probes."
    )
    parser.add_argument("--addr", required=True, help="target host:port")
    parser.add_argument("--requests", type=int, default=500, help="total connections to open")
    parser.add_argument("--concurrency", type=int, default=64, help="maximum concurrent connections")
    parser.add_argument("--payload", help="literal payload to send")
    parser.add_argument("--payload-file", help="file containing payload bytes")
    parser.add_argument("--host-header", default="example.com", help="Host value for the default HTTP payload")
    parser.add_argument(
        "--expect-substring",
        default="aether-gateway-ok",
        help="response substring required for a successful request",
    )
    parser.add_argument("--connect-timeout", type=float, default=3.0, help="connect timeout seconds")
    parser.add_argument("--request-timeout", type=float, default=10.0, help="read timeout seconds")
    parser.add_argument("--output", help="optional JSON output path")
    parser.add_argument("--scenario", default="steady", help="profile scenario label")
    return parser.parse_args()


def split_addr(addr: str) -> tuple[str, int]:
    host, port_text = addr.rsplit(":", 1)
    return host, int(port_text)


def load_payload(args: argparse.Namespace) -> bytes:
    if args.payload_file:
        return Path(args.payload_file).read_bytes()
    if args.payload is not None:
        return args.payload.encode("utf-8")
    return DEFAULT_PAYLOAD.format(host=args.host_header).encode("ascii")


async def read_until_eof(reader: asyncio.StreamReader) -> bytes:
    chunks: list[bytes] = []
    while True:
        chunk = await reader.read(65536)
        if not chunk:
            break
        chunks.append(chunk)
    return b"".join(chunks)


def success_result(
    started: float,
    response: bytes,
    expected_substring: str,
) -> dict[str, Any]:
    text = response.decode("utf-8", errors="replace")
    return {
        "ok": expected_substring in text,
        "connected": True,
        "error": None,
        "latency_ms": (time.perf_counter() - started) * 1000.0,
        "bytes_received": len(response),
    }


def failure_result(started: float, error: str, connected: bool = False) -> dict[str, Any]:
    return {
        "ok": False,
        "connected": connected,
        "error": error,
        "latency_ms": (time.perf_counter() - started) * 1000.0,
        "bytes_received": 0,
    }


async def execute_request(
    host: str,
    port: int,
    payload: bytes,
    args: argparse.Namespace,
) -> dict[str, Any]:
    started = time.perf_counter()
    writer = None
    connected = False
    try:
        connect = asyncio.open_connection(host, port)
        reader, writer = await asyncio.wait_for(connect, timeout=args.connect_timeout)
        connected = True
        writer.write(payload)
        await writer.drain()
        try:
            writer.write_eof()
        except (OSError, RuntimeError):
            pass
        response = await asyncio.wait_for(read_until_eof(reader), timeout=args.request_timeout)
        return success_result(started, response, args.expect_substring)
    except asyncio.TimeoutError:
        return failure_result(started, "timeout", connected)
    except OSError as exc:
        return failure_result(started, str(exc), connected)
    finally:
        if writer is not None:
            writer.close()
            try:
                await writer.wait_closed()
            except Exception:
                pass


def percentile(values: list[float], ratio: float) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    index = min(max(math.ceil(len(values) * ratio) - 1, 0), len(values) - 1)
    return values[index]


def summarize(results: list[dict[str, Any]], args: argparse.Namespace) -> dict[str, Any]:
    latencies = sorted(float(item["latency_ms"]) for item in results)
    errors = Counter(str(item["error"]) for item in results if item["error"] is not None)
    successes = sum(1 for item in results if item["ok"])
    bytes_received = sum(int(item["bytes_received"]) for item in results)
    connections_opened = sum(1 for item in results if item["connected"])
    active_connection_count = min(args.concurrency, args.requests)
    return {
        "addr": args.addr,
        "scenario": args.scenario,
        "requests": args.requests,
        "completed": len(results),
        "successes": successes,
        "success_rate": successes / len(results) if results else 0.0,
        "concurrency": args.concurrency,
        "connection_count": active_connection_count,
        "connections_opened": connections_opened,
        "bytes_received": bytes_received,
        "error_counts": dict(sorted(errors.items())),
        "latency_ms": {
            "min": latencies[0] if latencies else 0.0,
            "mean": statistics.fmean(latencies) if latencies else 0.0,
            "p50": percentile(latencies, 0.50),
            "p90": percentile(latencies, 0.90),
            "p95": percentile(latencies, 0.95),
            "p99": percentile(latencies, 0.99),
            "p999": percentile(latencies, 0.999),
            "max": latencies[-1] if latencies else 0.0,
        },
    }


async def run(args: argparse.Namespace) -> dict[str, Any]:
    host, port = split_addr(args.addr)
    payload = load_payload(args)
    results: list[dict[str, Any]] = []
    semaphore = asyncio.Semaphore(args.concurrency)

    async def run_one() -> None:
        async with semaphore:
            results.append(await execute_request(host, port, payload, args))

    await asyncio.gather(*(run_one() for _ in range(args.requests)))
    return summarize(results, args)


def main() -> int:
    args = parse_args()
    summary = asyncio.run(run(args))
    payload = json.dumps(summary, sort_keys=True)
    print(payload)
    if args.output:
        Path(args.output).write_text(payload + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
