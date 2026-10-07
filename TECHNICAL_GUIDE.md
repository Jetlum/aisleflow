# AisleFlow — technical guide and interview preparation

## 1. What you built

AisleFlow combines the two proposed projects into one feedback loop:

1. A compiler reads a warehouse process expressed as a constrained BPMN diagram.
2. It generates executable Go orchestration and an Ent schema for task receipts.
3. A Temporal worker executes the generated process.
4. Each simulated movement emits a named OpenTelemetry span.
5. An OTLP/gRPC service validates that movement and compares recent durations to a baseline.
6. It atomically persists the movement and any resulting congestion alert.
7. A dispatcher sends that alert to the specific Temporal workflow run.
8. Before its next activity, the workflow can choose a pre-approved alternate aisle.

The demonstration is about reliable process adaptation. It does not update stock, control machinery, compute a shortest path, or claim a measured warehouse productivity gain.

### Your 45-second introduction

> “I built a Go prototype that connects process authoring to operational feedback. A restricted BPMN compiler generates a Temporal workflow and its Ent task schema. The workflow emits movement telemetry, and an analyzer detects slow aisle traversal. Alerts are committed through a PostgreSQL outbox and delivered as run-specific Temporal signals. The workflow applies an approved alternate route between tasks. The interesting parts are the contracts: unsupported BPMN fails compilation, tenant isolation is enforced in the receiver and database, duplicate telemetry does not distort detection, and retries do not require canceling the workflow.”

Say **“I built and studied this prototype with AI assistance”** if that reflects how you present your work. Be able to change it, explain it, and defend its limits. Do not claim production experience from running a demonstration.

### Why this is relevant to Pyck

Pyck's public repository describes Go, Ent/PostgreSQL, Temporal, OTel/Jaeger, and a `backend/workflowgen` area. That makes these interfaces relevant discussion material. This implementation is independent and does not assume access to Pyck Studio or their internal workflow contracts. See the [public repository](https://github.com/pyck-ai/pyck-oss).

The original notes contained funding, award, leadership, and hiring claims with broken citation markers. Those are not dependencies of this implementation and have not been verified here. Avoid repeating them as established facts in the interview.

## 2. Read the code in this order

| File or directory | What to learn |
|---|---|
| `examples/picking.bpmn` | Source process, explicit routing alternatives, exclusive branch |
| `backend/common/contracts/contracts.go` | Shared input, movement, result, signal, stable identity |
| `backend/workflowgen/core/parser.go` | XML parsing and graph invariants |
| `backend/workflowgen/core/generator.go` | Embedded templates and Go source validation |
| `backend/workflowgen/core/templates/` | What code generation actually produces |
| `backend/temporal/generated/workflow_gen.go` | Generated branch resolution and workflow entry point |
| `backend/temporal/runtime/workflow.go` | Deterministic orchestration and signal handling |
| `backend/temporal/activities/movement.go` | Activity side effects, receipts, synthetic spans |
| `backend/analytics/transport/otlp.go` | Authentication, OTLP decoding, rejection/retry semantics |
| `backend/analytics/core/analyzer.go` | Detection state machine and dispatcher |
| `backend/analytics/store/postgres.go` | Tenant transaction, Ent operations, outbox durability |
| `backend/analytics/ent/schema/` | Handwritten schemas plus generated task schema |
| `cmd/migrate/main.go` | Local schema bootstrap and RLS policy installation |
| `tests/closedloop/closedloop_test.go` | Executable explanation of the entire feedback loop |

`backend/analytics/ent/gen` contains generated ORM code. Understand the schema and repository before reading generated builders. One root Go module keeps the demo easy to build; its directory layout resembles service boundaries without requiring a multi-module workspace.

## 3. Process model and compiler

### Supported language

The XML root must be BPMN `definitions` in `http://www.omg.org/spec/BPMN/20100524/MODEL`, containing exactly one process explicitly marked `isExecutable="true"`. Prefix names such as `bpmn:` are arbitrary; namespace URIs determine meaning.

Supported flow nodes are:

| Element | Semantics |
|---|---|
| `startEvent` | Exactly one; no incoming flow, one outgoing flow |
| `serviceTask` | One movement; incoming flow(s), one outgoing flow |
| `exclusiveGateway` | Select exactly one outgoing edge using an input variable |
| `endEvent` | Finish the selected route; incoming flow(s), no outgoing flow |
| `sequenceFlow` | Reference existing source and target IDs |

The AisleFlow extension namespace is `https://aisleflow.dev/bpmn/v1`. It defines:

