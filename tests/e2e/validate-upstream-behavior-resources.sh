render_resources() {
  cat >"${TMP_DIR}/resources.yaml" <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${TEST_NAMESPACE}
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: upstream-server-script
  namespace: ${TEST_NAMESPACE}
data:
  server.py: |
    import os
    import socketserver
    import threading
    import time
    from http.server import BaseHTTPRequestHandler

    MODE = os.environ.get("MODE", "echo")
    RESPONSE_BODY = os.environ.get("RESPONSE_BODY", "ok")
    STATUS_CODE = int(os.environ.get("STATUS_CODE", "200"))
    RESPONSE_DELAY_MS = int(os.environ.get("RESPONSE_DELAY_MS", "0"))
    STREAM_IDLE_GAP_MS = int(os.environ.get("STREAM_IDLE_GAP_MS", "1500"))

    class ThreadedServer(socketserver.ThreadingMixIn, socketserver.TCPServer):
        allow_reuse_address = True
        daemon_threads = True

        def __init__(self, addr, handler):
            super().__init__(addr, handler)
            self._lock = threading.Lock()
            self._next_conn_id = 0
            self._connection_ids = {}
            self._request_counts = {}

        def get_request(self):
            request, client_address = super().get_request()
            with self._lock:
                self._next_conn_id += 1
                self._connection_ids[request.fileno()] = self._next_conn_id
                self._request_counts.setdefault(request.fileno(), 0)
            return request, client_address

        def connection_id(self, sock):
            with self._lock:
                return self._connection_ids.get(sock.fileno(), 0)

        def increment_request_count(self, sock):
            with self._lock:
                fileno = sock.fileno()
                self._request_counts[fileno] = self._request_counts.get(fileno, 0) + 1
                return self._request_counts[fileno]

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def log_message(self, format, *args):
            return

        def write_chunk(self, payload):
            body = payload.encode("utf-8")
            self.wfile.write(f"{len(body):x}\r\n".encode("ascii"))
            self.wfile.write(body)
            self.wfile.write(b"\r\n")
            self.wfile.flush()

        def do_GET(self):
            connection_id = self.server.connection_id(self.connection)
            if self.path == "/stats":
                with self.server._lock:
                    body = (
                        f"connections={self.server._next_conn_id}\n"
                        f"requests={sum(self.server._request_counts.values())}\n"
                    ).encode("utf-8")
                self.send_response(200)
                self.send_header("Content-Type", "text/plain")
                self.send_header("Content-Length", str(len(body)))
                self.send_header("Connection", "close")
                self.end_headers()
                self.wfile.write(body)
                self.close_connection = True
                return

            request_count = self.server.increment_request_count(self.connection)
            if MODE == "streaming":
                self.send_response(200)
                self.send_header("Content-Type", "text/event-stream")
                self.send_header("Cache-Control", "no-cache")
                self.send_header("Transfer-Encoding", "chunked")
                self.send_header("X-Backend-Connection-Id", str(connection_id))
                self.send_header("X-Backend-Connection-Request", str(request_count))
                self.end_headers()
                self.write_chunk("event: open\ndata: first\n\n")
                time.sleep(STREAM_IDLE_GAP_MS / 1000.0)
                self.write_chunk("event: update\ndata: second\n\n")
                self.wfile.write(b"0\r\n\r\n")
                self.wfile.flush()
                return

            if RESPONSE_DELAY_MS > 0:
                time.sleep(RESPONSE_DELAY_MS / 1000.0)

            body = RESPONSE_BODY.encode("utf-8")
            self.send_response(STATUS_CODE)
            self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(body)))
            self.send_header("X-Backend-Connection-Id", str(connection_id))
            self.send_header("X-Backend-Connection-Request", str(request_count))
            self.end_headers()
            self.wfile.write(body)

    if __name__ == "__main__":
        server = ThreadedServer(("0.0.0.0", 8080), Handler)
        server.serve_forever()
---
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  name: nantian
spec:
  controllerName: gateway.networking.k8s.io/nantian-gw
