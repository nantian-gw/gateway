#!/usr/bin/env python3

import asyncio
import base64
import hashlib
import importlib.util
from pathlib import Path
from types import SimpleNamespace


ROOT = Path(__file__).resolve().parents[2]
CLIENT_PATH = ROOT / "tests" / "e2e" / "websocket_concurrency_client.py"
GUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"


def load_client():
    spec = importlib.util.spec_from_file_location("websocket_concurrency_client", CLIENT_PATH)
    if spec is None or spec.loader is None:
        raise RuntimeError("failed to load websocket_concurrency_client.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


async def read_frame(reader):
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


def text_frame(payload):
    body = payload.encode("utf-8") if isinstance(payload, str) else payload
    header = bytearray([0x81])
    if len(body) < 126:
        header.append(len(body))
    elif len(body) < 65536:
        header.append(126)
        header.extend(len(body).to_bytes(2, "big"))
    else:
        header.append(127)
        header.extend(len(body).to_bytes(8, "big"))
    return bytes(header) + body


class WebSocketEchoServer:
    def __init__(self):
        self.connections = 0
        self.messages = 0
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
        self.connections += 1
        try:
            header = await reader.readuntil(b"\r\n\r\n")
            lines = header.decode("latin1").split("\r\n")
            headers = {}
            for line in lines[1:]:
                if ":" not in line:
                    continue
                name, value = line.split(":", 1)
                headers[name.strip().lower()] = value.strip()

            key = headers.get("sec-websocket-key", "")
            accept = base64.b64encode(
                hashlib.sha1((key + GUID).encode("ascii")).digest()
            ).decode("ascii")
            writer.write(
                b"HTTP/1.1 101 Switching Protocols\r\n"
                b"Upgrade: websocket\r\n"
                b"Connection: Upgrade\r\n"
                + f"Sec-WebSocket-Accept: {accept}\r\n\r\n".encode("ascii")
            )
            await writer.drain()

            opcode, payload = await read_frame(reader)
            if opcode == 0x1:
                self.messages += 1
                writer.write(text_frame(payload))
                await writer.drain()
        finally:
            writer.close()
            await writer.wait_closed()


def client_args(port):
    return SimpleNamespace(
        url=f"ws://127.0.0.1:{port}/ws",
        requests=6,
        concurrency=3,
        host_header="ws.example.com",
        payload="nantian-websocket",
        connect_timeout=1.0,
        request_timeout=1.0,
        hold_ms=10,
        scenario="long-lived-streaming",
        output=None,
    )


async def assert_websocket_client_reports_echo_profile(client):
    async with WebSocketEchoServer() as server:
        summary = await client.run(client_args(server.port))

    assert server.connections == 6
    assert server.messages == 6
    assert summary["protocol"] == "websocket"
    assert summary["scenario"] == "long-lived-streaming"
    assert summary["requests"] == 6
    assert summary["completed"] == 6
    assert summary["successes"] == 6
    assert summary["success_rate"] == 1.0
    assert summary["upgrade_successes"] == 6
    assert summary["messages_sent"] == 6
    assert summary["messages_received"] == 6
    assert summary["connection_count"] == 6
    assert summary["error_counts"] == {}
    assert summary["latency_ms"]["p99"] >= 10


async def main():
    client = load_client()
    await assert_websocket_client_reports_echo_profile(client)


if __name__ == "__main__":
    asyncio.run(main())
