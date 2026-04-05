# Learning Runtime — MCP Server

An adaptive learning engine exposed as a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server. It turns any LLM into a personalised tutor by providing real-time cognitive state tracking, spaced-repetition scheduling, and intelligent activity routing.

## How It Works

The server sits between a learner and an LLM (e.g. Claude). The LLM calls MCP tools before and after every exchange to:

1. **Check alerts** — detect forgetting, plateaus, cognitive overload, or zone-of-proximal-development drift.
2. **Get the next optimal activity** — the router picks the best exercise type and concept based on the learner's current state.
3. **Record interactions** — each response is logged and fed into four cognitive algorithms that update the learner model in real time.

The LLM never invents its own scheduling — the runtime decides *what* to teach and *when*, while the LLM handles *how*.

## Cognitive Algorithms

| Algorithm | Role |
|-----------|------|
| **BKT** (Bayesian Knowledge Tracing) | Tracks mastery probability per concept. Distinguishes syntax errors from knowledge gaps. |
| **FSRS** (Free Spaced Repetition Scheduler) | Schedules reviews using stability/difficulty curves. Determines optimal review intervals. |
| **IRT** (Item Response Theory) | Estimates learner ability (θ) from response patterns. Calibrates exercise difficulty. |
| **PFA** (Performance Factor Analysis) | Weights success/failure history to predict performance on each concept. |

All four algorithms run on every interaction and jointly inform the activity router.

## MCP Tools

| Tool | Description |
|------|-------------|
| `get_learner_context` | Session context: active domain, concept states, recent history |
| `get_pending_alerts` | Critical alerts requiring immediate action |
| `get_next_activity` | Next optimal activity (session-aware, dedup-aware) |
| `record_interaction` | Log an interaction result; updates all four algorithms; returns fatigue/frustration signals |
| `check_mastery` | Check if a concept is eligible for a mastery challenge |
| `get_cockpit_state` | Full dashboard: per-concept progress, retention, ETA |
| `get_availability_model` | Learner's time windows and session frequency |
| `init_domain` | Create a knowledge domain with concepts and prerequisite graph |
| `add_concepts` | Add concepts to an existing domain without resetting progress |
| `update_learner_profile` | Persist learner metadata (device, background, level, etc.) |

All tools accept an optional `domain_id` for multi-domain support. Without it, the most recently active domain is used.

## Alert Engine

The scheduler runs background jobs that detect five alert types:

- **FORGETTING** — FSRS retention dropped below threshold; triggers recall exercise.
- **PLATEAU** — No mastery progress after multiple sessions; triggers debugging case.
- **ZPD_DRIFT** — Error rate too high; the learner is outside their zone of proximal development.
- **OVERLOAD** — Too many concepts in review simultaneously; suggests rest.
- **MASTERY_READY** — Concept is ready for a mastery challenge.

Alerts are delivered via webhook when the learner has one configured.

## Authentication

OAuth 2.1 with PKCE. Learners register and authenticate through a built-in flow:

- `GET /.well-known/oauth-authorization-server` — server metadata
- `GET /authorize` — registration/login page
- `POST /token` — exchange authorization code for JWT access + refresh tokens
- Bearer token required on `/mcp`

## Architecture

```
main.go              HTTP server, MCP handler, OAuth, scheduler startup
├── auth/            OAuth 2.1 server, JWT middleware, PKCE
├── algorithms/      BKT, FSRS, IRT, PFA (all with tests)
├── engine/
│   ├── router.go    Activity routing with priority-based alert handling
│   └── scheduler.go Cron jobs: alert detection, review reminders, webhooks
├── models/          Domain types: learner, concept state, alerts, activities
├── db/              SQLite store, migrations, schema
└── tools/           MCP tool handlers + system prompt
```

## Running

```bash
# Required
export JWT_SECRET="your-secret-key"

# Optional
export PORT=3000              # default: 3000
export DB_PATH=./data/runtime.db  # default: ./data/runtime.db
export BASE_URL=http://localhost:3000
export LOG_LEVEL=debug        # debug | info | warn | error

go build -o learning-runtime && ./learning-runtime
```

## Tech Stack

- **Go** with the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- **SQLite** (via modernc.org/sqlite — pure Go, no CGO)
- **JWT** for access tokens, bcrypt for passwords
- **robfig/cron** for background scheduling

## License

MIT
