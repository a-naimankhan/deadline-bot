# Deadline Bot — AGENTS.md

Single source of truth for humans and AI agents working on this repo.
Last updated: 2026-10-29. If code and this file disagree, the code wins: fix this file.

---

## 1. What we are building

A personal Telegram bot in Go for ONE user (the owner; authorization by `OWNER_CHAT_ID`).
Nickname: "mini Jarvis". It is also a public pet project for GitHub / portfolio, so code quality and honest README matter.

### Features
1. **University deadlines.** The owner has no API access to MS Teams (Assignments), so he adds tasks manually:
   `/add БИС | Lab 5 | 23.10 23:59`.
   The bot sends reminders at **12:00 Asia/Almaty**: 2 days before, 1 day before, and on the deadline day (loud message: "СЕГОДНЯ ДЕДЛАЙН, СДАЙ").
   Every reminder has inline buttons: **Сдал ✅**, **Отложить на 3ч**, **До завтра**.
   If a task is not done, the bot keeps nagging ("душит") until the owner presses Сдал.
   Priority subjects: БИС (Basics of IS) and ФБИС (Fundamentals of Business for IS).
2. **NetAcad tracker** (course: Cybersecurity Essentials; the owner is behind and needs pressure). Stage 2.
   The owner sets "X topics in Y days"; the bot asks about progress daily. Anything not marked done becomes **debt** carried to the next days. Later: progress via screenshot.
3. **Parsing microservice** (stage 3): separate Go service that parses PDF / screenshots into text. Based on the owner's unfinished "file processing system" in Go.
4. **Deploy** (stage 4): free hosting. The laptop cannot be a server.

### Non-goals (for now)
Multi-user support, web UI, Teams API integration, heavy local LLMs (Ollama on hosting is not an option).

---

## 2. How we work (roles and rules)

