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
from urllib.parse import urlparse


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Run lightweight concurrent HTTP/1.1 requests against a target endpoint."
    )
    parser.add_argument("--url", required=True, help="target URL, for example http://127.0.0.1:18080/")
    parser.add_argument("--requests", type=int, default=1000, help="total number of requests")
    parser.add_argument("--concurrency", type=int, default=32, help="maximum concurrent requests")
    parser.add_argument("--method", default="GET", help="HTTP method")
    parser.add_argument(
        "--header",
        action="append",
        default=[],
        help="additional request header in 'Name: Value' form",
    )
    parser.add_argument(
        "--host-header",
        help="override Host header; defaults to the URL host",
    )
    parser.add_argument(
        "--body-file",
        help="optional request body file",
    )
    parser.add_argument(
        "--body-bytes",
        type=int,
        default=0,
        help="send a synthetic request body with this many bytes",
    )
    parser.add_argument(
        "--body-chunk-size",
        type=int,
        default=0,
        help="stream the body in chunks of this size; 0 sends it in one write",
    )
    parser.add_argument(
        "--body-chunk-interval-ms",
        type=int,
        default=0,
        help="sleep between body chunks",
    )
    parser.add_argument(
        "--connect-timeout",
        type=float,
        default=3.0,
        help="TCP connect timeout in seconds",
    )
    parser.add_argument(
        "--request-timeout",
        type=float,
        default=10.0,
        help="total per-request timeout in seconds",
    )
    parser.add_argument(
        "--expect-status",
        type=int,
        action="append",
        default=[],
        help="expected HTTP status code; may be specified multiple times",
    )
    parser.add_argument(
        "--expect-body-substring",
        help="require the response body to contain this substring",
    )
    parser.add_argument(
        "--output",
        help="optional JSON output path; stdout is always written",
    )
    parser.add_argument(
        "--connection-mode",
        choices=("close", "keepalive"),
        default="close",
        help="use one TCP connection per request or reuse one connection per worker",
    )
    return parser.parse_args()


def load_body(args: argparse.Namespace) -> bytes:
    if args.body_file:
        return Path(args.body_file).read_bytes()
    if args.body_bytes > 0:
        return b"x" * args.body_bytes
    return b""


def build_headers(args: argparse.Namespace, parsed_url, body: bytes) -> list[tuple[str, str]]:
    headers: list[tuple[str, str]] = []
    host = args.host_header or parsed_url.netloc or parsed_url.hostname or "localhost"
    headers.append(("Host", host))
    headers.append(
        ("Connection", "keep-alive" if args.connection_mode == "keepalive" else "close")
    )
    headers.append(("User-Agent", "pgw-http-concurrency-client/1.0"))
    if body:
        headers.append(("Content-Length", str(len(body))))

    for raw in args.header:
        if ":" not in raw:
            raise SystemExit(f"invalid header {raw!r}; expected 'Name: Value'")
        name, value = raw.split(":", 1)
        headers.append((name.strip(), value.strip()))

    return headers


def build_request_bytes(
    method: str,
    parsed_url,
    headers: list[tuple[str, str]],
) -> bytes:
    path = parsed_url.path or "/"
    if parsed_url.query:
        path = f"{path}?{parsed_url.query}"
    lines = [f"{method} {path} HTTP/1.1"]
    lines.extend(f"{name}: {value}" for name, value in headers)
    return ("\r\n".join(lines) + "\r\n\r\n").encode("ascii")


async def read_response_until_eof(reader: asyncio.StreamReader) -> bytes:
    chunks: list[bytes] = []
    while True:
        chunk = await reader.read(65536)
        if not chunk:
            break
        chunks.append(chunk)
    return b"".join(chunks)


async def read_chunked_body(reader: asyncio.StreamReader) -> bytes:
    chunks: list[bytes] = []
    while True:
        line = await reader.readline()
        if not line:
            break
        size_token = line.split(b";", 1)[0].strip()
        size = int(size_token, 16)
        if size == 0:
            while True:
                trailer = await reader.readline()
                if trailer in (b"\r\n", b"\n", b""):
                    break
            break
        chunks.append(await reader.readexactly(size))
        await reader.readexactly(2)
    return b"".join(chunks)


