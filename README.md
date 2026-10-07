# AisleFlow

**Compile warehouse processes. Observe movement. Adapt the next task.**

A Go portfolio project combining a strict BPMN-to-Temporal compiler with an OTLP congestion analyzer. Built as an independent demonstration for a senior Go engineering interview; it is not an official Pyck integration.

```mermaid
flowchart LR
    BPMN[BPMN XML] --> Compiler[bpmn2go]
    Compiler --> Workflow[Generated Go workflow]
    Compiler --> Schema[Generated Ent task schema]
    Workflow --> Activities[Movement activities]
    Activities --> Collector[OTel Collector]
    Collector --> Jaeger[Jaeger]
    Collector --> Analyzer[Authenticated OTLP analyzer]
    Analyzer --> DB[(PostgreSQL: observations + outbox)]
    DB --> Dispatcher[Retrying dispatcher]
    Dispatcher -->|Temporal signal| Workflow
    Schema --> DB
```

## Start here

For a visual walkthrough, open **[the Pyck functionality animation](docs/animation/index.html)** in a browser. The interactive 54-second demo illustrates compilation, movement telemetry, congestion detection, and the approved aisle reroute. It runs offline without the backend. See [animation notes](docs/animation/README.md) for controls and scope.

Read **[TECHNICAL_GUIDE.md](TECHNICAL_GUIDE.md)** for the architecture, code walkthrough, formulas, reliability guarantees, tradeoffs, demonstration script, and interview questions.

Go 1.26+ is required. Generated source is included, so generation is not needed just to run the tests.

```sh
go mod download
go test ./...
go test -v -count=1 ./tests/closedloop
```

The closed-loop test uses the production movement activity and OpenTelemetry SDK exporter, real OTLP protobufs over gRPC, the actual analyzer, and the generated workflow running in Temporal's SDK test environment. It needs no Docker, PostgreSQL, or Temporal server. Its repository is in memory.

Expected route: `A01 A01 A01 A01 A02 A02 A02 A02 EXPRESS`, nine completed tasks, one accepted alert. Every movement is exported twice to verify deduplication.

## Full local stack

Requires Docker Engine/Desktop with Compose v2. Task v3 is optional.

```sh
docker compose up -d --build --wait
docker compose run --rm demo
docker compose down
```

Equivalent Task commands: `task up`, `task demo`, `task down`. Postgres and Temporal volumes persist across `down`.

| Surface | Address |
|---|---|
| Temporal workflow history | http://localhost:8233 |
| Jaeger, service `aisleflow-worker` | http://localhost:16686 |
| Analyzer readiness | http://localhost:8080/readyz |
| Collector OTLP/gRPC | localhost:14317 |
| PostgreSQL | localhost:5432 |

Live timing can cause the reroute to take effect one or more tasks later than in the controlled test. The current task always finishes first. The demo command fails if no reroute was observed.

## Development

```sh
go run ./backend/workflowgen/cmd/bpmn2go
go generate ./backend/analytics/ent
go run ./backend/workflowgen/cmd/bpmn2go --check
go vet ./...
go test -race -count=1 ./...
go build ./...
```

For PostgreSQL integration tests after starting the stack:

```powershell
$env:TEST_DATABASE_URL='postgres://aisleflow_app:aisleflow_demo@localhost:5432/aisleflow?sslmode=disable'
go test -tags=integration -count=1 ./tests/integration
```

On a POSIX shell, prefix the test command with `TEST_DATABASE_URL='postgres://...'` instead. `task test:integration` sets this variable for you.

## What is implemented

- Namespace-aware BPMN parser, bounded input, graph validation, exclusive branches, deterministic Go templates, and stale-output checking.
- Generated typed activity contract, executable Temporal workflow, and Ent completion-receipt schema.
- Authenticated OTLP/gRPC ingestion, tenant/run correlation, timestamp validation, retry deduplication, and rolling-window detection.
- Ent/PostgreSQL persistence, forced row-level security, transactional alert outbox, delivery backoff, and dead-letter state.
- Run-specific Temporal signals, duplicate suppression, and routing through explicitly approved alternatives.
- Docker Compose, Taskfile, CI configuration, unit tests, a closed-loop test, and database integration tests.

## Boundaries

This is a focused engineering demo. Movement durations are synthetic; the baseline is fixed configuration; detector windows are in memory; one analyzer instance owns them. It supports a documented BPMN subset, not arbitrary BPMN execution. There is no real scanner, hardware, Pyck tenant, NATS integration, GraphQL API, or inventory mutation.

Telemetry export is best effort before database ingestion. After an observation and alert commit, the outbox is durable and delivery is at least once. Routing advice is advisory, not a physical safety system. See the guide for production changes and verification status.

Local credentials and insecure transports are for the loopback-bound development stack only.