```xml
<bpmn:serviceTask id="pick_01" af:aisle="A01" af:alternative="A02"/>
<bpmn:exclusiveGateway id="dispatch" af:variable="service"/>
<bpmn:sequenceFlow id="express_edge" sourceRef="dispatch"
                   targetRef="pack_express" af:value="express"/>
```

The task specifies its normal aisle and an optional approved alternative. A gateway reads `input.Variables["service"]`; an edge matches a literal value. There is no expression evaluator, code injection, default edge, dynamic variable mutation, or condition language. Missing/unknown branch values fail route resolution.

BPMN in general is **not** a DAG. This compiler deliberately supports only acyclic processes. Cycles, user tasks, parallel gateways, inclusive gateways, event definitions, compensation, timers, subprocesses, and arbitrary execution extensions are rejected. Documentation and a top-level BPMN diagram interchange element are ignored because they do not change execution in this subset.

### Parsing and validation

`encoding/xml.Decoder` processes tokens. Input is capped at 1 MiB; the graph is capped at 500 nodes. DOCTYPE/directives, unexpected executable text, foreign executable elements, duplicate supported attributes, and unknown executable attributes fail.

IDs use a deliberately restrictive 1–64 character ASCII identifier format: letters, digits, underscore, hyphen. This is a subset of XML IDs. Process, node, and flow IDs must be distinct. A source or target referring to an unknown node fails validation.

The validator checks node degrees and requires unique, nonempty branch values. A three-color DFS detects back edges and verifies that every node is reachable from the start. With acyclicity and the degree rules, every path eventually terminates at an end event. Implicit merges are safe here because execution follows a single exclusive path; there is no parallel token semantics to synchronize.

Graph validation is O(V + E), excluding XML parsing and sorting. Nodes and edges are sorted before generation, giving stable output even when element order changes. The node bound also bounds recursion depth.

### Generation pipeline

```text
XML -> validated Process/Node/Flow structures -> templates -> go/format -> files
```

The IR is a small graph representation, not Go's `go/ast` tree and not a complete BPMN metamodel. `text/template` emits source; `go/format` verifies Go syntax and formats it. Ordinary `go build` and tests provide the type check. Both generated artifacts are rendered before either is written. Each file uses a sibling temporary file followed by rename; the two files are not an atomic pair. CI's generated-output check detects mismatches.

The compiler emits:

1. A Go file with `ProcessID`, the typed `Activities` interface, `Resolve`, and `PickingWorkflow`.
2. `ProcessTask`, an Ent schema with task IDs restricted to an enum generated from the diagram and a compound receipt uniqueness index.

`Resolve` follows a generated Go switch, building an ordered `[]contracts.Step` for the selected path. A hop bound is a defensive assertion; valid input already has no cycles. The workflow wrapper passes those steps to the handwritten runtime.

The generator currently targets this repository's module and fixed `ProcessTask`/`PickingWorkflow` names. It compiles one process into one output location. A general public CLI should accept a target module and names, support multiple independently generated processes, and publish a versioned extension schema.

### Correct generation order

```sh
go run ./backend/workflowgen/cmd/bpmn2go
go generate ./backend/analytics/ent
```

The BPMN compiler must run first because its output is input to Ent generation. `--check` renders without writing and fails if a committed compiler artifact is stale. The CI workflow also regenerates Ent and checks the diff.

## 4. Temporal workflow design

Temporal persists workflow execution history. The worker re-executes workflow code against that history, so workflow code must produce the same commands when replayed. This repository keeps network calls, database access, wall-clock timestamps, random IDs, and tracing in activities or service processes.

Workflow IDs follow `picking/{tenant}/{wave}`. Input validation and runtime verification prevent a caller from accidentally running one tenant's input under a differently named workflow. This naming is defense in depth, not a substitute for Temporal authorization.

The runtime uses:

| Setting | Value | Purpose |
|---|---|---|
| Task queue | `aisleflow-picking` | Worker registration/routing |
| Activity name | `aisleflow.move.v1` | Stable, versioned activity contract |
| Start-to-close | 30 seconds | Bound a single activity attempt |
| Schedule-to-close | 2 minutes | Bound time across queueing and retries |
| Retry attempts | 3 | Bound failures in the demo |
| Initial retry interval | 1 second | Avoid immediate retry loops |
| Signal name | `aisleflow.reroute.v1` | Versioned control message |

An activity duration of 30 synthetic seconds still consumes about one actual second: the worker simulates a past completed movement and sets span timestamps explicitly. That is why the demo finishes quickly. A real scanner would report a real direction/confirmation interval.

### Signal boundary