async def read_response_message(reader: asyncio.StreamReader) -> tuple[int | None, str, int]:
    head = await reader.readuntil(b"\r\n\r\n")
    lines = head.decode("iso-8859-1", errors="replace").split("\r\n")
    parts = lines[0].split(" ", 2) if lines else []
    status = int(parts[1]) if len(parts) >= 2 and parts[1].isdigit() else None
    headers = {}
    for line in lines[1:]:
        if ":" not in line:
            continue
        name, value = line.split(":", 1)
        headers[name.strip().lower()] = value.strip().lower()

    body = b""
    if "content-length" in headers:
        length = int(headers["content-length"])
        body = await reader.readexactly(length) if length > 0 else b""
    elif headers.get("transfer-encoding") == "chunked":
        body = await read_chunked_body(reader)

    return status, body.decode("utf-8", errors="replace"), len(head) + len(body)


async def send_request_payload(
    writer: asyncio.StreamWriter,
    request_prefix: bytes,
    body: bytes,
    args: argparse.Namespace,
) -> None:
    writer.write(request_prefix)
    await writer.drain()

    if not body:
        return

    chunk_size = args.body_chunk_size or len(body)
    offset = 0
    while offset < len(body):
        end = min(offset + chunk_size, len(body))
        writer.write(body[offset:end])
        await writer.drain()
        offset = end
        if offset < len(body) and args.body_chunk_interval_ms > 0:
            await asyncio.sleep(args.body_chunk_interval_ms / 1000.0)


def success_result(
    started: float,
    status: int | None,
    response_body: str,
    bytes_received: int,
    args: argparse.Namespace,
    expected_statuses: set[int],
) -> dict[str, Any]:
    body_match = (
        True if args.expect_body_substring is None else args.expect_body_substring in response_body
    )
    return {
        "ok": status in expected_statuses and body_match,
        "status": status,
        "body_match": body_match,
        "error": None,
        "latency_ms": (time.perf_counter() - started) * 1000.0,
        "bytes_received": bytes_received,
    }


def failure_result(started: float, error: str) -> dict[str, Any]:
    return {
        "ok": False,
        "status": None,
        "body_match": False,
        "error": error,
        "latency_ms": (time.perf_counter() - started) * 1000.0,
        "bytes_received": 0,
    }


def split_response(raw: bytes) -> tuple[int | None, str]:
    if not raw:
        return None, ""

    head, _, body = raw.partition(b"\r\n\r\n")
    lines = head.decode("iso-8859-1", errors="replace").split("\r\n")
    if not lines:
        return None, body.decode("utf-8", errors="replace")

    parts = lines[0].split(" ", 2)
    if len(parts) < 2 or not parts[1].isdigit():
        return None, body.decode("utf-8", errors="replace")

    return int(parts[1]), body.decode("utf-8", errors="replace")


async def execute_request(
    host: str,
    port: int,
    request_prefix: bytes,
    body: bytes,
    args: argparse.Namespace,
    expected_statuses: set[int],
) -> dict[str, Any]:
    started = time.perf_counter()
    writer = None
    try:
        connect = asyncio.open_connection(host, port)
        reader, writer = await asyncio.wait_for(connect, timeout=args.connect_timeout)
        await send_request_payload(writer, request_prefix, body, args)

        raw = await asyncio.wait_for(read_response_until_eof(reader), timeout=args.request_timeout)
        status, response_body = split_response(raw)
        return success_result(started, status, response_body, len(raw), args, expected_statuses)
    except asyncio.TimeoutError:
        return failure_result(started, "timeout")
    except OSError as exc:
        return failure_result(started, str(exc))
    finally:
        if writer is not None:
            writer.close()
            try:
                await writer.wait_closed()
            except Exception:
                pass


async def execute_keepalive_request(
    reader: asyncio.StreamReader,
    writer: asyncio.StreamWriter,
    request_prefix: bytes,
    body: bytes,
    args: argparse.Namespace,
    expected_statuses: set[int],
) -> dict[str, Any]:
    started = time.perf_counter()
    try:
        await send_request_payload(writer, request_prefix, body, args)

        status, response_body, bytes_received = await asyncio.wait_for(
            read_response_message(reader), timeout=args.request_timeout
        )
        return success_result(
            started,
            status,
            response_body,
            bytes_received,
            args,
            expected_statuses,
        )
    except asyncio.TimeoutError:
        return failure_result(started, "timeout")
    except (OSError, asyncio.IncompleteReadError, ValueError) as exc:
        return failure_result(started, str(exc))


