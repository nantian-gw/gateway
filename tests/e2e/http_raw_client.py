#!/usr/bin/env python3

import argparse
import base64
import json
import re
import socket
import sys
import time
from pathlib import Path


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Send raw HTTP bytes to a TCP listener and capture the response."
    )
    parser.add_argument("--host", required=True, help="target host")
    parser.add_argument("--port", required=True, type=int, help="target port")
    parser.add_argument(
        "--payload-file",
        help="file containing the raw bytes to send; stdin is used when omitted",
    )
    parser.add_argument(
        "--connect-timeout",
        type=float,
        default=5.0,
        help="socket connect timeout in seconds",
    )
    parser.add_argument(
        "--read-timeout",
        type=float,
        default=1.0,
        help="socket read timeout in seconds",
    )
    parser.add_argument(
        "--send-chunk-size",
        type=int,
        default=0,
        help="send payload in chunks of this size; 0 sends everything at once",
    )
    parser.add_argument(
        "--send-interval-ms",
        type=int,
        default=0,
        help="sleep between send chunks",
    )
    parser.add_argument(
        "--linger-ms",
        type=int,
        default=0,
        help="sleep after the payload is sent before shutdown or close",
    )
    parser.add_argument(
        "--skip-read",
        action="store_true",
        help="do not read the response body; useful for slow-connection probes",
    )
    parser.add_argument(
        "--no-shutdown",
        action="store_true",
        help="do not call shutdown(SHUT_WR) after sending the payload",
    )
    return parser.parse_args()


def load_payload(path: str | None) -> bytes:
    if path:
        return Path(path).read_bytes()
    return sys.stdin.buffer.read()


def status_lines(text: str) -> list[str]:
    return re.findall(r"HTTP/1\.[01] \d{3} [^\r\n]+", text)


def main() -> int:
    args = parse_args()
    payload = load_payload(args.payload_file)
    chunk_size = args.send_chunk_size or max(len(payload), 1)
    response_chunks: list[bytes] = []
    result: dict[str, object] = {
        "host": args.host,
        "port": args.port,
        "sent_bytes": 0,
        "received_bytes": 0,
        "timed_out": False,
        "connect_error": None,
        "text": "",
        "raw_base64": "",
        "status_lines": [],
    }

    try:
        with socket.create_connection(
            (args.host, args.port), timeout=args.connect_timeout
        ) as sock:
            sock.settimeout(args.read_timeout)
            offset = 0
            while offset < len(payload):
                end = min(offset + chunk_size, len(payload))
                sock.sendall(payload[offset:end])
                result["sent_bytes"] = int(result["sent_bytes"]) + (end - offset)
                offset = end
                if offset < len(payload) and args.send_interval_ms > 0:
                    time.sleep(args.send_interval_ms / 1000.0)

            if args.linger_ms > 0:
                time.sleep(args.linger_ms / 1000.0)

            if not args.no_shutdown:
                try:
                    sock.shutdown(socket.SHUT_WR)
                except OSError:
                    pass

            if not args.skip_read:
                while True:
                    try:
                        chunk = sock.recv(65536)
                    except socket.timeout:
                        result["timed_out"] = True
                        break
                    if not chunk:
                        break
                    response_chunks.append(chunk)
                    result["received_bytes"] = int(result["received_bytes"]) + len(chunk)
    except OSError as exc:
        result["connect_error"] = str(exc)

    raw = b"".join(response_chunks)
    text = raw.decode("iso-8859-1", errors="replace")
    result["text"] = text
    result["raw_base64"] = base64.b64encode(raw).decode("ascii")
    result["status_lines"] = status_lines(text)
    json.dump(result, sys.stdout, sort_keys=True)
    sys.stdout.write("\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