### Roles
- **Owner** — architect. Makes all key decisions and commits. Wants to understand every decision ("не слепой вайбкодер").
- **Main assistant (Claude chat)** — designs, writes code, prepares prompts for agents, triages agent reviews. Presents forks with a recommendation; the owner decides.
- **AI agents (this file's readers)** — executors/reviewers working in the repo (they can run `go build/vet/test`, migrations, integration tests). Free-tier limits: be economical.

### Rules for agents
1. **Do not commit or push.** The owner commits.
2. Make only the changes requested. No new features, patterns or abstractions "while you're at it". Report extra findings instead of fixing them.
3. If a point is ambiguous or conflicts with the code, STOP on that point and describe it. Do not guess.
4. **Do not tinker with dependency versions or `go.mod` by hand.** Do not loop `go get` / `go mod tidy` trying versions. If a build fails because of version compatibility, report the exact error and `go list -m` output and stop; the owner picks versions.
5. Never print, request, or commit secrets (bot token, DSN with password). Use mocks / `.env.example`.
6. Do not run `go test -race` on Windows (no CGO). Use `make test-race` only where a C toolchain exists (CI).
7. Do not write a full development plan unless asked; this file already has the roadmap.
8. Report format: files changed (one sentence each) → command output (errors in full) → what you could not do → findings you did not fix.
9. Do not trust claims from your own training data about "nonexistent" Go or library versions. Today's toolchain is newer than most training data. Check with `go version` / `go list -m`.

### Commits
Conventional style, one commit per step: `feat(config): ...`, `feat(repo): ...`, `fix(...)`, `test(...)`, `chore: ...`.

---

## 3. Stack and environment

- Go toolchain **1.27.1** installed (go.mod says 1.26.8 — fine; Dockerfile must use a matching Go image).
- PostgreSQL via **pgx v5** (`v5.11.0`), `pgxpool`.
- Migrations: **goose** (`migrations/*.sql`).
- Telegram: `github.com/go-telegram/bot` (planned).
- Logging: stdlib `log/slog`, JSON, global (see decisions).
- Env: `github.com/joho/godotenv` (only in `main`).
- Tests: stdlib `testing`; `pgxmock/v5` (`v5.2.0`) for cheap unit tests; real Postgres for integration tests.
- Owner develops on **Windows without CGO**. Some tooling runs in a Linux shell without C headers: keep `CGO_ENABLED=0` as default (see Makefile).

### Makefile targets
`make fmt`, `make vet`, `make test`, `make test-race` (`CGO_ENABLED=1`, CI only), `make run`, `make migrate-up`, `make migrate-down`.
`CGO_ENABLED=0` must be the default for `vet`/`test` — pgx is pure Go, and a missing C toolchain must not break the build.

---

## 4. Architecture and conventions

Layers: **delivery (telegram) → service → repo**. A layer knows the layer below only through an interface, **declared on the consumer side** (e.g. `service.TaskRepo`, `scheduler.Store`).

- `ctx context.Context` is always the first argument for anything touching DB/network.
- Errors wrapped with `%w`; no `panic` in working code; no mutable globals (the logger is the one deliberate exception).
- Constructors named `NewX`. Dependencies injected; time injected via `clock func() time.Time` where logic depends on "now".
- All times stored and passed as **UTC**; converted to `Asia/Almaty` only for display and for computing "12:00 local".
- DB errors are mapped to domain errors **inside the repository**: `pgx.ErrNoRows` and FK violation (`23503`) → `domain.ErrNotFound`. Only `internal/repository/postgres` imports pgx.
- Concurrency from day one: scheduler goroutine with ticker + worker pool, `errgroup` in `main`, graceful shutdown on SIGINT/SIGTERM.
- Secrets never in logs. Do not log the whole `Config`.

---

## 5. Decision log (made by the owner)

| Topic | Decision | Why |
|---|---|---|
| IDs | `BIGSERIAL` / `int64` | owner types `/done 7` by hand |
| Task status | `done_at TIMESTAMPTZ NULL` (NULL = not done) | gives fact + time, only 2 states |
| Subject | free text | no JOINs or management commands needed yet |
| DB errors | mapped to domain errors in repo | DB may not always be Postgres |
| Transactions | inside repo method (`CreateTask`), no UnitOfWork | personal bot, no over-engineering |
| Config | env only for: `TELEGRAM_BOT_TOKEN`, `OWNER_CHAT_ID`, `DATABASE_URL`, `LOG_LEVEL`; the rest are constants | secrets + env-specific only |
| `.env` | loaded in `main` (`loadDotEnv`, searches upward), never in `config.Load()`; missing file is not an error | hosting provides real env |
| `config.Load()` | returns `(*Config, error)`, no panic; lists all missing vars | clean exit code 1 |
| Logger | global singleton: `logger.Init(level) error` (sync.Once) + `logger.Get()` = `slog.Default()` | owner's explicit choice |
| Log level | text `debug\|info\|warn\|error`, default `info`; validated only in logger | single place of validation |
| Reminder time | 12:00 local, constant in code | owner may lower it later |
| Input | manual `/add`; no Teams API | not available |
| Hosting | free tier; code must stay host-independent (single Docker image + env) | free tiers change |
| Postgres | chosen (also for the owner's interview) | — |

---

## 6. Current state (2026-10-29)

**Done:** config (+tests), logger, `main` skeleton (loadDotEnv, config, logger, pool, repo, graceful shutdown), domain, migrations, repository (tasks + reminders) with unit tests and an integration test.

**Not done:** service layer, Telegram delivery, scheduler logic, wiring in `main`, nagging rules, NetAcad, parsing microservice, Dockerfile/CI, README.

### Structure
```
deadline-bot/
├── cmd/bot/main.go
├── internal/
│   ├── config/            config.go, config_test.go
│   ├── domain/            errors.go, reminder.go, task.go
│   ├── logger/            slogger.go
│   ├── repository/postgres/
│   │     pool.go, repo.go, tasks.go, reminders.go,
│   │     *_test.go, repo_integration_test.go (tag: integration)
│   ├── scheduler/         scheduler.go   (interfaces + TODO)
│   ├── service/           (empty)
│   └── delivery/telegram/ (planned, see cleanup item 3)
├── migrations/            00001_init.sql, 00002_claim.sql
├── Makefile, go.mod, go.sum, .env.example, .gitignore
```

---

## 7. Configuration

Required env: `TELEGRAM_BOT_TOKEN`, `OWNER_CHAT_ID` (int64), `DATABASE_URL`.
Optional: `LOG_LEVEL` (default `info`).
Constants in `config`: timezone `Asia/Almaty`, `ReminderHour=12`, `Tick=1m`, `Workers=3`.
Import `_ "time/tzdata"` in `main` so `LoadLocation` works in minimal Docker images.

---

## 8. Database schema

`tasks`: `id BIGSERIAL PK, subject TEXT, title TEXT, details TEXT DEFAULT '', deadline TIMESTAMPTZ, done_at TIMESTAMPTZ NULL, created_at TIMESTAMPTZ DEFAULT now()`. Partial index on `deadline WHERE done_at IS NULL`.

`reminders`: `id BIGSERIAL PK, task_id → tasks ON DELETE CASCADE, kind TEXT CHECK IN ('d2','d1','dday','nag','snooze'), remind_at TIMESTAMPTZ, claimed_at TIMESTAMPTZ NULL, sent_at TIMESTAMPTZ NULL`. Partial index `reminders_due_idx (remind_at) WHERE sent_at IS NULL`.

`netacad_goals` (stage 2): `id, total_topics, per_day, done_topics, start_date, end_date, active`.
NetAcad **debt is computed, not stored**: `debt = per_day × elapsed_days − done_topics`.

---

## 9. Repository contracts (implemented)

`type DB interface { Begin, Exec, Query, QueryRow }` — satisfied by `*pgxpool.Pool` and `pgxmock.PgxPoolIface`. `New(db DB) *Repo`.

| Method | Contract |
|---|---|
| `CreateTask(ctx, t, rem) (Task, error)` | task + reminders in ONE transaction; returns task with ID, CreatedAt |
| `ListActive(ctx) ([]Task, error)` | not done, `deadline ASC` |
| `GetTask(ctx, id) (Task, error)` | `domain.ErrNotFound` if absent |
| `MarkDone(ctx, id, at) error` | `ErrNotFound` if absent **or already done** |
| `AddReminder(ctx, rem) error` | `ErrNotFound` on FK violation (23503) |
| `ClaimDue(ctx, now, limit) ([]DueReminder, error)` | atomic lease: `FOR UPDATE OF r SKIP LOCKED`, only undone tasks, lease 5 min via `claimed_at` |
| `MarkSent(ctx, id, at) error` | `ErrNotFound` if absent or already sent |

Delivery semantics are **at-least-once**: a worker that dies mid-send loses its lease after 5 minutes and the reminder is claimed again.
Done tasks never reach `ClaimDue`, so their pending reminders need no cleanup.

---

## 10. Specs for the next steps

### Step 6 — `internal/service/planner.go` (pure function)
```go
func PlanReminders(deadline, now time.Time, loc *time.Location, hour int) []domain.Reminder
```
- `d2`: deadline's local date − 2 days at `hour:00`; `d1`: − 1 day; `dday`: deadline's date at `hour:00`.
- If the `dday` time is not before the deadline (e.g. deadline 09:00), use `deadline − 1h`.
- Drop everything with `RemindAt <= now`. Sorted ascending, UTC.
- Table-driven tests: deadline in 5 days; in 1.5 days (`d2` dropped); tonight; today 09:00 with hour=12; already passed.

### Step 7 — `internal/service/tasks.go`
```go
type TaskRepo interface { // declared in service
    CreateTask(ctx, domain.Task, []domain.Reminder) (domain.Task, error)
    ListActive(ctx) ([]domain.Task, error)
    MarkDone(ctx, id int64, at time.Time) error
    AddReminder(ctx, domain.Reminder) error
}
func New(repo TaskRepo, loc *time.Location, hour int, clock func() time.Time) *Service
type AddTaskInput struct{ Subject, Title, Details string; Deadline time.Time }
func (s *Service) AddTask(ctx, in AddTaskInput) (domain.Task, error) // ErrInvalidInput: empty subject/title, deadline not in future
func (s *Service) ListActive(ctx) ([]domain.Task, error)
func (s *Service) Done(ctx, taskID int64) error
func (s *Service) Snooze(ctx, taskID int64, d time.Duration) error        // reminder kind=snooze at now+d
func (s *Service) SnoozeUntilTomorrow(ctx, taskID int64) error            // next day at ReminderHour local
```
Tests with a fake repo and a fake clock.

### Step 8 — `internal/delivery/telegram/parse.go`
```go
func ParseAdd(args string, now time.Time, loc *time.Location) (service.AddTaskInput, error)
```
Format `Subject | Title | date`. Dates: `DD.MM HH:MM`, `DD.MM.YYYY HH:MM`, `DD.MM` (time 23:59). No year → current year; a date in the past is an error. Tests: normal, extra spaces, missing separator, bad date, past date.

### Step 9 — Telegram delivery
- `Bot.Run(ctx) error` blocks until ctx is cancelled. Any update/callback not from `OWNER_CHAT_ID` is ignored and logged at warn.
- Commands: `/start`, `/add`, `/list`, `/done <id>`.
- Inline callbacks: `done:<taskID>`, `snooze:<taskID>:3h`, `snooze:<taskID>:tomorrow`. Always answer the callback and edit the message so buttons disappear.
- `Notifier.Send(ctx, domain.DueReminder) error` implements `scheduler.Notifier`; uses `rate.Limiter` (`limiter.Wait(ctx)` before each send). `d2`/`d1` calm text; `dday`/`nag` loud text.

### Step 10 — Scheduler (`internal/scheduler`)
Ticker goroutine → `ClaimDue(now, 50)` → unbuffered `jobs` channel → N workers → `Notifier.Send` → `MarkSent`.
- On shutdown: stop claiming, close `jobs`, `wg.Wait()`.
- `MarkSent` uses `context.WithTimeout(context.WithoutCancel(ctx), 5s)` so a sent reminder is still marked during shutdown.
- On send error do NOT call `MarkSent`; the lease expires and the reminder retries.
- Test with fake `Store`/`Notifier`: two reminders go out; a send error never triggers `MarkSent`.

### Step 11 — Nagging
After a successful send of a `dday` or `nag` reminder for an undone task: schedule a `nag` in +3h until the deadline, then 1–2 more after it, then stop. Where this logic lives (service vs scheduler) is the owner's decision — OPEN.

### Step 12 — Wiring in `main`
cfg → logger → pool (`defer pool.Close()`) → repo → service → bot + notifier → scheduler → `errgroup` → graceful shutdown.
DoD: `/add`, `/list`, `/done` work locally; a test reminder with a deadline a few minutes ahead arrives.

### Step 13 — Release v1
Multi-stage Dockerfile (Go image matching go.mod), GitHub Actions (`go vet`, `go test -race`, integration job with a Postgres service), README with a screenshot.

---

## 11. Roadmap

- **Stage 2 — NetAcad:** `/netgoal X Y` (X topics over Y days, per_day = X/Y), daily check-in with buttons ("Всё прошёл" / number), debt carried over, firm nagging tone. Manual numbers first; screenshot input later.
- **Stage 3 — parser microservice:** separate service on hosting, worker pool over files; AI via API only if needed (free-tier limits — check current terms); regex/manual dates first.
- **Stage 4 — deploy:** one Docker image + env. A candidate free setup discussed: always-free small VM + free serverless Postgres. Free-tier terms change: verify before relying on them. Long-polling bots need an always-on process; sleeping free tiers break them.

---

## 12. Testing strategy

- Unit tests (stdlib + small fakes): config, planner, service, parser, scheduler.
- `pgxmock/v5` tests only for cheap repo concerns: **error mapping** (`errors.Is(err, domain.ErrNotFound)`), behavior on DB errors (error wrapped and NOT `ErrNotFound`), FK violation (`&pgconn.PgError{Code: "23503"}`).
- **Real Postgres** (tag `integration`, `TEST_DATABASE_URL`) for anything where SQL behavior matters, especially `ClaimDue` (second claim must not return the same reminder).
  `go test -tags integration ./internal/repository/postgres`
- If pgxmock keeps fighting the pgx version, the fallback decided in advance: drop mock tests for the repo and rely on integration tests. Do not burn time on version juggling.

---

## 13. Known issues / cleanup list

1. `migrations/00002_claim.sql`: SQL placed after the `+goose Down` section looks misplaced. Fix, and verify `make migrate-up` then `make migrate-down` both work. (Option: fold `claimed_at` into `00001` if nothing was ever applied elsewhere.)
2. `env-example.env` should be named `.env.example`; keep it in sync with the required variables.
3. Directory naming: `internal/transport-telegram/` (empty) vs planned `internal/delivery/telegram/`. Use `delivery/telegram`, remove the empty directory.
4. `reminders_test.go` contains `t.Skip` stubs with a wrong reason. Replace with real tests (see §12).
5. `tasks_test.go`: "not found" cases must assert `errors.Is(err, domain.ErrNotFound)`, not just `err != nil`; error cases should use a plain `errors.New("boom")`.
6. Compatibility note about pgxmock in older docs is stale/unverified: the `TypeMap` build error came from `pgxmock/v2`. Re-check with `v5.2.0` + pgx `v5.11.0` and update this section.
7. `main.go`: remove stale TODO comments; repo is created but unused until the service exists.
8. `scheduler.go` is a stub. Do not extend it until step 10.
9. Confirm `import _ "time/tzdata"` exists in `main`.

---

## 14. Security and git hygiene

- The repository is **public**. `.env` is in `.gitignore`; `go.mod` and `go.sum` MUST be committed (never ignore `*.sum`).
- If a bot token ever lands in git history: revoke it via @BotFather immediately.
- Never put real credentials in tests; use mocks and a local docker-compose database (`bot/bot@localhost:5432/deadlines`).
- `.gitignore` should cover: `.env`, `bin/`, `*.log`, `.DS_Store`, `go.work`, `go.work.sum`.