Before each activity, `ReceiveAsync` drains buffered signals. A signal must match the current tenant, site, workflow ID, and **run ID**, have an alert ID, and refer to an aisle in the resolved route. The workflow keeps an in-memory `seen` map during execution; replay reconstructs it from history. Duplicate alert IDs are ignored.

Accepted signals mark the reported aisle blocked for the remainder of the run. The next affected task can use its declared alternative. If there is no alternative or that alternative is also blocked, execution returns a non-retryable `RouteUnavailable` application error.

This is intentionally conservative: no automatic unblocking, route oscillation, mid-activity cancellation, arbitrary aisle supplied by the analyzer, or resumption after `RouteUnavailable`. Operator recovery is a future feature. The analyzer suggests a blocked aisle; the process definition owns where a picker may go instead.

A signal arriving while an activity is executing is buffered and considered before the following activity. A signal after the final boundary may be recorded by Temporal without affecting any task. “Signal delivered” therefore means accepted by Temporal, not necessarily applied by workflow logic. `Result.AcceptedAlerts` supplies the latter evidence when the workflow completes.

### Retry and physical-world limits

Temporal activity execution is at least once. The generated task receipt has a unique `(tenant_id, workflow_id, run_id, task_id)` key. The OTLP observation has the same logical identity hashed into `event_id`. Those prevent duplicate records and duplicate statistical samples.

They do **not** make physical picking exactly once. A real hardware/scanner command needs a device-level idempotency token or an explicit acknowledgement/reconciliation protocol. The simulator writes a receipt after its delay, then exports telemetry; retries may repeat its simulated work but do not create another logical observation.

Changing generated workflow code while old runs are open can break replay. For production, version workflow deployments, replay saved histories in CI, use Temporal patch/versioning mechanisms where necessary, and retain workers capable of completing old histories. The checked-in tests exercise orchestration but do not replay a captured production history.

## 5. Telemetry contract and trust boundary

Only spans named `warehouse.movement` are analyzed. Unrelated spans are ignored and can still appear in Jaeger. All identity attributes below are string-valued **span attributes**, not resource attributes.

| Attribute | Example | Validation |
|---|---|---|
| `tenant.id` | `demo` | Must equal tenant authenticated by API key |
| `warehouse.site` | `hall-1` | Restricted identifier |
| `warehouse.aisle` | `A01` | Restricted identifier; configured baseline required |
| `warehouse.task` | `pick_04` | Restricted logical task ID |
| `warehouse.wave` | `interview` | Restricted identifier |
| `temporal.workflow_id` | `picking/demo/interview` | Must equal reconstructed tenant/wave ID |
| `temporal.run_id` | Temporal run ID | Restricted nonempty identifier |
| `demo.synthetic` | boolean `true` | Simulator annotation; not used by detector |

The receiver gets `x-api-key` from gRPC metadata and compares it against `TELEMETRY_TOKENS`, a JSON map from token to tenant. It requires exactly one metadata value. Configuration rejects short tokens and invalid tenant IDs. A span claiming another tenant is rejected even if its other fields are plausible.

The Compose collector supplies a single demo tenant credential. It is a trusted ingress for this local demo. A multi-tenant installation must authenticate producers before routing to tenant-specific exporters or use mTLS/OIDC identity propagation. Merely accepting a client-supplied `tenant.id` would be insufficient.

Trace IDs must be 16 nonzero bytes; span IDs must be 8 nonzero bytes. Required duplicate keys, incorrectly typed attributes, explicitly failed spans, zero/negative durations, durations over an hour, integer-overflow timestamps, observations over five minutes old, and observations over one minute in the future fail. Status `UNSET` is accepted because successful OTel spans commonly omit an explicit OK status.

The service allows at most 1,000 spans per export and configures a 4 MiB gRPC receive limit. Thirty-two concurrent streams are allowed per connection; this is not a global client rate limit. Production should add ingress quotas and authentication before expensive work.

### Responses and backpressure

- Invalid credentials: gRPC `Unauthenticated`.
- Oversized span batch: gRPC `ResourceExhausted`.
- Invalid movement: OTLP partial success with a rejected-span count.
- Database failure or exhausted detector capacity: gRPC `Unavailable`, allowing retry.

An export is not one database transaction. Valid movements before a later failure can have committed; a retried request safely deduplicates them. If all recognized movements succeed or are individually rejected, the RPC completes. Invalid samples do not poison healthy samples in a batch.

### Span time is not queue time

The intended measurement is `confirmation_time - direction_issued_time`, measured on one coherent clock. A production implementation must distinguish walking, task queueing, picking, waiting for equipment, and human confirmation delay. Cross-device subtraction requires clock synchronization or a domain timing protocol; distributed trace parent-child relationships alone do not fix clock skew.

