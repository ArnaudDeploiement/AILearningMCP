<p align="center">
  <img src="docs/banner.svg" alt="tutor/mcp — Self-learning is a superpower." width="100%" />
</p>

# Tutor MCP — Server (v0.2)

**Tutor MCP is the adaptive brain behind a personalised tutor.** You tell an LLM (Claude, ChatGPT, …) what you want to learn — *Spanish for travel*, *Go for backend*, *options trading*, *medieval history* — and the runtime orchestrates the journey end-to-end: what to study next, when to review, how hard the next exercise should be, when you've mastered a concept, when you're losing motivation, when you're ready to be more autonomous. It works on **any subject the learner can describe in natural language** — no content catalog, no curation, no editorial backlog.

Concretely, it provides real-time cognitive state tracking, spaced-repetition scheduling, intelligent activity routing, misconception diagnosis, a motivation layer, and a metacognitive loop that helps learners become autonomous — all exposed as a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that any MCP-compatible LLM can drive.

## Compatible MCP clients

Tutor MCP works out of the box with the main MCP-capable assistants — connect it to any of them via a custom connector (see [Setup workflow](#setup-workflow)).

<p align="left">
  <a href="#claude-claudeai"><img src="docs/assets/logos/claude.svg" width="40" height="40" alt="Claude" title="Claude" /></a>
  &nbsp;&nbsp;&nbsp;&nbsp;
  <a href="#chatgpt"><img src="docs/assets/logos/openai.svg" width="40" height="40" alt="ChatGPT" title="ChatGPT" /></a>
  &nbsp;&nbsp;&nbsp;&nbsp;
  <a href="#le-chat-mistral"><img src="docs/assets/logos/mistral.svg" width="40" height="40" alt="Le Chat" title="Le Chat (Mistral)" /></a>
  &nbsp;&nbsp;&nbsp;&nbsp;
  <a href="#gemini"><img src="docs/assets/logos/gemini.svg" width="40" height="40" alt="Gemini" title="Gemini" /></a>
</p>

## Design Choice — The LLM is the Content Engine

Traditional learning platforms (Duolingo, Anki, Khan Academy) are bounded by their item banks: once the catalog is exhausted, the system stops. Tutor MCP takes the opposite bet.

> The LLM is trained on humanity's knowledge. It is the most flexible content generator that has ever existed. The runtime never stores exercises — it generates them on demand, calibrated in real time to the learner's mastery, ability, affect, and personal goal.

- **Domains and prerequisite graphs** are co-authored by the learner and the LLM at `init_domain` time — not pre-curated.
- **Exercises, explanations, hints, feedback, motivational nudges** — all generated on the fly by the LLM.
- The **runtime** brings the cognitive science: five algorithms (BKT, FSRS, IRT, PFA, KST), an alert engine, a motivation engine, a metacognitive loop.
- The **LLM** brings infinite domain breadth and on-demand calibration.

This is what makes the system work on day one for any topic — and what lets it scale without an editorial team.

## How It Works

The server sits between a learner and an LLM. Three parallel loops run from the first session:

**Learning loop** (what to learn, when) — The LLM calls MCP tools before and after every exchange to check alerts, get the next optimal activity, and record interactions. Four cognitive algorithms update the learner model in real time. The LLM never invents its own scheduling.

**Metacognitive loop** (how the learner learns) — Affect check-ins, calibration tracking, and autonomy metrics observe the learner's relationship to the system. A mirror mechanism surfaces factual observations about dependency patterns without judging. The system aims to make itself progressively unnecessary.

**Motivation loop** (why this matters) — A brief engine selects a single motivational angle per exercise (milestone, competence value, growth mindset, affect reframe, plateau recontextualization, or utility value linked to the learner's personal goal). The LLM composes the text from structured signals; the system never emits canned pep talk.

## Cognitive Algorithms

| Algorithm | Role |
|-----------|------|
| **BKT** (Bayesian Knowledge Tracing) | Tracks mastery probability per concept. Distinguishes syntax errors from knowledge gaps. |
| **FSRS** (Free Spaced Repetition Scheduler) | Schedules reviews using stability/difficulty curves. Determines optimal review intervals. |
| **IRT** (Item Response Theory) | Estimates learner ability (θ) from response patterns. Calibrates exercise difficulty. |
| **PFA** (Performance Factor Analysis) | Weights success/failure history to predict performance on each concept. |
| **KST** (Knowledge Space Theory) | Validates prerequisite graphs and gates new concepts on mastery of their ancestors. |

All algorithms run on every interaction and jointly inform the activity router.

## MCP Tools (25)

### Core Learning

| Tool | Description |
|------|-------------|
| `get_learner_context` | Session context: active domain, concept states, recent history, active misconceptions |
| `get_pending_alerts` | Critical alerts requiring immediate action |
| `get_next_activity` | Next optimal activity + metacognitive mirror + tutor mode + motivation brief |
| `record_interaction` | Log result; updates BKT/FSRS/IRT/PFA; tracks hints, initiative, proactive reviews, error type, misconception type/detail |
| `check_mastery` | Check if a concept is eligible for a mastery challenge |
| `get_cockpit_state` | Full dashboard: progress, retention, autonomy score, calibration bias, affect history |
| `get_availability_model` | Learner's time windows and session frequency |
| `update_learner_profile` | Persist learner metadata (device, background, level, calibration bias, autonomy score) |

### Domain Management

| Tool | Description |
|------|-------------|
| `init_domain` | Create a knowledge domain with concepts, prerequisite graph, personal goal, and optional value framings |
| `add_concepts` | Add concepts to an existing domain without resetting progress |
| `archive_domain` | Hide a domain from cockpit/routing while preserving progress |
| `unarchive_domain` | Reactivate an archived domain |
| `delete_domain` | Permanently remove a domain and all its data |

### Metacognitive Loop

| Tool | Description |
|------|-------------|
| `record_affect` | Session check-in: energy + confidence (start), satisfaction + perceived difficulty + next intent (end) |
| `calibration_check` | Before exercise: learner self-assesses mastery (1-5), stores prediction for comparison |
| `record_calibration_result` | After exercise: compares prediction with actual result, updates calibration bias |
| `get_autonomy_metrics` | Autonomy score (0-1) with 4 components: initiative, calibration, hint independence, proactive review |
| `get_metacognitive_mirror` | Factual mirror message if a dependency pattern is consolidated over 3+ sessions |

### Diagnosis & Transfer

| Tool | Description |
|------|-------------|
| `get_misconceptions` | Lists detected misconceptions per concept with status (active/resolved) and frequency |
| `feynman_challenge` | Feynman method: learner explains a mastered concept; LLM identifies gaps for BKT injection |
| `transfer_challenge` | Tests concept transfer in novel contexts outside initial learning |
| `record_transfer_result` | Records transfer challenge score for a concept/context pair |
| `learning_negotiation` | Exposes system plan with tradeoffs; learner can propose alternatives |

### Session Close & Nudges

| Tool | Description |
|------|-------------|
| `record_session_close` | Closes session: persists optional implementation intention (Gollwitzer if-then) and returns a recap brief (concepts practiced, wins, struggles, next review, intent prompt) |
| `queue_webhook_message` | Queues an LLM-authored Discord message (daily_motivation, daily_recap, reactivation, reminder) for the scheduler to dispatch in a future window |

All tools accept an optional `domain_id` for multi-domain support. Without it, the most recently active (non-archived) domain is used.

## Alert Engine

The scheduler runs background jobs that detect nine alert types:

### Learning Alerts
- **FORGETTING** — FSRS retention dropped below threshold; triggers recall exercise.
- **PLATEAU** — No mastery progress after multiple sessions; triggers debugging case with rotating exercise format.
- **ZPD_DRIFT** — Error rate too high; enriched with error-type patterns (KNOWLEDGE_GAP / LOGIC_ERROR / SYNTAX_ERROR) and IRT ability signal.
- **OVERLOAD** — Session exceeds 45 minutes; suggests rest.
- **MASTERY_READY** — Concept ready for mastery challenge.

### Metacognitive Alerts
- **DEPENDENCY_INCREASING** — Autonomy score declining over 3 consecutive sessions.
- **CALIBRATION_DIVERGING** — Calibration bias exceeds threshold; persistent over/under-estimation.
- **AFFECT_NEGATIVE** — Low satisfaction or excessive difficulty on 2 consecutive sessions.
- **TRANSFER_BLOCKED** — BKT shows mastery but transfer scores remain low across contexts.

Critical alerts are delivered via Discord webhook; dedup prevents the same alert firing twice in one day. Alert computation, the cockpit, and `priority_concept` filter out concept history that no longer belongs to an active (non-archived) domain, so `delete_domain` keeps progression on disk (re-`init_domain` brings it back) without leaking into reads or webhooks.

## Motivation Engine

On every `get_next_activity` call, a brief engine selects at most one motivational angle for the LLM to voice. Selection is signal-driven and priority-ordered:

| Brief kind | Fires when |
|------------|-----------|
| `milestone` | Mastery just crossed 0.5 / 0.7 / 0.85 |
| `competence_value` | First exercise of a session on a new concept, every 5 sessions on the same concept, or combined with a milestone — rotates through four value axes (Financial, Employment, Intellectual, Innovation) using authored domain-level statements when present |
| `growth_mindset` | A failure occurred on the same concept within 24h |
| `affect_reframe` | Latest end-of-session affect is negative (low satisfaction, high difficulty, or low energy) |
| `plateau_recontext` | A PLATEAU alert is active on this concept |
| `why_this_exercise` | Utility-value fallback linked to `personal_goal` on new-concept or first-of-session exercises |

Each brief also carries a Hidi-Renninger **interest phase** (triggered → emerging → sustained → individual) inferred from session count, mastery, and self-initiated ratio. The LLM receives signals and an instruction — no pre-written text.

## Webhook Queue (LLM-Authored Nudges)

Daily motivation (8h UTC) and daily recap (21h UTC) are **no longer composed by Go templates**. During sessions, the LLM calls `queue_webhook_message` to schedule a warm, personal message tied to the learner's goal; the scheduler dispatches from the queue within a ±30-minute window. A sober Go fallback fires only when the queue is empty. Failed sends are marked for retry; past-due messages are expired hourly.

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
│   ├── alert.go              Learning + metacognitive alert computation
│   ├── router.go             Activity routing with priority-based alert handling
│   ├── metacognition.go      Autonomy score, mirror detection, tutor mode
│   ├── motivation.go         Brief selection + composition (6 kinds, Hidi-Renninger phase)
│   └── scheduler.go          Cron jobs: critical alerts, reviews, queued nudges, cleanup
├── models/
│   ├── learner.go            Learner, ConceptState, Interaction, RefreshToken
│   ├── domain.go             AlertType, Activity, KnowledgeSpace, Domain (+ value framings)
│   ├── metacognition.go      AffectState, CalibrationRecord, MirrorMessage, AutonomyMetrics
│   └── motivation.go         MotivationBrief, RecapBrief, WebhookQueueItem, interest phases
├── db/
│   ├── store.go                     SQLite persistence: learners, domains, concepts, interactions
│   ├── metacognition.go             Affect, calibration, transfer, autonomy queries
│   ├── misconceptions.go            Misconception aggregation and status tracking
│   ├── motivation_queries.go        Brief-engine signals (failures, session counts, self-init ratio)
│   ├── implementation_intentions.go If-then commitments (Gollwitzer)
│   ├── webhook_queue.go             LLM-authored nudge queue with dequeue/dedup/expire
│   ├── schema.sql                   Table definitions (embedded)
│   └── migrations.go                Idempotent migrations for existing databases
└── tools/               25 MCP tool handlers + system prompt
```

## Running

### Setup workflow

The setup has two phases: **deploy the server**, then **connect a client**.

---

#### 1. Deploy the server

**1.1 — Build and start**

```bash
# Required
export JWT_SECRET="your-secret-key"

# Optional
export PORT=3000                       # default: 3000
export DB_PATH=./data/runtime.db       # default: ./data/runtime.db
export BASE_URL=https://your.domain    # public origin, no trailing slash
export LOG_LEVEL=debug                 # debug | info | warn | error

go build -o tutor-mcp && ./tutor-mcp
```

For real use, put the runtime behind a public reverse proxy with TLS — see [Server Configuration](#server-configuration).

**1.2 — Verify**

```bash
curl $BASE_URL/health   # → {"status":"ok"}
```

No manual user registration is needed at this stage. Accounts are created on demand during the first client connection (see below).

---

#### 2. Connect a client

The runtime exposes a single MCP endpoint: `$BASE_URL/mcp`. Add it as a custom connector in any MCP-compatible client.

**Prerequisite for web UIs.** Claude.ai, ChatGPT, Le Chat and Gemini reach your server from their own cloud, not from your browser — they require a publicly reachable HTTPS URL. `http://localhost` will not work. Put the runtime behind a public reverse proxy of your choice (Caddy, Nginx, Traefik, Cloudflare Tunnel, …) with a valid TLS certificate. The steps below assume `https://your.domain` as the public origin.

CLI clients running on the same machine as the runtime can use `http://localhost:3000` directly — see the **Advanced — CLI clients** section below.

**First-time login.** When a client connects for the first time, the runtime starts an OAuth 2.1 + PKCE flow with dynamic client registration (no client ID or secret to copy by hand). The client opens the runtime's `/authorize` page in a new tab — click **"Create one"** to register (email + password) on the very first connection, or log in if the account already exists. Subsequent launches reuse the issued tokens and re-authenticate silently.

##### <img src="docs/assets/logos/claude.svg" width="20" height="20" alt="" /> Claude (claude.ai)

Custom connectors are available on Pro, Max, Team and Enterprise plans (Free is limited to one connector).

1. Open `claude.ai` → **Settings** (or **Customize**) → **Connectors**.
2. Click the **+** button next to *Connectors*.
3. Fill in:
   - **Name**: `Tutor MCP`
   - **Remote MCP server URL**: `https://your.domain/mcp`
4. Click **Add**, then complete the OAuth login when Claude prompts you.

Reference: [Anthropic — Get started with custom connectors](https://support.claude.com/en/articles/11175166-get-started-with-custom-connectors-using-remote-mcp).

##### <img src="docs/assets/logos/openai.svg" width="20" height="20" alt="" /> ChatGPT

Custom MCP connectors require **Developer Mode** and are available on Plus, Pro, Team, Enterprise and Edu plans (not Free).

1. Open `chatgpt.com` → profile picture → **Settings** → **Connectors**.
2. Open **Advanced** at the bottom and toggle **Developer mode** on.
3. Back in **Settings → Connectors**, click **Create** (or **Add new connector**).
4. Fill in:
   - **Name**: `Tutor MCP`
   - **Description**: `Adaptive learning brain (BKT/FSRS/IRT/PFA/KST)`
   - **MCP server URL**: `https://your.domain/mcp`
5. Click **Create** and complete the OAuth login. Write actions still require manual confirmation in chat.

Reference: [OpenAI — Developer mode and MCP apps in ChatGPT](https://help.openai.com/en/articles/12584461-developer-mode-apps-and-full-mcp-connectors-in-chatgpt-beta).

##### <img src="docs/assets/logos/mistral.svg" width="20" height="20" alt="" /> Le Chat (Mistral)

1. Open `chat.mistral.ai` → **Connectors**.
2. Click **+ Add Connector** and switch to the **Custom MCP Connector** tab.
3. Fill in:
   - **Connector name**: `tutor_mcp` (no spaces or special characters)
   - **Server URL**: `https://your.domain/mcp`
   - **Description** (optional)
4. Click **Connect**. Le Chat auto-detects OAuth 2.1 with dynamic registration — complete the login when prompted.

Reference: [Mistral — Configuring a Custom Connector](https://help.mistral.ai/en/articles/393572-configuring-a-custom-connector).

##### <img src="docs/assets/logos/gemini.svg" width="20" height="20" alt="" /> Gemini

As of April 2026, the consumer Gemini web app (`gemini.google.com`) does not yet expose a self-serve UI to add a custom MCP server. Two paths are available today:

- **Gemini Enterprise** (Google Cloud Console) — register the runtime as a [custom MCP server data store](https://docs.cloud.google.com/gemini/enterprise/docs/connectors/custom-mcp-server/set-up-custom-mcp-server). The runtime uses the StreamableHTTP transport, which is the one Gemini Enterprise supports.
- **Gemini CLI** — see [MCP servers with Gemini CLI](https://geminicli.com/docs/tools/mcp-server/).

Watch Google's release notes for native consumer-app support.

---

### Advanced — CLI clients

#### Claude Code

Add a `.mcp.json` in your project root (or `~/.claude/mcp.json` for global use):

```json
{
  "mcpServers": {
    "tutor-mcp": {
      "type": "http",
      "url": "http://localhost:3000/mcp"
    }
  }
}
```

Claude Code runs on the same machine as the server, so `http://localhost:3000/mcp` works directly — no public HTTPS endpoint required. For a remote runtime, replace the URL with `https://your.domain/mcp`.

## Capacity & Sizing

This implementation is intentionally **single-tenant, single-node** — an open-source brick meant to be self-hosted for yourself, a small group, or a modest organisation. No cluster, no multi-writer: SQLite + in-process scheduler.

The figures below include a safety buffer (~50%) against the theoretical limits. Beyond that, you'd need to switch to Postgres + an externalised scheduler.

| Profile | Active learners / day | Registered learners | Usage |
|---------|----------------------|---------------------|-------|
| **Personal** | 1 | 1–5 | individual use |
| **Small group** (family, team) | 1–10 | up to 30 | regular use |
| **Classroom / workshop** | 10–50 | up to 150 | facilitated sessions |
| **Small organisation** | 50–200 | up to 600 | sustained load |

> **Recommended hard ceiling: ~200 concurrent active learners.** Beyond that, the scheduler's 30-minute tick and SQLite's serialised writes (WAL = single writer) become the limiting factor.

## Server Configuration

### Minimum (personal use or small group ≤10)

- **CPU**: 1 vCPU
- **RAM**: 512 MB
- **Disk**: 2 GB SSD (binary ~15 MB + DB that grows slowly)
- **OS**: modern Linux (Debian 12+, Ubuntu 22.04+, Alpine)
- **Network**: outbound Internet for Discord webhooks
- **Examples**: Raspberry Pi 4, €5/month VPS, LXC container

### Recommended (up to 200 active learners)

- **CPU**: 2 vCPU
- **RAM**: 2 GB
- **Disk**: 20 GB SSD (the `interactions` table grows by a few KB per learner per day)
- **OS**: modern Linux with systemd
- **Reverse proxy**: Caddy, Nginx, or Traefik for TLS
- **Backup**: daily snapshot of `data/runtime.db` (in WAL mode: copy `runtime.db` + `runtime.db-wal` or use `sqlite3 .backup`)

### Idle footprint

- Go binary: ~15 MB on disk, ~30 MB RSS
- SQLite WAL database: ~10 MB initial, +~50 KB per active learner per month
- No external dependencies: no Redis, no broker, no second process — everything fits in the binary and the `.db` file

## Tech Stack

- **Go 1.25** with the official [MCP Go SDK](https://github.com/modelcontextprotocol/go-sdk)
- **SQLite** (via modernc.org/sqlite — pure Go, no CGO)
- **JWT** for access tokens, bcrypt for passwords
- **robfig/cron** for background scheduling
- **70 tests** covering algorithms, engine logic, motivation selection, misconception aggregation, and active-domain filtering

## License

MIT — see [LICENSE](./LICENSE).

## Author

**Arnaud Guiovanna**
- Website: [aguiovanna.fr](https://www.aguiovanna.fr)
- GitHub: [@ArnaudGuiovanna](https://github.com/ArnaudGuiovanna)
