#!/usr/bin/env python3

import asyncio
import importlib.util
from pathlib import Path
from types import SimpleNamespace


ROOT = Path(__file__).resolve().parents[2]
CLIENT_PATH = ROOT / "tests" / "e2e" / "tcp_concurrency_client.py"


def load_client():
    spec = importlib.util.spec_from_file_location("tcp_concurrency_client", CLIENT_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load tcp_concurrency_client.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class TcpEchoServer:
    def __init__(self):
        self.accepted_connections = 0
        self.received_payloads = 0
        self.server = None
        self.port = 0

    async def __aenter__(self):
        self.server = await asyncio.start_server(self.handle, "127.0.0.1", 0)
        socket = self.server.sockets[0]
        self.port = socket.getsockname()[1]
        return self

    async def __aexit__(self, *_):
        self.server.close()
        await self.server.wait_closed()

    async def handle(self, reader, writer):
        self.accepted_connections += 1
        try:
            await reader.readuntil(b"\r\n\r\n")
            self.received_payloads += 1
            body = b"nantian-gw-ok"
            writer.write(
                b"HTTP/1.1 200 OK\r\n"
                + b"Content-Length: "
                + str(len(body)).encode("ascii")
                + b"\r\nConnection: close\r\n\r\n"
                + body
            )
            await writer.drain()
        finally:
            writer.close()
            await writer.wait_closed()


def client_args(port):
    return SimpleNamespace(
        addr=f"127.0.0.1:{port}",
        requests=8,
        concurrency=4,
        payload=None,
        payload_file=None,
        host_header="example.com",
        expect_substring="nantian-gw-ok",
        connect_timeout=1.0,
        request_timeout=1.0,
        output=None,
        scenario="steady",
    )


async def assert_tcp_client_reports_concurrency_profile(client):
    async with TcpEchoServer() as server:
        summary = await client.run(client_args(server.port))

    assert server.received_payloads == 8
    assert server.accepted_connections == 8
    assert summary["requests"] == 8
    assert summary["completed"] == 8
    assert summary["successes"] == 8
    assert summary["success_rate"] == 1.0
    assert summary["concurrency"] == 4
    assert summary["connection_count"] == 4
    assert summary["connections_opened"] == 8
    assert summary["scenario"] == "steady"
    assert summary["bytes_received"] > 0
    assert summary["latency_ms"]["p99"] >= summary["latency_ms"]["p50"]


async def main():
    client = load_client()
    await assert_tcp_client_reports_concurrency_profile(client)


if __name__ == "__main__":
    asyncio.run(main())