The demo creates one movement span covering this interval. It does not try to reconstruct it from two unrelated spans. OTel Collector fans the same spans to Jaeger and the analyzer; Jaeger is a visualization sink, not the analytics query engine.

## 6. Detection mathematics and state

Configuration contains fixed `(tenant, site, aisle)` baselines with mean μ and standard deviation σ. Both must be positive finite numbers. Current sample windows are keyed by `(tenant, site, aisle, workflow ID, run ID)`. Keeping runs separate avoids mixing a finished wave into a new wave and prevents one wave's outlier from directly rerouting another wave.

With the default window of three observations:

```text
movement duration d = (span end - span start) / 1e9 seconds
rolling mean m = (d1 + d2 + d3) / 3
threshold T = μ + 2.5 × σ
alert if m >= T and this window is outside its cooldown
```

For μ = 10 s and σ = 2 s, T = 15 s. Demo values for A01 are:

| Task | Duration | Window | Mean | Decision |
|---|---:|---|---:|---|
| pick_01 | 9 s | [9] | — | Warmup |
| pick_02 | 10 s | [9, 10] | — | Warmup |
| pick_03 | 11 s | [9, 10, 11] | 10 s | Healthy |
| pick_04 | 30 s | [10, 11, 30] | 17 s | Congestion alert |
| following tasks on A02 | 10 s | Separate A02 window | 10 s once full | Healthy |

This is a **heuristic**, not a statistical significance test. It compares a sample-window mean against a threshold based on individual-duration standard deviation; it does not substitute σ/√n or assume independent Gaussian samples. Real walking times are often skewed and dependent. Route length, equipment type, shift, and task class can confound an aisle baseline. Empirical percentiles, robust median/MAD, change-point detection, or an EWMA with measured false-positive rates are sensible next steps.

There is a one-minute cooldown per window, a ten-minute idle TTL, and a maximum of 10,000 active windows in the service configuration. A zero-variance or missing baseline is rejected, avoiding the original sketch's accidental zero threshold. Configuration is copied into the analyzer, preventing external map mutation.

The detector is serialized under a mutex. It creates a candidate window, attempts the database transaction, and updates live state **only after a successful new commit**. A duplicate event or database failure leaves its sample count unchanged. This provides a clear correctness model for one instance at modest throughput; it is not a high-throughput benchmark result.

Each observation scans windows for TTL eviction, so processing includes O(K) expiry work and O(W) window work, plus a database round trip held under the mutex. K is bounded but 10,000 is not a performance target. For scale, use sharded ownership, per-partition ordered processing, efficient expiration, and measured capacity planning.

Windows and cooldowns reset on process restart. Persisted events remain deduplicated, and pending alerts survive, but the detector needs new samples to warm up. A crash after commit and before the in-memory update can omit that sample from the current window. It will not duplicate the sample or lose a committed alert. Production can persist detector state or rebuild windows from ordered observations and a checkpoint.

The window uses arrival order, not event-time sorting. Late-but-valid observations can affect the last-three window. Strong event-time semantics need a watermark and bounded reordering policy.

## 7. Ent and PostgreSQL

### Schemas

`IdentityMixin` adds UUID v7 `id`, immutable `tenant_id`, and immutable `created_at`. Ent's schema method is `Mixin()`; using `Mixins()` would not install a mixin. `uuid.NewV7` returns `(UUID, error)`, so the default uses `func() uuid.UUID { return uuid.Must(uuid.NewV7()) }`. Ent expects a zero-argument function returning a UUID. This conventional default panics on cryptographic randomness failure; explicitly allocating IDs in a service layer would allow returning that error instead.

| Entity | Role | Uniqueness / indexes |
|---|---|---|
| Observation | Immutable accepted movement | Unique tenant/event; tenant/site/aisle/time lookup |
| Alert | Auditable decision plus outbox delivery state | Unique tenant/alert; pending-delivery lookup |
| ProcessTask | Generated completion receipt | Unique tenant/workflow/run/task |

Observation stores the logical event key, site, aisle, workflow/run/task, seconds, and observation time. Alert stores the target identity, mean and threshold at decision time, attempts, next attempt time, delivered time, dead flag, and last error. ProcessTask stores the actual chosen aisle and a generated enum of legal task IDs.

UUID v7 improves index locality relative to randomly distributed IDs. It is neither authorization nor a guarantee that commit order equals ID order. Event hashes provide idempotency; UUIDs provide row identity.

