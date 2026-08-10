# Agent Guide

## Project Boundaries

- Read `CLAUDE.md` first; its response and Swagger rules remain required.
- `cmd/main.go` starts the HTTP service; `cmd/seed/main.go` is a separate, destructive seed command.
- Routes are registered in `internal/api/routes.go` under `/api/v1`; handlers and service logic live together in `internal/api`.
- `internal/db` is sqlc-generated code. Edit `sqlc/query/*.sql`, `sqlc/schema.sql`, or `sqlc.yaml`, then run `sqlc generate`; do not hand-edit generated `.sql.go` files.
- `internal/pkg/render` owns the JSON response and request-body helpers; reuse them instead of introducing per-handler formats.

## Required Checks

```sh
go test ./...
make swagger
```

- Run the focused package test with `go test ./internal/api -run TestSearchType -count=1` (replace the test name as needed).
- `make swagger` is the canonical Swagger command: it runs `swag fmt` and generates OpenAPI v3.1 with `--parseInternal`; do not use bare `swag init ... --ot yaml`, which changes the document format.
- Swagger annotations use generic `render.Response[T]`; no-data success annotations use `render.ResponseWithoutData`. Keep `@accept`/`@produce` global in `routes.go` only.
- Keep `@Success` comments tab-aligned. Regenerate `docs/swagger.yaml` whenever API annotations or response types change.

## Runtime Setup

- There is no migration runner. Initialize PostgreSQL from `sqlc/schema.sql` before running the service; the schema requires PostgreSQL support for `uuidv7()`.
- The service reads `DATABASE_URL`, `JWT_SECRET`, and `HTTP_ADDR`; defaults are local development values. `JWT_SECRET` must be at least 32 characters.
- The default service database URL is `postgres://postgres:password@localhost:5432/blue_book?sslmode=disable` and the seed command currently uses that URL directly rather than `DATABASE_URL`.
- Running `go run ./cmd/seed` truncates application tables before inserting data; never run it against a database that must be preserved.
- Media presigning requires `S3_ACCESS_KEY_ID`, `S3_ACCESS_KEY_SECRET`, `S3_BUCKET`, `S3_ENDPOINT`, `S3_REGION`, `S3_USE_SSL`, and `S3_USE_PATH_STYLE`; without valid storage config the endpoint intentionally returns 503.

## Implementation Traps

- Do not ignore database errors. Use `db.Store.ExecTx` for multi-step writes that update relationships or denormalized counters, and classify `pgx.ErrNoRows` at the handler boundary.
- Use `render.ReadBody` for required JSON and `render.ReadOptionalBody` for optional JSON. The body helpers write the 400 response themselves; return immediately on error.
- Use `render.Success` when a response has `data`; use `render.SuccessNoData` for no-data success operations. Those operations return HTTP 204, so the wire response has no body even though the Swagger type is `ResponseWithoutData`.
- Preserve idempotency and counter updates in the existing SQL transactions for likes, follows, collections, comments, and posts.