def percentile(values: list[float], ratio: float) -> float:
    if not values:
        return 0.0
    if len(values) == 1:
        return values[0]
    index = min(max(math.ceil(len(values) * ratio) - 1, 0), len(values) - 1)
    return values[index]


def summarize(results: list[dict[str, Any]], args: argparse.Namespace) -> dict[str, Any]:
    statuses = Counter()
    errors = Counter()
    latencies: list[float] = []
    bytes_received = 0
    successes = 0
    body_mismatches = 0

    for item in results:
        latencies.append(item["latency_ms"])
        bytes_received += item["bytes_received"]
        if item["status"] is not None:
            statuses[str(item["status"])] += 1
        if item["error"] is not None:
            errors[item["error"]] += 1
        if item["body_match"]:
            pass
        elif args.expect_body_substring is not None:
            body_mismatches += 1
        if item["ok"]:
            successes += 1

    latencies.sort()
    summary = {
        "url": args.url,
        "requests": args.requests,
        "concurrency": args.concurrency,
        "connection_mode": args.connection_mode,
        "method": args.method.upper(),
        "completed": len(results),
        "successes": successes,
        "success_rate": (successes / len(results)) if results else 0.0,
        "body_mismatches": body_mismatches,
        "bytes_received": bytes_received,
        "status_counts": dict(sorted(statuses.items())),
        "error_counts": dict(sorted(errors.items())),
        "latency_ms": {
            "min": latencies[0] if latencies else 0.0,
            "mean": statistics.fmean(latencies) if latencies else 0.0,
            "p50": percentile(latencies, 0.50),
            "p90": percentile(latencies, 0.90),
            "p95": percentile(latencies, 0.95),
            "p99": percentile(latencies, 0.99),
            "max": latencies[-1] if latencies else 0.0,
        },
    }
    return summary


async def run(args: argparse.Namespace) -> dict[str, Any]:
    parsed_url = urlparse(args.url)
    if parsed_url.scheme != "http":
        raise SystemExit("only plain http:// URLs are supported")
    if not parsed_url.hostname:
        raise SystemExit("URL must include a hostname")

    body = load_body(args)
    expected_statuses = set(args.expect_status or [200])
    headers = build_headers(args, parsed_url, body)
    request_prefix = build_request_bytes(args.method.upper(), parsed_url, headers)
    results: list[dict[str, Any]] = []

    async def run_one_close() -> None:
        semaphore = close_semaphore
        async with semaphore:
            results.append(
                await execute_request(
                    parsed_url.hostname,
                    parsed_url.port or 80,
                    request_prefix,
                    body,
                    args,
                    expected_statuses,
                )
            )

    async def run_keepalive_worker(request_count: int) -> None:
        if request_count <= 0:
            return
        reader = None
        writer = None
        try:
            connect = asyncio.open_connection(parsed_url.hostname, parsed_url.port or 80)
            reader, writer = await asyncio.wait_for(connect, timeout=args.connect_timeout)
            for _ in range(request_count):
                results.append(
                    await execute_keepalive_request(
                        reader,
                        writer,
                        request_prefix,
                        body,
                        args,
                        expected_statuses,
                    )
                )
        except (asyncio.TimeoutError, OSError) as exc:
            for _ in range(request_count):
                results.append(
                    {
                        "ok": False,
                        "status": None,
                        "body_match": False,
                        "error": str(exc) if not isinstance(exc, asyncio.TimeoutError) else "timeout",
                        "latency_ms": args.connect_timeout * 1000.0,
                        "bytes_received": 0,
                    }
                )
        finally:
            if writer is not None:
                writer.close()
                try:
                    await writer.wait_closed()
                except Exception:
                    pass

    if args.connection_mode == "keepalive":
        workers = min(args.concurrency, args.requests)
        base = args.requests // workers
        extra = args.requests % workers
        await asyncio.gather(
            *(
                run_keepalive_worker(base + (1 if index < extra else 0))
                for index in range(workers)
            )
        )
    else:
        close_semaphore = asyncio.Semaphore(args.concurrency)
        await asyncio.gather(*(run_one_close() for _ in range(args.requests)))
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