Observations and task receipts are append-only for the runtime role. Alerts allow only delivery-state column updates at the database grant level. This intentionally does not copy soft-delete behavior from mutable business entities: immutable telemetry/audit data needs retention and archival policy, not hidden historical mutations. There is no retention job in the demo.

### Tenant isolation

Every repository call begins a SQL transaction and uses parameterized:

```sql
SELECT set_config('app.tenant_id', $1, true);
```

`true` scopes the setting to that transaction. The Ent driver is bound to that exact `*sql.Tx`, so another pooled connection cannot lose the setting. A small driver adapter keeps nested Ent transaction requests within the caller-owned transaction; only `withTenant` commits or rolls back. No returned entities are used for later lazy queries outside this scope.

The migration enables and **forces** RLS on each table and installs both `USING` and `WITH CHECK`:

```sql
tenant_id = current_setting('app.tenant_id', true)
```

Queries include explicit tenant predicates as well. Runtime credentials are rejected if their role is a superuser or has `BYPASSRLS`. Schema bootstrap uses separate administrative credentials. The database integration test intentionally omits a tenant predicate and tries a foreign-tenant insertion to test database enforcement.

RLS is defense against application mistakes, not against a fully compromised runtime process that can issue its own tenant-setting SQL. Tenant identity must first be authenticated and propagated correctly. The SQL adapter remains an internal boundary.

### Atomic ingestion

`Postgres.Commit` locks a transaction-scoped advisory key derived from tenant/event, checks for the event, inserts the observation, optionally inserts its alert, and commits. Advisory locking prevents concurrent duplicates from racing between existence check and insert; a unique index remains the final constraint. Hash collisions cause unnecessary serialization, not accidental acceptance of a different event.

If alert insertion fails, the observation rolls back too. A successful duplicate returns `inserted=false`. This lets the detector distinguish “already counted” from “new observation”. Receipts follow a similar transaction-scoped lock and unique key.

### Transactional outbox

```mermaid
sequenceDiagram
    participant A as Analyzer
    participant DB as PostgreSQL
    participant D as Dispatcher
    participant T as Temporal
    A->>DB: Begin; tenant setting; dedup lock
    A->>DB: Insert observation + optional alert
    A->>DB: Commit
    D->>DB: Read pending alerts
    D->>T: Signal(workflow ID, run ID, alert ID)
    T-->>D: Acknowledgement
    D->>DB: Mark delivered
```

The dispatcher polls every 200 ms, reads at most 100 eligible alerts per configured tenant, and sends signals with an explicit run ID. A reused workflow ID cannot accidentally receive a delayed old-run alert.

Failures record attempt count, last error, and exponential retry time: 2, 4, 8, 16, 32, then up to 64 seconds. Eight failed attempts mark an item dead. A crash after Temporal acknowledges but before the delivered mark causes redelivery; workflow alert-ID deduplication makes that safe within the run. This is at-least-once delivery, not a distributed exactly-once transaction.

One dispatcher is intended. Multiple dispatchers could send the same row concurrently because there is no claim/lease or `SKIP LOCKED` mechanism. The workflow still deduplicates, but production should add claiming, ownership and metrics. Completed/missing workflows currently exhaust the bounded retry policy; a production adapter should classify permanent Temporal errors immediately.

## 8. Failure matrix

| Failure | Current behavior | Remaining limit |
|---|---|---|
| Malformed/unsupported BPMN | Compiler error before output writes | No general BPMN execution |
| Unknown exclusive branch | Workflow fails before movement activities | No default branch |
| Duplicate OTLP export or activity retry | Same event key; no extra sample | One logical completion per task per run |
| Wrong tenant in span | Partial rejection | Credential holder can forge its own tenant's telemetry |
| Wrong run/site in signal | Workflow ignores it | Temporal client authorization still required |
| PostgreSQL down during ingestion | Retryable RPC failure; window unchanged | Collector retry budget and queue are finite |
| Temporal down after DB commit | Alert remains pending and retries | Dead-letter handling needs operator process |
| Dispatcher crash after send | Possible duplicate signal | Idempotency is scoped to workflow run |
| Analyzer restart | Durable outbox survives; windows warm up again | Cooldown and samples are not durable |
| Alternate also congested | Workflow fails `RouteUnavailable` | No operator resume flow yet |
| SDK/collector exits before export | Span can be lost | No transactional source telemetry outbox |
| Reroute arrives after final task | No useful routing change | Delivery acknowledgement is not application acknowledgement |

