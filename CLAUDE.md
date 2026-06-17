# CLAUDE.md

Project guidance for Claude Code. This file is loaded every session — keep it concise and point to authoritative docs rather than duplicating them.

## What this is

**Court booking backend (場地預約系統後端)** — a Go REST API for venue/resource booking, with auth, role-based permissions, and multi-tenant organization management.

## Stack

Go 1.25+ · Gin · PostgreSQL (pgx/v5) · Squirrel query builder · golang-migrate · JWT + bcrypt · OpenAPI/Swagger.

## Architecture

Layered, dependencies point inward (handler → service → repository → model), wired by dependency injection in `internal/app/container.go`.

- **Handler** (`internal/*/http/`) — parse HTTP request (body/query/param), check permissions (middleware), map domain errors to HTTP status. No business logic, no `gin.Context` leaking downward.
- **Service** (`internal/*/service.go`) — core business logic; cross-module orchestration. No HTTP dependencies.
- **Repository** (`internal/*/repository.go`) — raw/Squirrel SQL via pgx; scan rows into structs.
- **Model** (`internal/*/model.go`) — domain entities, enums, filters, and `apperror.New(...)` error definitions.

### Module map (`internal/`)

`user` · `auth` · `organization` · `location` · `resource` · `booking` · `pickup` (pickup-group enrollment) · `favorite` · `file` · `announcement`, plus shared `api` (router/middleware), `app` (DI container), `config`, `db`, `pkg` (e.g. `response`, `apperror`, `request`).

## Conventions

See [README.md](README.md) §開發規範 for the authoritative rules. Key points:

- Format with `go fmt`. All comments/docstrings in **English**. No emojis in code or comments.
- Define business errors with `apperror.New(status, msg)` in the model layer; surface them in handlers via `response.Error(c, err)` (auto-maps `AppError` → its status, anything else → 500 with internal details hidden).
- Raw SQL only (no ORM). Prefer soft delete (`is_active`) for primary entities.
- Schema changes = a **new** migration pair under `db/migrations/` (`{next_version}_{name}.up.sql` + `.down.sql`); never edit an existing migration. Migrations are embedded in the binary and applied automatically on startup.
- List endpoints return `{items, page, page_size, total}`; errors return `{"error": "..."}`.

### Reuse before inventing

When solving a class of problem, check whether another module already handles it:

- Concurrency control: the **pickup** module uses transaction + `SELECT ... FOR UPDATE` + unique-constraint interception (`internal/pickup/repository.go`).
- FK-violation → domain error mapping: `internal/user/repository.go`.

## Commands

```bash
docker compose up -d            # start PostgreSQL + Swagger UI (and test DB)
go run cmd/server/main.go       # run server (auto-applies pending migrations) → :8080
go test ./tests/... -v          # integration tests (connect to the test DB)
go fmt ./...                    # format
```

## Git commits

- Write the commit title in **English**, following the existing history's style: imperative mood, capitalized first word, no trailing period (e.g. "Add golang-migrate for database migrations", "Fix concurrency, auth, and validation issues from code review").
- Title only — **do not** write a commit description/body.
- Commit directly on `main`.

## Code review workflow

Rolling code reviews live in `./ref-only/`:

- `code-review-{idx}.md` — one report per round (Traditional Chinese), highest index = latest.
- `code-review-decisions.md` — issues deliberately NOT fixed (design decisions), so later rounds don't re-report them. May not exist until the first decision is recorded.

The `/review-round` and `/fix-review` slash commands drive this loop.
