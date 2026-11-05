# deadline-bot

[![CI](https://github.com/a-naimankhan/deadline-bot/actions/workflows/ci.yml/badge.svg?branch=master)](https://github.com/a-naimankhan/deadline-bot/actions/workflows/ci.yml)

A personal Telegram bot in Go that tracks university deadlines for a single owner and nags until each task is marked done. It is built with PostgreSQL, a ticker + worker-pool scheduler, and a distroless Docker image. The bot talks in Russian, so the commands and messages below are shown in Russian with English glosses.

<!-- TODO(owner): add docs/screenshot.png and uncomment: ![Bot in Telegram](docs/screenshot.png) -->

## Why

- The university uses MS Teams Assignments, and a student gets no API access to it. Instead, you copy the assignment text from Teams and paste it into the chat (optionally with the PDF), or type one `/add` line.
- Reminders come at a fixed time (12:00 Almaty) and get louder on the deadline day, until you press "Сдал" ("Submitted").
- It is also a portfolio project focused on clean layering, testable concurrency and honest docs.

## Features

- Add, list, complete and snooze tasks from Telegram (owner only; every other chat is ignored and logged).
- Import an assignment by pasting its text from MS Teams, with a draft to confirm first. The assignment PDF can be attached and is resent from the reminder with one button (see [Import from Teams](#import-from-teams)).
- Automatic reminder schedule, described next.
- Inline buttons on every reminder: `Сдал ✅` (done), `Отложить на 3ч` (snooze 3h), `До завтра` (until next 12:00).
- One JSON log line per command or button, with duration and result. Task text is never logged.
- No paid APIs: the import parser is plain Go (regexp), and files stay in Telegram.
- `GET /healthz` and a Docker HEALTHCHECK.

### Reminder rules

Times are Asia/Almaty. They are stored as UTC.

| Reminder | When | Message (gloss) |
|---|---|---|
| d2 | 12:00, 2 days before | `📌 Через 2 дня дедлайн` (deadline in 2 days) |
| d1 | 12:00, 1 day before | `⚠️ Завтра дедлайн` (deadline tomorrow) |
| dday | 12:00 on the deadline day; if the deadline is at or before 12:00, then deadline − 1h | `🚨 СЕГОДНЯ ДЕДЛАЙН, СДАЙ!` (deadline today, submit it!) |
| nag | every 2h after dday, at most 6: all before the deadline plus the first one at or after it | `🔔 ВСЁ ЕЩЁ НЕ СДАНО! Осталось 9ч 59м` (still not submitted, 9h 59m left) or, once passed, `⛔ ДЕДЛАЙН ПРОШЁЛ, А ТЫ НЕ СДАЛ!` (deadline passed and you didn't submit) |
| snooze | when you asked | `⏰ Напоминаю` (reminder) |

Example timeline for `/add БИС | Lab 5 | 23.11 23:59`:

- Sat 21.11 12:00 d2
- Sun 22.11 12:00 d1
- Mon 23.11 12:00 dday
- Mon 14:00, 16:00, 18:00, 20:00, 22:00 nag
- Tue 24.11 00:00 the final "passed" nag

Deadline 09:00 example: dday at 08:00 and one nag at 10:00.

- Reminders whose time already passed when the task is added are skipped (a task added 1.5 days ahead gets no d2).
- Once a task is done, its remaining reminders are never sent. The claim query skips done tasks, so no cleanup is needed.
- "Отложить на 3ч" adds a reminder in 3 hours. "До завтра" adds one at the next 12:00 (today if before noon). Snoozing adds a reminder and does not cancel the planned nags; that is on purpose.

## Commands

- `/start`, `/help`: list of commands
- `/add Subject | Title | date`: create a task (examples below)
- `/list`: tasks not done yet, nearest deadline first, with the time left or `просрочено` (overdue)
- `/done <id>`: mark done (`/done 7`)
- `/snooze <id> <duration>`: extra reminder later
- Any message without a leading `/`: import an assignment from Teams text (next section)

```
/add БИС | Lab 5 | 23.11 23:59
/add ФБИС | Essay | 23.11
/add БИС | Final | 23.11.2026 18:00
/snooze 7 3h
/snooze 7 1h30m
/snooze 7 3ч
/snooze 7 1ч30м
```

- Dates are `day.month` with an optional year and time; without a time the deadline is 23:59.
- Without a year, the current year is used. A date that has already passed is rejected, so in late December write the year explicitly.
- Durations use Go `time.ParseDuration` syntax (Russian `ч` = hours, `м` = minutes also work) and must be > 0.

Example exchange:

```
> /add БИС | Lab 5 | 23.11 23:59
Добавил #7: БИС — Lab 5
Дедлайн: 23.11 (пн) 23:59
> /list
Несданные задачи:

#7 БИС — Lab 5
   до 23.11 (пн) 23:59 (через 2д 3ч)
```

Glosses: "Added #7 ... Deadline: ..."; "Tasks not done: ... due ... (in 2d 3h)".

A reminder message (the dday one) looks like this, with three buttons under it:

```
🚨 СЕГОДНЯ ДЕДЛАЙН, СДАЙ!
БИС — Lab 5 (#7)
До 23.11 (пн) 23:59

[ Сдал ✅ ]
[ Отложить на 3ч ] [ До завтра ]
```

After a button press the message gets a status line (`✅ Сдано`, or `⏰ Отложено до 23.11 (пн) 15:00`) and the buttons disappear.

## Import from Teams

Copy the assignment from MS Teams, write the subject as the first line and send it to the bot:

```
фбис
Reading Exercise: Business Intelligence
Срок заканчивается 10 ноября 2026 г., 23:59
•
Допускается несколько отправок
Инструкции
Read case on pp.512-514 and answer questions on p.514
```

The bot answers with a draft. Nothing is saved until you press a button:

```
Нашёл задание:
ФБИС — Reading Exercise: Business Intelligence
Дедлайн: 10.11 (вт) 23:59

Read case on pp.512-514 and answer questions on p.514

[ Добавить ✅ ] [ Отмена ]
```

Glosses: "Found an assignment ... Deadline ...", buttons "Add" and "Cancel".

How the text is read:

- Line with the deadline: `Срок заканчивается 10 ноября 2026 г., 23:59` ("due on ..."). Also accepted: `Срок сдачи: 10 ноября, 18:00` and a date without a year or time. Missing year means the current year, missing time means 23:59.
- The line just above it is the title; everything above the title is the subject. One-word subjects up to 8 letters are upper-cased (`фбис` → `ФБИС`), so one subject does not end up under two names.
- Text after the deadline goes to `details`, without Teams service lines (`•`, `Допускается несколько отправок`, `Инструкции`).

Validation (the bot refuses with a hint instead of guessing):

- No deadline line, no subject or no title.
- A date that has passed, is more than a year ahead, or does not exist (`31 ноября`, `25:00`).
- Subject over 50 characters (usually the subject line was forgotten), title over 200; details are cut at 1000.
- If an unfinished task has the same title (and the same subject or deadline), the draft shows `⚠️ Похоже, это уже есть: #7 …` ("looks like it already exists").

Drafts:

- Kept in memory for 24 hours. Each draft can be used once, so a second press does not create a duplicate. After a restart, send the text again.
- Plain text that is not an assignment (for example `привет`) gets the format hint. Unknown `/commands` still get the help text.

### Attaching the PDF

- Send the PDF with the Teams text as its caption. Telegram limits captions to 1024 characters; for longer text use the next option.
- Or send the text first and then the PDF without a caption. The file is attached to the latest draft that has not been confirmed yet, and its old buttons keep working.
- Only Telegram's `file_id` is stored (`tasks.file_id`); the file itself stays in Telegram.
- Reminders of a task with a file get one more button, `📎 Задание` ("assignment"). It sends the PDF and leaves the reminder buttons in place. `/list` marks such tasks with 📎.

Error replies: `/done` (or the `Сдал ✅` button) on an unknown or already done task, and `/snooze` on an unknown id, give `Задачи #7 нет или она уже сдана.` (task #7 doesn't exist or is already done). Internal errors give a generic message; details go only to the logs.

## Architecture

Layers: delivery (telegram) -> service -> repository (pgx); the health endpoint pings the pool directly. Interfaces are declared on the consumer side: `service.TaskRepo`, `scheduler.Store`, `scheduler.Notifier`, `telegram.TaskService`, `health.Pinger`.

```mermaid
flowchart LR
  TG["Telegram Bot API"] <-->|long polling| BOT["delivery/telegram: commands, buttons"]
  BOT --> SVC["service: AddTask, Done, Snooze, PlanReminders"]
  SVC --> REPO["repository/postgres (pgx)"]
  SCHED["scheduler: ticker 1m + 3 workers"] -->|"ClaimDue / MarkSent"| REPO
  SCHED -->|Send| NOTIF["telegram Notifier, 1 msg/s"]
  NOTIF --> TG
  REPO --> PG[("PostgreSQL")]
  HEALTH["GET /healthz"] -->|Ping| PG
```

### Life of a reminder

1. `/add` is parsed. `PlanReminders` computes all reminders up front, including nags. The task and its reminders are inserted in one transaction.
2. Once at startup and then every minute, the scheduler calls `ClaimDue(now, 50)`. A single SQL statement selects due, unsent reminders of unfinished tasks whose lease is free or expired, locks them with `FOR UPDATE OF r SKIP LOCKED`, and stamps `claimed_at = now`. That is a 5-minute lease.
3. Claimed reminders go through an unbuffered channel to 3 workers.
4. A worker waits on the rate limiter (1 message/s, Telegram's per-chat limit), sends the message with buttons, then calls `MarkSent`.
5. If the send fails, the reminder is not marked; after the 5-minute lease it is claimed again. If the send succeeds but `MarkSent` fails, a duplicate is possible. The delivery semantics are at-least-once: a duplicate reminder beats a missed deadline.
6. Rationale for SKIP LOCKED + lease: the claim is atomic in the database, so correctness does not depend on running exactly one process, for example when an old and a new container overlap during a redeploy.

### Graceful shutdown

- SIGINT/SIGTERM cancels the root context. `errgroup` runs the bot, the scheduler and the health server; if one fails, the others stop too.
- The scheduler stops claiming, closes the jobs channel and waits for its workers.
- `MarkSent` runs under `context.WithTimeout(context.WithoutCancel(ctx), 5s)`, so a reminder that was already sent is still recorded during shutdown.
- The health server shuts down with a 5s timeout, and the DB pool is closed last.

### Startup and operations

- Migrations are embedded in the binary (`go:embed`) and applied at startup with goose. A Postgres advisory lock prevents two instances from migrating at once.
- `GET /healthz` returns 200 `ok` if the DB answers a ping within 2s, otherwise 503 `db unavailable`. Details go only to the logs.
- The Docker image is built with `golang:1.27.1-alpine`. It is a static CGO-free binary on `gcr.io/distroless/static-debian13:nonroot`, with no shell and running as non-root. tzdata is embedded via `time/tzdata`.
- There is no curl in the image, so the HEALTHCHECK runs `/bot -healthcheck`. This mode reads only `PORT` (no `.env`, no secrets) and GETs `http://127.0.0.1:$PORT/healthz`.

## Tech stack

Go 1.27.1 · PostgreSQL 16 · pgx v5 (pgxpool) · goose v3 · go-telegram/bot · golang.org/x/sync/errgroup · golang.org/x/time/rate · log/slog (JSON) · pgxmock v5 · Docker (distroless) · GitHub Actions

## Quick start

Prerequisites: Go 1.27.1+, Docker with Compose v2, and a bot token from @BotFather.

Option A, the DB in Docker and the bot from source:

```bash
git clone https://github.com/a-naimankhan/deadline-bot.git
cd deadline-bot
docker compose up -d --wait     # Postgres 16 on localhost:5433 (bot/bot, db "deadlines")
cp .env.example .env            # set TELEGRAM_BOT_TOKEN and OWNER_CHAT_ID
go run ./cmd/bot                # applies migrations, starts polling; or: make run
curl http://localhost:8080/healthz   # -> ok
```

Option B, everything in Docker:

```bash
cp .env.example .env            # set TELEGRAM_BOT_TOKEN and OWNER_CHAT_ID
docker compose --profile app up -d --build
docker compose ps               # bot should become "healthy"
docker compose logs -f bot      # Ctrl+C to stop following
```

Notes for option B:

- Inside compose, DATABASE_URL is overridden to `db:5432`.
- `/healthz` is published only on `127.0.0.1:8080`.
- Do not run A and B at the same time: two pollers with one token get `409 Conflict` from Telegram.
- Stop with `docker compose --profile app down` (or `docker compose down` for option A).

See [.env.example](.env.example) and [docker-compose.yaml](docker-compose.yaml).

### Finding your OWNER_CHAT_ID

- Easiest: message @userinfobot in Telegram; it replies with your numeric ID.
- Or: start the bot with the placeholder `OWNER_CHAT_ID=123456789` from `.env.example` and send your bot any message. The bot ignores it and logs a WARN line like `{"time":"…","level":"WARN","msg":"ignored message from stranger","chat_id":<your id>}`. Put that number into `.env` and restart.
- Use a private chat with the bot. Messages are matched by chat ID and buttons by user ID, and in a private chat the two are equal.

## Configuration

| Variable | Required | Default | Description |
|---|---|---|---|
| `TELEGRAM_BOT_TOKEN` | yes | — | Token from @BotFather. Checked at startup (getMe). |
| `OWNER_CHAT_ID` | yes | — | Numeric (int64) Telegram ID of the only user the bot serves. |
| `DATABASE_URL` | yes | — | Postgres DSN, e.g. `postgres://bot:bot@localhost:5433/deadlines?sslmode=disable`. |
| `LOG_LEVEL` | no | `info` | `debug`, `info`, `warn` or `error`. JSON logs to stdout; `debug` adds source locations. |
| `PORT` | no | `8080` | HTTP port for `/healthz` (1..65535). Hosting platforms often set it. |

- `.env` is optional. It is loaded from the current directory or the nearest parent, and real environment variables take precedence.
- If required variables are missing, the bot exits with code 1 and lists all of them.
- Fixed in code on purpose (not configurable): timezone Asia/Almaty, reminder hour 12:00, tick 1 min, 3 workers, claim batch 50, lease 5 min, nag every 2h (max 6), send rate 1 msg/s, DB ping timeout 2s.

## Testing

```bash
make test        # go fmt + go vet + unit tests (CGO_ENABLED=0); or: go test ./...
```

Integration test (real Postgres; skipped when TEST_DATABASE_URL is unset; it applies migrations and cleans up its own rows):

```bash
docker run -d --rm --name deadline-bot-testdb -e POSTGRES_USER=bot -e POSTGRES_PASSWORD=bot \
  -e POSTGRES_DB=deadlines -p 5434:5432 postgres:16
until docker exec deadline-bot-testdb pg_isready -h 127.0.0.1 -U bot -d deadlines; do sleep 1; done
TEST_DATABASE_URL='postgres://bot:bot@localhost:5434/deadlines?sslmode=disable' \
  go test -count=1 -tags integration ./internal/repository/postgres
docker stop deadline-bot-testdb
```

- Race detector: `make test-race` (needs CGO and a C toolchain). CI always runs `go test -race ./...` on Linux.
- CI ([.github/workflows/ci.yml](.github/workflows/ci.yml)) runs on every push to master and every PR. Checks: gofmt, `go vet` (with and without the integration tag), race tests, the integration test against a `postgres:16` service, and `docker build`.
- What is covered:
  - the planner: table-driven edge cases such as noon deadlines, month boundaries and late additions;
  - the service, with a fake repo and a fake clock;
  - parsers, including the Teams import on real assignment texts;
  - the import flow: draft, confirm, cancel, expiry, double press, duplicate warning, PDF in a caption or attached later, the `📎` button;
  - bot commands, buttons and the stranger filter, against a fake Telegram API;
  - notifier texts and buttons (including a cancelled rate-limit wait that sends nothing);
  - the scheduler: send then mark, a failed send is never marked, a claim error does not stop the loop, MarkSent survives shutdown;
  - the health handler;
  - repository error mapping with pgxmock;
  - `ClaimDue` never returning the same reminder twice, and `file_id` surviving a round trip (integration).

## Project layout

```
.
├── cmd/bot/main.go             # wiring: config, logger, DB pool, migrations, errgroup; -healthcheck mode
├── internal/
│   ├── config/                 # env parsing and constants
│   ├── domain/                 # Task, Reminder, domain errors
│   ├── logger/                 # global JSON slog logger
│   ├── service/                # PlanReminders (pure) and task use cases
│   ├── scheduler/              # ticker + worker pool, at-least-once delivery
│   ├── repository/postgres/    # pgx repository, ClaimDue lease, goose migrator
│   └── delivery/
│       ├── telegram/           # commands, buttons, parsers, Teams import + drafts, rate-limited notifier
│       └── health/             # /healthz server and -healthcheck client
├── migrations/                 # goose SQL files + embed.go
├── .github/workflows/ci.yml
├── Dockerfile, .dockerignore, docker-compose.yaml
├── Makefile, .env.example, go.mod, go.sum
└── AGENTS/AGENTS.md            # design notes and decision log
```

## Design decisions

One line each; the full log is in [AGENTS/AGENTS.md](AGENTS/AGENTS.md) (section 5).

- Paste-to-import (plus manual `/add`) instead of an integration: there is no access to the MS Teams Assignments API, and the bot runs on a server, so it cannot read anything from the owner's devices.
- The import parser is regexp-based, not an LLM: no API costs, and its results are deterministic and testable on real Teams texts. A draft with confirmation buttons catches what the parser gets wrong.
- Import drafts live in memory, not in the DB: there is one owner, and resending the text after a restart is cheap.
- Attachments are stored as Telegram `file_id` only: no file storage on the server.
- All reminders (including nags) are planned when the task is created. The scheduler stays simple (claim, send, mark) and holds no business rules.
- At-least-once delivery with a DB lease: a duplicate reminder is cheaper than a missed deadline, and a crashed worker cannot lose one.
- `done_at TIMESTAMPTZ NULL` instead of a status enum: it records both the fact and the time, and there are only two states.
- `BIGSERIAL` IDs: the owner types `/done 7` by hand.
- Interfaces live on the consumer side: each layer depends only on what it uses, and fakes in tests are trivial.
- DB errors are mapped to domain errors inside the repository: only `internal/repository/postgres` imports pgx.
- Times are stored in UTC; Asia/Almaty is used only for display and for computing "12:00 local".
- Only secrets and environment-specific values come from env (plus an optional `PORT`); everything else is a constant. That means fewer knobs to misconfigure.
- Migrations are embedded and run at startup under an advisory lock: one binary, and no separate migrate step on the host.
- One Docker image plus env, host-independent: free hosting tiers change.
- Distroless image with a self-healthcheck: a small attack surface, and no shell or curl needed.

## Limitations

- Single user by design.
- The timezone and the 12:00 reminder time are fixed in code.
- There is no edit or delete command; a wrong task can only be marked done.
- `/add` sets subject, title and deadline only; `details` is filled only by the Teams import and is not shown in reminders yet.
- The import understands the Russian Teams interface only (`Срок заканчивается …`).
- Import drafts are lost on restart.
- A PDF is not parsed; the deadline always comes from the pasted text.
- Duplicates are possible in rare failure cases (at-least-once).
- `/snooze` on an already done task is accepted, but the reminder never fires.
- Not deployed yet; it runs locally or in Docker.

## Roadmap

- **NetAcad tracker** (next): `/netgoal X Y` (X topics in Y days), a daily check-in with buttons, and unfinished topics carried over as debt. Debt is computed, not stored. The `netacad_goals` table already exists; there is no code yet.
- **Teams e-mail import**: read the Teams notification e-mails over IMAP (forwarded to Gmail) and send the same drafts automatically.
- **Parsing microservice**: a separate Go service that turns PDFs and screenshots of assignments into text.
- **Deploy**: the same image plus env on an always-on free host. Long polling needs a process that does not sleep.
- Out of scope: multi-user support, a web UI, a Teams API integration.