The OTel SDK and collector use memory queues. After database ingestion the outbox is durable; **before ingestion telemetry is best effort**. Do not sell this architecture as safety-critical control. A production control loop can consume durable domain movement events, with OTel traces used for diagnosis and correlation. An optional NATS JetStream ingress would fit there, but adding an unused broker to the demo would not provide that guarantee.

## 9. Run and present the demo

### Service configuration

| Variable | Consumer | Meaning/default |
|---|---|---|
| `DATABASE_URL` | Worker and analyzer | Required non-superuser PostgreSQL connection |
| `MIGRATION_DATABASE_URL` | Migration command | Required administrative schema connection |
| `TEMPORAL_ADDRESS` | Worker, analyzer, demo | `localhost:7233` outside Compose |
| `TELEMETRY_TOKENS` | Analyzer | Required JSON token-to-tenant map |
| `BASELINES_FILE` | Analyzer | `config/baselines.json` |
| `OTLP_LISTEN` | Analyzer | `:4317` inside service |
| `HTTP_LISTEN` | Analyzer | `:8080` for `/healthz` and `/readyz` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | Worker | `localhost:14317`; this code expects host:port, without a URL scheme |
| `TEST_DATABASE_URL` | Tagged integration test | Runtime role connection; required when running the test |

Compose supplies internal service hostnames. `/healthz` only reports process liveness; `/readyz` checks database connectivity and Temporal health. The analyzer handles shutdown signals, stops its polling loop, drains gRPC with a bounded wait, and shuts down HTTP. The worker flushes its OTel provider during shutdown. `task lint` runs `go vet`; `task test` uses a version-pinned gotestsum invocation.

### Without infrastructure

```sh
go test -v -count=1 ./tests/closedloop
```

This uses the production movement activity, the OTel SDK exporter with a synchronous span processor, a real in-process gRPC server and client connected with `bufconn`, the real detector/dispatcher, and Temporal's SDK test environment. An exporter wrapper sends each span twice. It substitutes an in-memory repository and a test signal adapter for PostgreSQL and Temporal Server; activity database receipts are omitted. It is a strong component integration test, not a proof of live external-service behavior.

### With Docker

```sh
docker compose up -d --build --wait
docker compose run --rm demo
```

Open Temporal at `http://localhost:8233`, Jaeger at `http://localhost:16686`, and readiness at `http://localhost:8080/readyz`. In Jaeger, select service `aisleflow-worker` and inspect `warehouse.movement` attributes. In Temporal, inspect the printed workflow ID, activity sequence, received signal, and final JSON result.

The live loop is asynchronous: telemetry batching and outbox polling may let another activity start on A01 before the signal arrives. Explain that this is intentional safe-boundary behavior, not nondeterministic Temporal replay.

The worker writes receipts, exports traces with a 100 ms batch timeout, and sleeps one second per simulated task. The collector batches for up to 100 ms and sends the data to both consumers. The demo uses an express packing branch. Changing its input variable to `standard` selects `pack_standard`/`PACK` instead.

### Five-minute presentation

1. **0:00–0:45, outcome:** Give the short introduction and identify simulated behavior.
2. **0:45–1:30, authoring:** Show the BPMN task alternative and gateway. Run the compiler and open the generated workflow.
3. **1:30–2:30, execution:** Run the demo. Show that a workflow continues and later tasks choose A02.
4. **2:30–3:30, reliability:** Show the observation/outbox transaction and tenant RLS policy. Explain the crash-after-send case.
5. **3:30–4:30, evidence:** Run the closed-loop test. Point out duplicate exports and the single accepted alert.
6. **4:30–5:00, judgment:** State the BPMN subset, synthetic baseline, single analyzer, and best-effort telemetry limits. Ask which boundary matters most in their actual architecture.

### Troubleshooting

| Symptom | Check |
|---|---|
| `bpmn2go --check` fails | Regenerate compiler output, then Ent; do not hand-edit generated code |
| Ent reports missing types with a newer Go | Keep the pinned `golang.org/x/tools` dependency; use `go mod download` |
| Worker cannot connect | Wait for Temporal health; confirm `TEMPORAL_ADDRESS` |
| No accepted alert | Check `docker compose logs analyzer otel-collector worker`; verify baseline and token |
| Database permission/RLS failure | Run migration; use runtime credentials for services, admin only for migrate |
| Database role missing after config changes | Init scripts only run on a new PostgreSQL volume; existing data requires deliberate migration |
| `-race` needs a compiler on Windows | Install/configure a compatible C compiler or run Linux CI |
| Windows blocks a generated executable | Use an approved execution environment; do not disable application control |

