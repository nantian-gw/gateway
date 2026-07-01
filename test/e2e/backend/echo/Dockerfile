FROM golang:1.24-alpine AS builder

WORKDIR /build

COPY go.mod ./
RUN go mod download

COPY main.go ./
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o echo-server .

FROM scratch

COPY --from=builder /build/echo-server /echo-server

EXPOSE 8080
ENTRYPOINT ["/echo-server"]
