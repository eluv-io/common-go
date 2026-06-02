# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Repository Overview

`github.com/eluv-io/common-go` is a shared Go library providing general-purpose data types, serialization formats, utilities,
and media processing primitives used across Eluvio's content fabric system.

## Commands

```bash
# Build
go build ./...

# Test all packages
go test ./...

# Run a single test
go test ./path/to/package -run TestName

# Run benchmarks
go test ./path/to/package -bench=. -benchmem

# Run tests with race detector
go test -race ./...
```

## Architecture

The repository is organized into three main top-level packages:

### `format/`

Self-describing, multiformats-based types for Eluvio's content fabric. All types use varint length headers and
multicodec paths for self-description.

- **`hash/`** — Content digests with support for multiple hash/encryption schemes
- **`id/`** — Entity identifiers (QID, QLibID, QNodeID, AccountID, etc.) with base58 encoding
- **`token/`** (`eat/`) — Eluvio Authorization Tokens with multiple signature types; complex nested format with embedded
  signatures and payloads
- **`structured/`** — Generic JSON-like data manipulation: path access, merging, filtering, flattening, JSONPath queries
- **`codecs/`** — CBOR, JSON, GOB codecs
- **`encryption/`, `sign/`, `keys/`, `drm/`** — Cryptographic scheme definitions

The `format` package uses a **Factory pattern with dependency injection** (`github.com/eluv-io/inject-go`). A `Factory`
interface provides constructors for all format objects. Tests create factories via `NewTestFactory(t)` /
`NewTestInjector(t)`.

### `util/`

40+ utility packages organized by domain. Key ones:

- **`lru/`** — LRU cache with atomic `GetOrCreate`, expiring entries, ref counting, multiple construction modes
  (Blocking, Concurrent, Decoupled)
- **`syncutil/`** — Synchronization primitives
- **`structured/`** (under format) — Reusable generic data traversal
- **`httputil/`, `ginutil/`** — HTTP/Gin web framework utilities
- **`traceutil/`, `statsutil/`, `histogram/`** — Observability (HDR histogram metrics, distributed tracing)
- **`ethutil/`** — Ethereum utilities (SECP256K1, EIP191)

### `media/`

Media packetization, transport, and timing:

- **`rtp/`** — RTP packet parsing, payload extraction, streaming
- **`mpegts/`** — MPEG-TS packet handling, stream sync modes (modulo, once, continuous), PCR unwrapping
- **`pacer/`** — Async packet pacing; two implementations: callback-based and disruptor-based (lock-free ring buffer)
- **`pktpool/`** — Packet memory pooling to eliminate per-call heap allocations

Key interfaces: `Packetizer`, `Pacer`, `AsyncPacer`, `CallbackPacer`, `Transformer`.

## Key Dependencies

| Package | Purpose |
|---------|---------|
| `github.com/eluv-io/errors-go` | Rich contextual error wrapping |
| `github.com/eluv-io/log-go` | Structured logging (package-level loggers per module) |
| `github.com/eluv-io/inject-go` | Dependency injection |
| `github.com/eluv-io/utc-go` | UTC time type |
| `github.com/fxamacker/cbor/v2` | CBOR encoding (Eluv fork) |
| `github.com/ethereum/go-ethereum` | Ethereum crypto |
| `github.com/pion/rtp` | RTP packet handling |
| `github.com/Comcast/gots/v2` | MPEG-TS packet support |
| `github.com/smarty/go-disruptor` | Lock-free ring buffer (TS pacer) |
| `github.com/stretchr/testify` | Test assertions (`require` preferred over `assert`) |

## Conventions

- **Error handling:** Use `github.com/eluv-io/errors-go` for wrapping errors with context keys; no panics in library code.
- **Logging:** Initialize a package-level logger per module (e.g., `log.Get("/eluvio/media/transport/rtp")`).
- **Immutability:** Format types (QID, etc.) are immutable; use factory methods for creation and parsing.
- **Interfaces:** Prefer interface-driven design for decoupling (see `media/pacer` for examples).
- **Benchmarks:** Performance-critical paths in `media/` have corresponding `*_bench_test.go` files.