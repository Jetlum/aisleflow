# Verification record

Implementation environment: Windows amd64, Go 1.27.0, 24 September 2026. Module target: Go 1.26.0. Dependency manifests and generated Ent source are included in the workspace.

## Executed successfully

| Check | Result |
|---|---|
| `go build ./...` | Pass |
| `go vet ./...` | Pass |
| `bpmn2go --check` through `go run` | Pass: 13 nodes, 13 flows, both committed compiler artifacts match |
| Ent generation through `go generate ./backend/analytics/ent` | Pass |
| Hash comparison before/after Ent regeneration | Byte-identical generated files |
| Parser fuzzing, 10-second budget, two workers | Pass: 19,046 executions; 31 additional interesting inputs; no failure |
| Compiler tests | Pass, including final race-enabled compiler package |
| Analyzer tests | Pass, including race-enabled run |
| OTLP transport tests | Pass, including race-enabled run |
| Workflow runtime tests | Pass, including race-enabled run |
| Final closed-loop functional test | Pass using the actual movement activity, OTel SDK exporter, real gRPC, analyzer, dispatcher, and generated workflow |
| PostgreSQL integration test compilation | Pass using `-tags=integration -run '^$'`; database assertions were **not executed** |

The closed-loop test exports each of nine movements twice. It asserts 18 successful exports, exactly one accepted congestion alert, and this completed route:

```text
pick_01       A01
pick_02       A01
pick_03       A01
pick_04       A01
pick_05       A02
pick_06       A02
pick_07       A02
pick_08       A02
pack_express  EXPRESS
```

## Environment-limited checks

- The final closed-loop **race-enabled** executable was blocked by Windows Application Control, including an approved attempt outside the sandbox. Earlier race tests passed before this test was strengthened to exercise the production OTel exporter. The final race result for this package is unverified.
- The parser fuzz executable was initially blocked by Application Control; the approved final attempt completed successfully.
- Aggregate `go test -race ./...` did not finish green because of the executable launch block. Do not report the aggregate race suite as passing.
- Docker and Task are not installed on this machine. The Compose services, container builds, live Temporal Server path, PostgreSQL RLS/transaction assertions, and Jaeger UI have not been run here.
- GitHub Actions configuration is provided; no repository was published and no hosted CI run was triggered.

These are execution-environment limits, not silently skipped successful tests. No Windows security controls were disabled.

## Before showing the full stack

On a machine with Docker and Go:

```sh
docker compose up -d --build --wait
docker compose run --rm demo
go test -race -count=1 ./...
go test ./backend/workflowgen/core -run '^$' -fuzz=FuzzParser -fuzztime=10s
```

Then set `TEST_DATABASE_URL` to the non-superuser runtime database connection and run:

```sh
go test -tags=integration -count=1 ./tests/integration
```

The README includes the PowerShell environment variable command. Confirm the live workflow accepted an alert and later tasks used A02; live asynchronous timing can differ from the controlled test. Inspect Temporal history and the `aisleflow-worker` service in Jaeger. Do this before presenting the Docker path as demonstrated.
