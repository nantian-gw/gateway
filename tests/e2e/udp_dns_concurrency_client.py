#!/usr/bin/env python3

import argparse
import concurrent.futures
import json
import math
import random
import socket
import statistics
import struct
import time
from collections import Counter
from pathlib import Path
from typing import Any


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Run concurrent UDP DNS probes.")
    parser.add_argument("--addr", required=True, help="target host:port")
    parser.add_argument("--requests", type=int, default=300, help="total DNS queries")
    parser.add_argument("--concurrency", type=int, default=64, help="maximum concurrent workers")
    parser.add_argument(
        "--clients",
        type=int,
        default=0,
        help="client socket count; defaults to min(concurrency, requests)",
    )
    parser.add_argument("--name", default="foo.bar.com", help="DNS name to query")
    parser.add_argument("--timeout", type=float, default=3.0, help="per-query timeout seconds")
    parser.add_argument(
        "--socket-mode",
        choices=("per-worker", "per-request"),
        default="per-worker",
        help="reuse one UDP socket per worker or create a socket for every query",
    )
    parser.add_argument("--scenario", default="multi-client", help="profile scenario label")
    parser.add_argument(
        "--expect-timeout",
        action="store_true",
        help="treat UDP query timeouts as the expected successful outcome",
    )
    parser.add_argument(
        "--upstream-count",
        type=int,
        default=1,
        help="expected upstream endpoint count to report with the profile",
    )
    parser.add_argument("--output", help="optional JSON output path")
    return parser.parse_args()


def split_addr(addr: str) -> tuple[str, int]:
    host, port_text = addr.rsplit(":", 1)
    return host, int(port_text)


def encode_name(name: str) -> bytes:
    labels = name.rstrip(".").split(".")
    return b"".join(bytes((len(label),)) + label.encode("ascii") for label in labels) + b"\x00"


def build_query(name: str, query_id: int) -> bytes:
    header = struct.pack("!HHHHHH", query_id, 0x0100, 1, 0, 0, 0)
    question = encode_name(name) + struct.pack("!HH", 1, 1)
    return header + question


def validate_response(query_id: int, response: bytes) -> tuple[bool, str | None]:
    if len(response) < 12:
        return False, "short dns response"
    resp_id, flags, qdcount, ancount, nscount, arcount = struct.unpack(
        "!HHHHHH", response[:12]
    )
    if resp_id != query_id:
        return False, "dns id mismatch"
    if (flags & 0x000F) != 0:
        return False, f"dns rcode {flags & 0x000F}"
    if qdcount != 1:
        return False, f"unexpected question count {qdcount}"
    if (ancount + nscount + arcount) < 1:
        return False, "dns response contained no resource records"
    return True, None


def query_once(sock: socket.socket, host: str, port: int, name: str, timeout: float) -> dict[str, Any]:
    query_id = random.randint(0, 0xFFFF)
    query = build_query(name, query_id)
    started = time.perf_counter()
    try:
        sock.settimeout(timeout)
        sock.sendto(query, (host, port))
        response, _ = sock.recvfrom(4096)
        ok, error = validate_response(query_id, response)
        return {
            "ok": ok,
            "received": True,
            "error": error,
            "latency_ms": (time.perf_counter() - started) * 1000.0,
            "bytes_received": len(response),
        }
    except TimeoutError:
        return {
            "ok": False,
            "received": False,
            "error": "timeout",
            "latency_ms": (time.perf_counter() - started) * 1000.0,
            "bytes_received": 0,
        }
    except OSError as exc:
        return {
            "ok": False,
            "received": False,
            "error": str(exc),
            "latency_ms": (time.perf_counter() - started) * 1000.0,
            "bytes_received": 0,
        }


def run_worker_socket(
    host: str,
    port: int,
    name: str,
    timeout: float,
    count: int,
) -> list[dict[str, Any]]:
    if count <= 0:
        return []
    results = []
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        for _ in range(count):
            results.append(query_once(sock, host, port, name, timeout))
    return results


def run_request_socket(host: str, port: int, name: str, timeout: float) -> dict[str, Any]:
    with socket.socket(socket.AF_INET, socket.SOCK_DGRAM) as sock:
        return query_once(sock, host, port, name, timeout)


def percentile(values: list[float], ratio: float) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    index = min(max(math.ceil(len(values) * ratio) - 1, 0), len(values) - 1)
    return values[index]


def summarize(
    results: list[dict[str, Any]],
    args: argparse.Namespace,
    client_count: int,
    socket_opens: int,
) -> dict[str, Any]:
    latencies = sorted(float(item["latency_ms"]) for item in results)
    errors = Counter(str(item["error"]) for item in results if item["error"] is not None)
    expected_timeout = bool(getattr(args, "expect_timeout", False))
    if expected_timeout:
        successes = sum(
            1
            for item in results
            if item["error"] == "timeout" and not item["received"]
        )
    else:
        successes = sum(1 for item in results if item["ok"])
    packets_received = sum(1 for item in results if item["received"])
    bytes_received = sum(int(item["bytes_received"]) for item in results)
    packets_sent = args.requests
    packets_lost = max(packets_sent - packets_received, 0)
    upstream_count = max(int(getattr(args, "upstream_count", 1)), 0)
    return {
        "addr": args.addr,
        "scenario": args.scenario,
        "requests": args.requests,
        "completed": len(results),
        "successes": successes,
        "success_rate": successes / len(results) if results else 0.0,
        "concurrency": args.concurrency,
        "client_count": client_count,
        "upstream_count": upstream_count,
        "expected_timeout": expected_timeout,
        "packets_sent": packets_sent,
        "packets_received": packets_received,
        "packets_lost": packets_lost,
        "bytes_received": bytes_received,
        "session_opens": socket_opens,
        "session_evictions": 0,
        "socket_mode": args.socket_mode,
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


def worker_counts(total: int, workers: int) -> list[int]:
    if total <= 0 or workers <= 0:
        return []
    active_workers = min(total, workers)
    base = total // active_workers
    extra = total % active_workers
    return [base + (1 if index < extra else 0) for index in range(active_workers)]


def run_sync(args: argparse.Namespace) -> dict[str, Any]:
    host, port = split_addr(args.addr)
    default_clients = min(args.concurrency, args.requests) if args.requests > 0 else 0
    client_count = args.clients if args.clients > 0 else default_clients
    client_count = min(client_count, args.requests) if args.requests > 0 else 0

    results: list[dict[str, Any]] = []
    if args.socket_mode == "per-worker":
        counts = worker_counts(args.requests, client_count)
        with concurrent.futures.ThreadPoolExecutor(max_workers=max(len(counts), 1)) as pool:
            futures = [
                pool.submit(run_worker_socket, host, port, args.name, args.timeout, count)
                for count in counts
            ]
            for future in concurrent.futures.as_completed(futures):
                results.extend(future.result())
        socket_opens = len(counts)
    else:
        with concurrent.futures.ThreadPoolExecutor(max_workers=max(args.concurrency, 1)) as pool:
            futures = [
                pool.submit(run_request_socket, host, port, args.name, args.timeout)
                for _ in range(args.requests)
            ]
            for future in concurrent.futures.as_completed(futures):
                results.append(future.result())
        client_count = args.requests
        socket_opens = args.requests

    return summarize(results, args, client_count, socket_opens)


def main() -> int:
    args = parse_args()
    summary = run_sync(args)
    payload = json.dumps(summary, sort_keys=True)
    print(payload)
    if args.output:
        Path(args.output).write_text(payload + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
