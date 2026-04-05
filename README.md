# Learning Runtime — MCP Server (v0.9)

An adaptive learning engine exposed as a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server. It turns any LLM into a personalised tutor by providing real-time cognitive state tracking, spaced-repetition scheduling, intelligent activity routing, and a metacognitive loop that helps learners become autonomous.

## How It Works

The server sits between a learner and an LLM (e.g. Claude). Two parallel loops run from the first session:

**Learning loop** (what to learn, when) — The LLM calls MCP tools before and after every exchange to check alerts, get the next optimal activity, and record interactions. Four cognitive algorithms update the learner model in real time. The LLM never invents its own scheduling.

**Metacognitive loop** (how the learner learns) — Affect check-ins, calibration tracking, and autonomy metrics observe the learner's relationship to the system. A mirror mechanism surfaces factual observations about dependency patterns without judging. The system aims to make itself progressively unnecessary.

## Cognitive Algorithms

| Algorithm | Role |
|-----------|------|
| **BKT** (Bayesian Knowledge Tracing) | Tracks mastery probability per concept. Distinguishes syntax errors from knowledge gaps. |
| **FSRS** (Free Spaced Repetition Scheduler) | Schedules reviews using stability/difficulty curves. Determines optimal review intervals. |
| **IRT** (Item Response Theory) | Estimates learner ability (θ) from response patterns. Calibrates exercise difficulty. |
| **PFA** (Performance Factor Analysis) | Weights success/failure history to predict performance on each concept. |

All four algorithms run on every interaction and jointly inform the activity router.

## MCP Tools (19)

### Core Learning

| Tool | Description |
|------|-------------|
| `get_learner_context` | Session context: active domain, concept states, recent history |
| `get_pending_alerts` | Critical alerts requiring immediate action |
| `get_next_activity` | Next optimal activity + metacognitive mirror + tutor mode |
| `record_interaction` | Log result; updates BKT/FSRS/IRT/PFA; tracks hints, initiative, proactive reviews |
| `check_mastery` | Check if a concept is eligible for a mastery challenge |
| `get_cockpit_state` | Full dashboard: progress, retention, autonomy score, calibration bias, affect history |
| `get_availability_model` | Learner's time windows and session frequency |
| `init_domain` | Create a knowledge domain with concepts, prerequisite graph, and personal goal |
| `add_concepts` | Add concepts to an existing domain without resetting progress |
| `update_learner_profile` | Persist learner metadata (device, background, level, calibration bias, autonomy score) |

### Metacognitive Loop

| Tool | Description |
|------|-------------|
| `record_affect` | Session check-in: energy + confidence (start), satisfaction + perceived difficulty + next intent (end) |
| `calibration_check` | Before exercise: learner self-assesses mastery (1-5), stores prediction for comparison |
| `record_calibration_result` | After exercise: compares prediction with actual result, updates calibration bias |
| `get_autonomy_metrics` | Autonomy score (0-1) with 4 components: initiative, calibration, hint independence, proactive review |
| `get_metacognitive_mirror` | Factual mirror message if a dependency pattern is consolidated over 3+ sessions |

### Advanced

| Tool | Description |
|------|-------------|
| `feynman_challenge` | Feynman method: learner explains a mastered concept; LLM identifies gaps for BKT injection |
| `transfer_challenge` | Tests concept transfer in novel contexts outside initial learning |
| `record_transfer_result` | Records transfer challenge score for a concept/context pair |
| `learning_negotiation` | Exposes system plan with tradeoffs; learner can propose alternatives |

All tools accept an optional `domain_id` for multi-domain support. Without it, the most recently active domain is used.

## Alert Engine

The scheduler runs background jobs that detect nine alert types:

### Learning Alerts
- **FORGETTING** — FSRS retention dropped below threshold; triggers recall exercise.
- **PLATEAU** — No mastery progress after multiple sessions; triggers debugging case.
- **ZPD_DRIFT** — Error rate too high; outside zone of proximal development.
- **OVERLOAD** — Session exceeds 45 minutes; suggests rest.
- **MASTERY_READY** — Concept ready for mastery challenge.

### Metacognitive Alerts
- **DEPENDENCY_INCREASING** — Autonomy score declining over 3 consecutive sessions.
- **CALIBRATION_DIVERGING** — Calibration bias exceeds threshold; persistent over/under-estimation.
- **AFFECT_NEGATIVE** — Low satisfaction or excessive difficulty on 2 consecutive sessions.
- **TRANSFER_BLOCKED** — BKT shows mastery but transfer scores remain low across contexts.

Alerts are delivered via Discord webhook when configured.

## Autonomy Score

A composite metric (0-1) tracking the learner's progression toward independence:

| Component (25% each) | What it measures |
|----------------------|------------------|
| **Initiative rate** | % of sessions started without a webhook nudge |
| **Calibration accuracy** | How well the learner estimates their own mastery |
| **Hint independence** | Ability to solve mastered concepts without hints |
| **Proactive review rate** | % of reviews done before FSRS scheduled date |

The trend compares the last 5 sessions to the previous 5 (improving / stable / declining).

## Tutor Mode

The system adapts its communication register based on affect state:

| Mode | Trigger |
|------|---------|
| `normal` | Default |
| `scaffolding` | Learner reports high anxiety (confidence = 1) |
| `lighter` | Learner reports fatigue (energy = 1) or frustration (2 negative sessions) |
| `recontextualize` | High energy but low satisfaction (boredom detected) |

## Authentication

OAuth 2.1 with PKCE. Learners register and authenticate through a built-in flow:

- `GET /.well-known/oauth-authorization-server` — server metadata
- `GET /authorize` — registration/login page
- `POST /token` — exchange authorization code for JWT access + refresh tokens
- Bearer token required on `/mcp`
- Rate limiting on auth (10/min), registration (5/min), and MCP (60/min) endpoints

## Architecture

```
main.go              HTTP server, MCP handler, OAuth, scheduler startup
├── auth/            OAuth 2.1 server, JWT middleware, PKCE, rate limiter
├── algorithms/      BKT, FSRS, IRT, KST, PFA (all with tests)
├── engine/
│   ├── alert.go         Learning + metacognitive alert computation
│   ├── router.go        Activity routing with priority-based alert handling
│   ├── metacognition.go Autonomy score, mirror detection, tutor mode
│   └── scheduler.go     Cron jobs: alerts, reminders, webhooks, cleanup
├── models/
│   ├── learner.go       Learner, ConceptState, Interaction, RefreshToken
│   ├── domain.go        AlertType, Activity, KnowledgeSpace, Domain
│   └── metacognition.go AffectState, CalibrationRecord, MirrorMessage, AutonomyMetrics
├── db/
│   ├── store.go         SQLite persistence: learners, domains, concepts, interactions
│   ├── metacognition.go Affect, calibration, transfer, autonomy queries
│   ├── schema.sql       Table definitions (embedded)
│   └── migrations.go    Idempotent migrations for existing databases
└── tools/               19 MCP tool handlers + system prompt
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

- **Go 1.25** with the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- **SQLite** (via modernc.org/sqlite — pure Go, no CGO)
- **JWT** for access tokens, bcrypt for passwords
- **robfig/cron** for background scheduling
- **35 tests** covering algorithms and engine logic

## License

MIT
