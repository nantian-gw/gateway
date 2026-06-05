#!/usr/bin/env python3

import asyncio
import importlib.util
import socket
import struct
from pathlib import Path
from types import SimpleNamespace


ROOT = Path(__file__).resolve().parents[2]
CLIENT_PATH = ROOT / "tests" / "e2e" / "udp_dns_concurrency_client.py"


def load_client():
    spec = importlib.util.spec_from_file_location("udp_dns_concurrency_client", CLIENT_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load udp_dns_concurrency_client.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def dns_response(query: bytes) -> bytes:
    query_id = struct.unpack("!H", query[:2])[0]
    question = query[12:]
    header = struct.pack("!HHHHHH", query_id, 0x8180, 1, 1, 0, 0)
    answer = (
        b"\xc0\x0c"
        + struct.pack("!HHIH", 1, 1, 30, 4)
        + socket.inet_aton("127.0.0.1")
    )
    return header + question + answer


class UdpDnsServer:
    def __init__(self, drop_every=0):
        self.drop_every = drop_every
        self.received = 0
        self.transport = None
        self.port = 0
        self.ready = asyncio.Event()

    async def __aenter__(self):
        loop = asyncio.get_running_loop()
        self.transport, _ = await loop.create_datagram_endpoint(
            lambda: self,
            local_addr=("127.0.0.1", 0),
        )
        self.port = self.transport.get_extra_info("sockname")[1]
        self.ready.set()
        return self

    async def __aexit__(self, *_):
        self.transport.close()

    def connection_made(self, transport):
        self.transport = transport

    def connection_lost(self, exc):
        pass

    def error_received(self, exc):
        pass

    def datagram_received(self, data, addr):
        self.received += 1
        if self.drop_every and self.received % self.drop_every == 0:
            return
        self.transport.sendto(dns_response(data), addr)


def client_args(port, socket_mode):
    return SimpleNamespace(
        addr=f"127.0.0.1:{port}",
        requests=8,
        concurrency=4,
        clients=0,
        name="foo.bar.com",
        timeout=1.0,
        output=None,
        scenario="multi-client" if socket_mode == "per-worker" else "high-churn",
        socket_mode=socket_mode,
        expect_timeout=False,
        upstream_count=1,
    )


async def run_in_thread(client, args):
    return await asyncio.to_thread(client.run_sync, args)


async def assert_udp_client_reports_worker_socket_profile(client):
    async with UdpDnsServer() as server:
        summary = await run_in_thread(client, client_args(server.port, "per-worker"))

    assert server.received == 8
    assert summary["requests"] == 8
    assert summary["completed"] == 8
    assert summary["successes"] == 8
    assert summary["packets_sent"] == 8
    assert summary["packets_received"] == 8
    assert summary["packets_lost"] == 0
    assert summary["success_rate"] == 1.0
    assert summary["client_count"] == 4
    assert summary["session_opens"] == 4
    assert summary["scenario"] == "multi-client"
    assert summary["latency_ms"]["p99"] >= summary["latency_ms"]["p50"]


async def assert_udp_client_reports_high_churn_packet_loss(client):
    async with UdpDnsServer(drop_every=2) as server:
        summary = await run_in_thread(client, client_args(server.port, "per-request"))

    assert server.received == 8
    assert summary["requests"] == 8
    assert summary["completed"] == 8
    assert summary["successes"] == 4
    assert summary["packets_sent"] == 8
    assert summary["packets_received"] == 4
    assert summary["packets_lost"] == 4
    assert summary["client_count"] == 8
    assert summary["session_opens"] == 8
    assert summary["scenario"] == "high-churn"


async def assert_udp_client_reports_expected_timeout_profile(client):
    async with UdpDnsServer(drop_every=1) as server:
        args = client_args(server.port, "per-worker")
        args.scenario = "backend-timeout"
        args.timeout = 0.05
        args.expect_timeout = True
        summary = await run_in_thread(client, args)

    assert server.received == 8
    assert summary["requests"] == 8
    assert summary["completed"] == 8
    assert summary["successes"] == 8
    assert summary["success_rate"] == 1.0
    assert summary["expected_timeout"] is True
    assert summary["packets_sent"] == 8
    assert summary["packets_received"] == 0
    assert summary["packets_lost"] == 8
    assert summary["error_counts"]["timeout"] == 8
    assert summary["scenario"] == "backend-timeout"


async def assert_udp_client_reports_upstream_count_override(client):
    async with UdpDnsServer() as server:
        args = client_args(server.port, "per-worker")
        args.scenario = "multi-upstream"
        args.upstream_count = 2
        summary = await run_in_thread(client, args)

    assert server.received == 8
    assert summary["requests"] == 8
    assert summary["successes"] == 8
    assert summary["upstream_count"] == 2
    assert summary["scenario"] == "multi-upstream"


async def main():
    client = load_client()
    await assert_udp_client_reports_worker_socket_profile(client)
    await assert_udp_client_reports_high_churn_packet_loss(client)
    await assert_udp_client_reports_expected_timeout_profile(client)
    await assert_udp_client_reports_upstream_count_override(client)


if __name__ == "__main__":
    asyncio.run(main())
