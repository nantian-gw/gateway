#!/usr/bin/env python3

import argparse
import asyncio
import base64
import hashlib
import json
import math
import os
import statistics
import time
from collections import Counter
from pathlib import Path
from typing import Any
from urllib.parse import urlparse


GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run concurrent WebSocket echo probes.")
    parser.add_argument("--url", required=True, help="target ws:// URL")
    parser.add_argument("--requests", type=int, default=20, help="total WebSocket probes")
    parser.add_argument("--concurrency", type=int, default=20, help="maximum concurrent probes")
    parser.add_argument("--host-header", help="override Host header; defaults to URL host")
    parser.add_argument("--payload", default="nantian-websocket", help="text payload to echo")
    parser.add_argument("--connect-timeout", type=float, default=3.0, help="connect timeout seconds")
    parser.add_argument("--request-timeout", type=float, default=5.0, help="per-probe timeout seconds")
    parser.add_argument("--hold-ms", type=int, default=0, help="hold upgraded connection open after echo")
    parser.add_argument("--scenario", default="long-lived-streaming", help="profile scenario label")
    parser.add_argument("--output", help="optional JSON output path")
    return parser.parse_args()


def websocket_accept(key: str) -> str:
    digest = hashlib.sha1((key + GUID).encode("ascii")).digest()
    return base64.b64encode(digest).decode("ascii")


def build_client_frame(payload: str) -> bytes:
    body = payload.encode("utf-8")
    mask = os.urandom(4)
    header = bytearray([0x81])
    if len(body) < 126:
        header.append(0x80 | len(body))
    elif len(body) < 65536:
        header.append(0x80 | 126)
        header.extend(len(body).to_bytes(2, "big"))
    else:
        header.append(0x80 | 127)
        header.extend(len(body).to_bytes(8, "big"))
    masked = bytes(body[index] ^ mask[index % 4] for index in range(len(body)))
    return bytes(header) + mask + masked


async def read_frame(reader: asyncio.StreamReader) -> tuple[int, bytes]:
    first = await reader.readexactly(1)
    second = await reader.readexactly(1)
    opcode = first[0] & 0x0F
    masked = (second[0] & 0x80) != 0
    length = second[0] & 0x7F
    if length == 126:
        length = int.from_bytes(await reader.readexactly(2), "big")
    elif length == 127:
        length = int.from_bytes(await reader.readexactly(8), "big")
    mask = await reader.readexactly(4) if masked else b""
    payload = await reader.readexactly(length)
    if masked:
        payload = bytes(payload[index] ^ mask[index % 4] for index in range(length))
    return opcode, payload


def split_header_line(line: str) -> tuple[str, str] | None:
    if ":" not in line:
        return None
    name, value = line.split(":", 1)
    return name.strip().lower(), value.strip()


async def websocket_probe(args: argparse.Namespace) -> dict[str, Any]:
    parsed = urlparse(args.url)
    if parsed.scheme != "ws":
        raise SystemExit("only ws:// URLs are supported")
    if not parsed.hostname:
        raise SystemExit("URL must include a hostname")

    host = parsed.hostname
    port = parsed.port or 80
    path = parsed.path or "/"
    if parsed.query:
        path = f"{path}?{parsed.query}"
    authority = args.host_header or parsed.netloc
    key = base64.b64encode(os.urandom(16)).decode("ascii")
    request = (
        f"GET {path} HTTP/1.1\r\n"
        f"Host: {authority}\r\n"
        "Upgrade: websocket\r\n"
        "Connection: Upgrade\r\n"
        f"Sec-WebSocket-Key: {key}\r\n"
        "Sec-WebSocket-Version: 13\r\n"
        "\r\n"
    ).encode("ascii")

    started = time.perf_counter()
    writer = None
    try:
        connect = asyncio.open_connection(host, port)
        reader, writer = await asyncio.wait_for(connect, timeout=args.connect_timeout)
        writer.write(request)
        await writer.drain()

        header_bytes = await asyncio.wait_for(
            reader.readuntil(b"\r\n\r\n"), timeout=args.request_timeout
        )
        header_text = header_bytes.decode("latin1", errors="replace")
        lines = header_text.split("\r\n")
        status_parts = lines[0].split(" ", 2) if lines else []
        status = int(status_parts[1]) if len(status_parts) >= 2 and status_parts[1].isdigit() else 0
        if status != 101:
            return failure_result(started, f"upgrade status {status}", status)

        headers = {}
        for line in lines[1:]:
            item = split_header_line(line)
            if item is not None:
                headers[item[0]] = item[1]
        if headers.get("sec-websocket-accept") != websocket_accept(key):
            return failure_result(started, "sec-websocket-accept mismatch", status)

        writer.write(build_client_frame(args.payload))
        await writer.drain()
        opcode, payload = await asyncio.wait_for(read_frame(reader), timeout=args.request_timeout)
        if opcode != 0x1:
            return failure_result(started, f"unexpected opcode {opcode}", status)
        response_text = payload.decode("utf-8", errors="replace")
        if response_text != args.payload:
            return failure_result(started, "payload mismatch", status)
        if args.hold_ms > 0:
            await asyncio.sleep(args.hold_ms / 1000.0)
        return {
            "ok": True,
            "status": status,
            "error": None,
            "latency_ms": (time.perf_counter() - started) * 1000.0,
            "bytes_received": len(header_bytes) + len(payload),
            "upgraded": True,
            "message_received": True,
        }
    except asyncio.TimeoutError:
        return failure_result(started, "timeout", None)
    except (OSError, asyncio.IncompleteReadError, ValueError) as exc:
        return failure_result(started, str(exc), None)
    finally:
        if writer is not None:
            writer.close()
            try:
                await writer.wait_closed()
            except Exception:
                pass


def failure_result(started: float, error: str, status: int | None) -> dict[str, Any]:
    return {
        "ok": False,
        "status": status,
        "error": error,
        "latency_ms": (time.perf_counter() - started) * 1000.0,
        "bytes_received": 0,
        "upgraded": False,
        "message_received": False,
    }


def percentile(values: list[float], ratio: float) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    index = min(max(math.ceil(len(values) * ratio) - 1, 0), len(values) - 1)
    return values[index]


def summarize(results: list[dict[str, Any]], args: argparse.Namespace) -> dict[str, Any]:
    latencies = sorted(float(item["latency_ms"]) for item in results)
    statuses = Counter(str(item["status"]) for item in results if item["status"] is not None)
    errors = Counter(str(item["error"]) for item in results if item["error"] is not None)
    successes = sum(1 for item in results if item["ok"])
    upgrade_successes = sum(1 for item in results if item["upgraded"])
    messages_received = sum(1 for item in results if item["message_received"])
    bytes_received = sum(int(item["bytes_received"]) for item in results)
    return {
        "url": args.url,
        "protocol": "websocket",
        "scenario": args.scenario,
        "requests": args.requests,
        "completed": len(results),
        "successes": successes,
        "success_rate": successes / len(results) if results else 0.0,
        "concurrency": args.concurrency,
        "connection_count": len(results),
        "upgrade_successes": upgrade_successes,
        "messages_sent": len(results),
        "messages_received": messages_received,
        "bytes_received": bytes_received,
        "hold_ms": args.hold_ms,
        "status_counts": dict(sorted(statuses.items())),
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
    semaphore = asyncio.Semaphore(max(args.concurrency, 1))

    async def guarded_probe() -> dict[str, Any]:
        async with semaphore:
            return await websocket_probe(args)

    return summarize(
        await asyncio.gather(*(guarded_probe() for _ in range(args.requests))),
        args,
    )


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
