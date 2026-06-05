#!/usr/bin/env python3

import argparse
import random
import socket
import struct


def encode_name(name: str) -> bytes:
    labels = name.rstrip(".").split(".")
    return b"".join(bytes((len(label),)) + label.encode("ascii") for label in labels) + b"\x00"


def build_query(name: str, query_id: int) -> bytes:
    header = struct.pack("!HHHHHH", query_id, 0x0100, 1, 0, 0, 0)
    question = encode_name(name) + struct.pack("!HH", 1, 1)
    return header + question


def main() -> int:
    parser = argparse.ArgumentParser(description="UDP DNS smoke probe")
    parser.add_argument("--addr", required=True, help="target host:port")
    parser.add_argument("--name", default="foo.bar.com", help="dns name to query")
    parser.add_argument("--timeout", type=float, default=5.0, help="socket timeout in seconds")
    parser.add_argument(
        "--expect-timeout",
        action="store_true",
        help="treat a receive timeout as success and any response as failure",
    )
    args = parser.parse_args()

    host, port_text = args.addr.rsplit(":", 1)
    port = int(port_text)
    query_id = random.randint(0, 0xFFFF)
    query = build_query(args.name, query_id)

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.settimeout(args.timeout)
    try:
        sock.sendto(query, (host, port))
        try:
            response, _ = sock.recvfrom(4096)
        except TimeoutError:
            if args.expect_timeout:
                return 0
            raise
    finally:
        sock.close()

    if args.expect_timeout:
        raise SystemExit("received a dns response when timeout was expected")

    if len(response) < 12:
        raise SystemExit("short dns response")

    resp_id, flags, qdcount, ancount, nscount, arcount = struct.unpack("!HHHHHH", response[:12])
    if resp_id != query_id:
        raise SystemExit("dns id mismatch")
    if (flags & 0x000F) != 0:
        raise SystemExit(f"dns rcode was {flags & 0x000F}")
    if qdcount != 1:
        raise SystemExit(f"unexpected question count {qdcount}")
    if (ancount + nscount + arcount) < 1:
        raise SystemExit("dns response contained no resource records")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