---
apiVersion: v1
kind: Service
metadata:
  name: pool-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: pool-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: pool-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: pool-backend
  template:
    metadata:
      labels:
        app: pool-backend
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: MODE
              value: echo
            - name: RESPONSE_BODY
              value: pool-backend
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: streaming-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: streaming-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: streaming-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: streaming-backend
  template:
    metadata:
      labels:
        app: streaming-backend
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: MODE
              value: streaming
            - name: STREAM_IDLE_GAP_MS
              value: "${STREAM_IDLE_GAP_MS}"
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: retry-failing
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: retry-failing
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: retry-failing
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: retry-failing
  template:
    metadata:
      labels:
        app: retry-failing
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: STATUS_CODE
              value: "503"
            - name: RESPONSE_BODY
              value: retry-failing
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: retry-healthy
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: retry-healthy
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: retry-healthy
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: retry-healthy
  template:
    metadata:
      labels:
        app: retry-healthy
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: retry-healthy
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: weighted-a
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: weighted-a
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: weighted-a
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: weighted-a
  template:
    metadata:
      labels:
        app: weighted-a
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: weighted-a
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: weighted-b
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: weighted-b
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: weighted-b
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: weighted-b
  template:
    metadata:
      labels:
        app: weighted-b
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: weighted-b
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: recover-a
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: recover-a
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: recover-a
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: recover-a
  template:
    metadata:
      labels:
        app: recover-a
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: recover-a
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: recover-b
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: recover-b
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: recover-b
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: recover-b
  template:
    metadata:
      labels:
        app: recover-b
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: recover-b
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: slow-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: slow-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: slow-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: slow-backend
  template:
    metadata:
      labels:
        app: slow-backend
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: slow-backend
            - name: RESPONSE_DELAY_MS
              value: "1500"
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: v1
kind: Service
metadata:
  name: fast-backend
  namespace: ${TEST_NAMESPACE}
spec:
  selector:
    app: fast-backend
  ports:
    - name: http
      port: 8080
      targetPort: 8080
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: fast-backend
  namespace: ${TEST_NAMESPACE}
spec:
  replicas: 1
  selector:
    matchLabels:
      app: fast-backend
  template:
    metadata:
      labels:
        app: fast-backend
    spec:
      containers:
        - name: server
          image: ${PYTHON_IMAGE}
          imagePullPolicy: IfNotPresent
          command: ["python3", "/app/server.py"]
          env:
            - name: RESPONSE_BODY
              value: fast-backend
          ports:
            - containerPort: 8080
          volumeMounts:
            - name: script
              mountPath: /app
              readOnly: true
      volumes:
        - name: script
          configMap:
            name: upstream-server-script
---
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  name: upstream-edge
  namespace: ${TEST_NAMESPACE}
spec:
  gatewayClassName: nantian
  listeners:
    - name: http
      protocol: HTTP
      port: 80
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: pool-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${POOL_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /pool
      backendRefs:
        - name: pool-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: streaming-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${STREAM_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /stream
      backendRefs:
        - name: streaming-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: retry-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${RETRY_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /retry
      retry:
        attempts: 2
        codes:
          - 503
      backendRefs:
        - name: retry-failing
          port: 8080
        - name: retry-healthy
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: timeout-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${TIMEOUT_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /timeout
      timeouts:
        backendRequest: 200ms
        request: 2s
      retry:
        attempts: 2
      backendRefs:
        - name: slow-backend
          port: 8080
        - name: fast-backend
          port: 8080
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: weighted-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${WEIGHT_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /weight
      backendRefs:
        - name: weighted-a
          port: 8080
          weight: 1
        - name: weighted-b
          port: 8080
          weight: 3
---
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  name: recover-route
  namespace: ${TEST_NAMESPACE}
spec:
  parentRefs:
    - name: upstream-edge
      sectionName: http
  hostnames:
    - ${RECOVER_HOST}
  rules:
    - matches:
        - path:
            type: PathPrefix
            value: /recover
      backendRefs:
        - name: recover-a
          port: 8080
        - name: recover-b
          port: 8080
EOF
}
