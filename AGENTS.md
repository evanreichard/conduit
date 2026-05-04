# Conduit — Agent Guidelines

## Project Overview

Conduit is a self-hosted tunneling service (Go, single binary). A **server** (`conduit serve`) runs on a public host and routes incoming HTTP requests by subdomain to registered **tunnels**. A **client** (`conduit tunnel`) connects via WebSocket, receives forwarded traffic, and relays it to a local target using either an HTTP reverse-proxy or raw TCP dial.

## Build & Test

```bash
# Build all platforms
make build_local

# Run tests
make tests              # includes coverage

# Lint
golangci-lint run
```

Go 1.25+ is required (`go.mod`). Nix devshell provides Go, gopls, golangci-lint.

## Architecture at a Glance

```
server/server.go        — net/http.Server, ServeHTTP routes by Host subdomain to tunnel or control API
tunnel/tunnel.go        — Core Tunnel struct, WebSocket message loop, stream management
tunnel/forwarder.go     — Forwarder interface; HTTP/HTTPS → HTTP forwarder, everything else → TCP
tunnel/http_forwarder.go — httputil.ReverseProxy served over net.Pipe via multiConnListener
tunnel/tcp_forwarder.go  — Direct net.Dial TCP forwarding
tunnel/stream.go        — Stream interface (io.ReadWriteCloser + Source/Target)
server/reconstructed_conn.go — Replays re-serialized headers + buffered body + raw conn after hijack
store/store.go          — In-memory request/response recorder with pub/sub (SSE)
web/web.go              — Local tunnel monitor (port 8181), SSE endpoint
config/config.go        — Reflection-based config from struct tags → flags + env vars + client config file
pkg/maps/map.go         — Generic sync.RWMutex-guarded map
```

## Code Conventions

- **Go style**: standard `gofmt`, golangci-lint with `.golangci.toml`
- **Comment style**: Title Case heading above logical blocks (see root `AGENTS.md`)
- **Config**: add struct tags (`json`, `default`, `description`) to `ServerConfig` or `ClientConfig` — flags and env vars are auto-derived. Client config may also come from `./conduit.json` or `~/.config/conduit/config.json` for `server`, `api_key`, `log_level`, and `log_format` only.
- **Logging**: use `logrus` (`log` alias); structured fields preferred
- **Concurrency**: use `pkg/maps.Map` for shared maps; protect other shared state with `sync.Mutex`
- **Error handling**: return errors up; log at command/entry-point level. Use `fmt.Errorf` with `%w` for wrapping

## Key Patterns

1. **ServeHTTP + Hijack for tunnel routing**: The server uses `net/http.Server` with a `ServeHTTP` handler. It inspects the `Host` header to determine routing. Control API requests are handled normally via `http.ResponseWriter`. Tunnel requests hijack the TCP connection, re-serialize the HTTP request, and forward it through the tunnel via `reconstructedConn`.
2. **Reconstructed connection**: After hijack, `reconstructedConn` combines re-serialized request headers, buffered body data from the hijacked `bufio.ReadWriter`, and the raw connection into a single `io.Reader` so the tunnel client receives the complete request.
3. **Forwarder abstraction**: `Forwarder` interface decouples tunnel transport from protocol handling. `NewForwarder` only uses `url.Parse` for `http://`/`https://` schemes; everything else (including parse failures like bare `host:port`) is treated as raw TCP. HTTP forwarder uses `net.Pipe` + `multiConnListener` to feed connections into a standard `http.Server`.
4. **Context-threaded records**: Request records are attached to context in `RecordRequest` and retrieved in `RecordResponse` via the `ModifyResponse` hook.

## Adding a New Forwarder

1. Implement `tunnel.Forwarder` interface (`Type()`, `Initialize()`, `Start()`)
2. Add a case in `tunnel.NewForwarder()` factory
3. Add corresponding `ForwarderType` const

## Testing

E2E tests live in `e2e_test.go` at the project root. They spin up real servers, tunnels, and targets on random ports.

```bash
# Run all tests
make tests

# Run specific test
 go test -v -run TestHTTPTunnelRoundTrip -count=1 ./...
```

16 tests covering: HTTP round-trip (GET/POST), TCP echo, large bodies (1MB resp / 512KB req), response quality (headers, content-type, content-length), error paths (404, 401, duplicate name), multi-tunnel routing, concurrency, and graceful shutdown.

## File Locations

| Concern | Files |
|---------|-------|
| CLI entry | `main.go`, `cmd/` |
| Server | `server/` |
| Tunneling | `tunnel/` |
| Config | `config/` |
| Storage | `store/` |
| Web UI | `web/`, `web/pages/` |
| Shared types | `types/` |
| Utilities | `pkg/maps/` |
| E2E tests | `e2e_test.go` |
