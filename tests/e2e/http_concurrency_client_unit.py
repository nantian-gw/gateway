#!/usr/bin/env python3

import asyncio
import importlib.util
from pathlib import Path
from types import SimpleNamespace


ROOT = Path(__file__).resolve().parents[2]
CLIENT_PATH = ROOT / "tests" / "e2e" / "http_concurrency_client.py"


def load_client():
    spec = importlib.util.spec_from_file_location("http_concurrency_client", CLIENT_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load http_concurrency_client.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class CountingHttpServer:
    def __init__(self):
        self.accepted_connections = 0
        self.requests = 0
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
            while True:
                try:
                    header = await reader.readuntil(b"\r\n\r\n")
                except asyncio.IncompleteReadError:
                    break
                except asyncio.LimitOverrunError:
                    break

                self.requests += 1
                connection_close = b"connection: close" in header.lower()
                connection_header = b"close" if connection_close else b"keep-alive"
                body = b"nantian-gw-ok"
                writer.write(
                    b"HTTP/1.1 200 OK\r\n"
                    + b"Content-Length: "
                    + str(len(body)).encode("ascii")
                    + b"\r\nConnection: "
                    + connection_header
                    + b"\r\n\r\n"
                    + body
                )
                await writer.drain()
                if connection_close:
                    break
        finally:
            writer.close()
            await writer.wait_closed()


def client_args(port, connection_mode):
    return SimpleNamespace(
        url=f"http://127.0.0.1:{port}/",
        requests=6,
        concurrency=3,
        method="GET",
        header=[],
        host_header="example.com",
        body_file=None,
        body_bytes=0,
        body_chunk_size=0,
        body_chunk_interval_ms=0,
        connect_timeout=1.0,
        request_timeout=1.0,
        expect_status=[200],
        expect_body_substring="nantian-gw-ok",
        output=None,
        connection_mode=connection_mode,
    )


async def assert_close_mode_opens_one_connection_per_request(client):
    async with CountingHttpServer() as server:
        summary = await client.run(client_args(server.port, "close"))

    assert summary["success_rate"] == 1.0
    assert server.requests == 6
    assert server.accepted_connections == 6
    assert summary["connection_mode"] == "close"


async def assert_keepalive_mode_reuses_worker_connections(client):
    async with CountingHttpServer() as server:
        summary = await client.run(client_args(server.port, "keepalive"))

    assert summary["success_rate"] == 1.0
    assert server.requests == 6
    assert server.accepted_connections == 3
    assert summary["connection_mode"] == "keepalive"


async def main():
    client = load_client()
    await assert_close_mode_opens_one_connection_per_request(client)
    await assert_keepalive_mode_reuses_worker_connections(client)


if __name__ == "__main__":
    asyncio.run(main())