`docker compose down` preserves data volumes. Do not remove them casually if they contain evidence you want to show. Services have local demo passwords and insecure internal transports; host-published ports bind to loopback. The dev Temporal server uses a persisted SQLite file and is not a production Temporal deployment. Jaeger uses transient memory storage.

## 10. Tests and what they prove

| Suite | Evidence |
|---|---|
| Compiler unit tests | Stable committed output; rejects cycles, unsupported execution, bad topology, duplicate IDs, oversized XML |
| Parser fuzz target | Arbitrary input must not panic; every accepted graph revalidates |
| Analyzer tests | Warmup, exact threshold, deduplication, cooldown, tenant isolation, bad baselines/samples, persistence failure |
| Concurrent ingestion test | Duplicate event delivered concurrently is counted once |
| OTLP tests | Credential handling, malformed/forged spans, retryable persistence errors, logical dedup identity |
| Workflow tests | Run/site/tenant validation, duplicate signal suppression, safe alternate, no-route failure |
| Closed-loop test | Generated workflow through real OTLP gRPC and back through a signal |
| PostgreSQL integration test | Actual RLS read/write isolation, atomic rollback, dedup, durable pending alert after reopening |

Run `go test -race -count=1 ./...`, `go vet ./...`, and `go build ./...`. Integration tests require `TEST_DATABASE_URL` and `-tags=integration`. CI defines separate Go and Compose jobs. A CI file is not evidence that hosted CI has already run.

See [docs/VERIFICATION.md](docs/VERIFICATION.md) for the exact checks executed in the implementation environment and remaining external checks. Do not describe an unrun Docker/PostgreSQL suite as passing.

## 11. Questions you are likely to get

### Why combine these projects?

The compiler demonstrates developer tooling, while the analyzer demonstrates an operational benefit. Their shared contracts make the combination more meaningful than two unrelated utilities: a process defines permissible actions and runtime evidence influences the next permitted choice.

### Why generate code instead of interpreting BPMN?

Generated code is reviewable, type-checked, testable, and deployable with the Go worker. An interpreter makes dynamic process updates easier but carries BPMN execution semantics and versioning into runtime. This prototype chooses a small statically deployed language and makes that constraint explicit.

### Is this compatible with arbitrary Pyck Studio output?

No. The extension namespace and subset are defined here. A proper adapter requires verified sample diagrams, supported element mappings, execution semantics, versioning rules, and tenant authorization contracts from Pyck. The code demonstrates a possible boundary, not plug-and-play integration.

### Why reject cycles and parallel gateways?

Cycles require iteration identity, bounded history/Continue-As-New, and loop termination rules. Parallel gateways require fork/join token semantics, failure propagation, and deterministic ordering of emitted commands. Silently flattening either would be wrong. The compiler rejects them until the runtime and tests support them explicitly.

### Why is the branch resolved before workflow execution?

The current input variables are immutable, and no activity output changes a branch. Pre-resolving simplifies deterministic execution. Supporting data-dependent decisions would require a typed activity-result model and explicit gateway evaluation points in the runtime.

### What makes the workflow deterministic?

It processes an ordered generated step list, reads recorded input and signals, and schedules activities through Temporal APIs. It does not use ordinary timers, random numbers, I/O, or map iteration to decide command order. Signal order and activity completion are replayed from history.

### Are signals reliable and exactly once?

The database outbox retries them; Temporal acknowledgement and the database mark cannot be atomic. Delivery is at least once. The workflow uses `AlertID` to deduplicate and `RunID` to avoid targeting a new incarnation. Application of the advice is visible in the workflow result, not inferred from RPC success.

### Why not cancel the ongoing activity?

A walking or machinery action may not be safely reversible. This demo finishes the current action and only changes a future one. Real cancellation would require hardware acknowledgement, a safe stop protocol, and recovery semantics, not just canceling a Go context.

### Why include the run ID?

A workflow ID can be reused after completion. A delayed outbox message for an old execution must not reroute the new execution. Explicit run targeting closes that ambiguity.

### How does multi-tenancy work?

A credential resolves to tenant identity, the receiver verifies span claims, workflow IDs include that tenant, signals are checked again, repository predicates scope queries, and PostgreSQL RLS enforces row access. An immutable `tenant_id` column alone would not enforce isolation.

### Why a UUID v7 and a separate event hash?

They solve different problems. UUID v7 is a row identifier with useful index locality. The stable hash represents logical movement identity across retries. A new UUID for each retry cannot deduplicate an operation.

### Can the detector confuse tenants sharing aisle A01?

The baseline key includes tenant and site, and the window additionally includes workflow and run. The tests deliberately use tenants with the same aisle but different baselines. PostgreSQL isolation separately protects persistence.

### Does 2.5σ imply a known false-alarm probability?

Not here. The distribution and independence assumptions are unverified, and the statistic is a rolling mean compared with an individual-duration threshold. It is a transparent heuristic for the demo. Calibrate with labeled operational data and measure precision, recall, and control-loop effects.

### Could delayed confirmation look like congestion?

Yes. The duration measures an observed interval, not causality. Separate physical movement, queueing, picking, device latency, and confirmation delay, and enrich the baseline with route/task context before attributing delays to aisle congestion.

### Can sampling hide congestion?

Yes. The demo samples all movement spans. Ordinary tail/head sampling could bias the detector. A production decision stream should be unsampled, explicitly reliable domain events or a dedicated telemetry pipeline with measured loss.

### Why no NATS or GraphQL service?

Neither is needed to prove this feedback loop. OTLP is the ingestion contract and Temporal is the control contract. NATS would be useful for a durable movement event stream, and GraphQL for browsing decisions or operator review, but an unused component would add setup without improving the current behavior.

### Can it scale horizontally?

Not by simply increasing analyzer replicas: windows live in process memory. Partition by tenant/site/run with exclusive ownership, or externalize ordered state. Add outbox claiming and concurrent delivery separately. First measure mutex contention, database latency, cardinality, and message throughput.

### What happens after restart?

The database retains observations, receipts, and undelivered alerts. A fresh analyzer warms up windows and cooldowns from new events. It does not automatically reconstruct past windows. Persisted dedup still prevents repeated old exports from becoming new samples.

### Can telemetry trigger an unauthorized physical action?

The workflow only considers approved routes from its process definition, but a holder of valid tenant credentials could send misleading evidence. Production should authorize producers to specific workflow/device identities, validate movement provenance, use operator policy where appropriate, and keep physical interlocks outside this software loop.

### What would you improve first?

First verify the target workflow and scanner contracts. Then separate reliable domain movement events from observability export, persist detector state with ordered processing, add real-data baseline calibration and alert/application metrics, harden deployment identity/TLS, and test replay compatibility against saved histories. Expand BPMN only when a real process needs additional semantics.

### How would you prove business value?

Start in shadow mode: record proposed reroutes without executing them. Compare against labeled congestion and track false positives, picker travel time, order SLA, distance, and downstream congestion. Then run a controlled rollout with approved alternatives and operational oversight. The current synthetic demo provides no ROI estimate.

## 12. Preparation exercises

1. Explain why one 30-second movement triggers at pick_04 when the previous two values were 10 and 11, and why the first two movements cannot trigger.
2. Change `service` to `standard`; predict the last task, then run it.
3. Remove A01's alternative from a task and send a valid congestion signal before it; explain the failure instead of promising an automatic resume.
4. Change the sample diagram to a parallel gateway; read the compiler error and explain which semantics are missing.
5. Send a `tenant.id` that differs from its token's tenant; identify the rejecting boundary.
6. Find the exact point where an outbox redelivery can occur and explain why the workflow accepts it only once.
7. Trace a task ID from XML to generated Ent enum to activity receipt to observation dedup key.
8. Explain what is and is not preserved across an analyzer restart.
9. Discuss how to support loops without treating each repeated activity as a duplicate; introduce explicit iteration identity.
10. Name one measurement that would falsify the claim that a slow span represents aisle congestion.

## 13. Primary references

- [Pyck public repository](https://github.com/pyck-ai/pyck-oss): public stack and directory conventions; no private integration claims.
- [Temporal Go message passing](https://docs.temporal.io/develop/go/workflows/message-passing): signals and workflow message handling.
- [Temporal CLI](https://github.com/temporalio/cli): local development server and Docker usage.
- [Ent mixins](https://entgo.io/docs/schema-mixin/): schema composition.
- [Ent transactions](https://entgo.io/docs/transactions/): transactional access patterns.
- [Google UUID API](https://pkg.go.dev/github.com/google/uuid#NewV7): UUID v7 function signature.
- [OpenTelemetry OTLP specification](https://opentelemetry.io/docs/specs/otlp/): export, partial success, and retry behavior.
- [PostgreSQL row security](https://www.postgresql.org/docs/current/ddl-rowsecurity.html): policies, owner behavior, and RLS bypass.
- [Jaeger deployment documentation](https://www.jaegertracing.io/docs/1.76/deployment/): OTLP ingestion and local deployment concepts.

The source code and pinned `go.mod` are the definitive description of this prototype. These references explain underlying APIs; they do not certify this implementation as production-ready.
