<p align="center">
  <img src="docs/banner.svg" alt="tutor/mcp — Self-learning is a superpower." width="100%" />
</p>

# Tutor MCP — Open-Source AI Tutor & Intelligent Tutoring System (ITS) for LLMs

> Self-hosted **AI tutor** runtime that turns any LLM (Claude, ChatGPT, Le Chat, Gemini) into a true **Intelligent Tutoring System (ITS)** — grounded in 50 years of cognitive science (BKT, FSRS, IRT, PFA, KST) and exposed over the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/). Adaptive learning, spaced repetition, mastery tracking, misconception diagnosis and a metacognitive loop — for any subject, with no item bank to curate.

**Tutor MCP is the adaptive brain behind a personalised tutor.** You tell an LLM (Claude, ChatGPT, …) what you want to learn — *Spanish for travel*, *Go for backend*, *options trading*, *medieval history* — and the runtime orchestrates the journey end-to-end: what to study next, when to review, how hard the next exercise should be, when you've mastered a concept, when you're losing motivation, when you're ready to be more autonomous. It works on **any subject the learner can describe in natural language** — no content catalog, no curation, no editorial backlog.

Under the hood, it provides real-time cognitive state tracking, spaced-repetition scheduling, intelligent activity routing, misconception diagnosis, a motivation layer, and a metacognitive loop that helps learners become autonomous — all exposed as a [Model Context Protocol (MCP)](https://modelcontextprotocol.io/) server that any MCP-compatible LLM can drive.

> Current release: **v0.3** — adds the regulation pipeline (BKT-aware action selector, KST-aware concept selector, gate controller, and a pure-FSM phase orchestrator). Active by default; the legacy router remains available as a kill switch via `REGULATION_PHASE=off`.

## An Intelligent Tutoring System, not a chatbot

For 50 years, **Intelligent Tutoring Systems (ITS)** research — Anderson, VanLehn, Koedinger and others — has converged on four pillars: a **domain model**, a **learner model**, a **pedagogical model**, and an **interface**. Generic LLM chat covers only the last one. Tutor MCP supplies the first three as a deterministic runtime, and delegates the interface and the natural-language content to the LLM:

| ITS pillar | Provided by | How |
|------------|-------------|-----|
| **Domain model** | Tutor MCP runtime | Concept graph with prerequisites, validated by **KST** (Knowledge Space Theory) |
| **Learner model** | Tutor MCP runtime | Mastery, ability and recall predicted by **BKT** (Bayesian Knowledge Tracing), **IRT** (Item Response Theory) and **PFA** (Performance Factor Analysis) |
| **Pedagogical model** | Tutor MCP runtime | Spaced-repetition scheduling via **FSRS** (Free Spaced Repetition Scheduler), activity router, alert engine, motivation engine, metacognitive loop |
| **Interface & content** | Any MCP-compatible LLM | Claude, ChatGPT, Le Chat, Gemini — generates exercises, hints, feedback and dialogue on demand |

The cognitive science is rigid and measurable; the LLM is infinitely flexible. Together they make an ITS that works on day one for any topic — without an editorial team.

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

## Design Choice — The LLM is the Content Engine, the Runtime is the ITS

Traditional learning platforms (Duolingo, Anki, Khan Academy) and most "AI tutor" demos are bounded by their item banks: once the catalog is exhausted, the system stops. Tutor MCP takes the opposite bet.

> The LLM is trained on humanity's knowledge. It is the most flexible content generator that has ever existed. The runtime never stores exercises — it generates them on demand, calibrated in real time to the learner's mastery, ability, affect, and personal goal.

- **Domains and prerequisite graphs** are co-authored by the learner and the LLM at `init_domain` time — not pre-curated.
- **Exercises, explanations, hints, feedback, motivational nudges** — all generated on the fly by the LLM.
- The **runtime** brings the cognitive science of an ITS: five algorithms (BKT, FSRS, IRT, PFA, KST), an alert engine, a motivation engine, a metacognitive loop.
- The **LLM** brings infinite domain breadth and on-demand calibration.

This is what makes the system work on day one for any topic — and what lets it scale without an editorial team.

## How It Works

The server sits between a learner and an LLM. Three parallel loops run from the first session:

**Learning loop** (what to learn, when) — The LLM calls MCP tools before and after every exchange to check alerts, get the next optimal activity, and record interactions. Four cognitive algorithms update the learner model in real time. The LLM never invents its own scheduling.

**Metacognitive loop** (how the learner learns) — Affect check-ins, calibration tracking, and autonomy metrics observe the learner's relationship to the system. A mirror mechanism surfaces factual observations about dependency patterns without judging. The system aims to make itself progressively unnecessary.

**Motivation loop** (why this matters) — A brief engine selects a single motivational angle per exercise (milestone, competence value, growth mindset, affect reframe, plateau recontextualization, or utility value linked to the learner's personal goal). The LLM composes the text from structured signals; the system never emits canned pep talk.

## Cognitive Science Engine — BKT, FSRS, IRT, PFA, KST

Five complementary learning-science algorithms run on every interaction and jointly inform the activity router. Together they form the learner-and-pedagogical model of the ITS.

| Algorithm | Role |
|-----------|------|
| **BKT** (Bayesian Knowledge Tracing) | Tracks mastery probability per concept. Distinguishes syntax errors from knowledge gaps. |
| **FSRS** (Free Spaced Repetition Scheduler) | Schedules reviews using stability/difficulty curves. Determines optimal review intervals. |
| **IRT** (Item Response Theory) | Estimates learner ability (θ) from response patterns. Calibrates exercise difficulty. |
| **PFA** (Performance Factor Analysis) | Weights success/failure history to predict performance on each concept. |
| **KST** (Knowledge Space Theory) | Validates prerequisite graphs and gates new concepts on mastery of their ancestors. |

## Regulation Pipeline (v0.3, behind feature flags)

On top of the legacy alert-and-router engine, v0.3 introduces a seven-stage **regulation pipeline** — pure functions composed by an impure orchestrator — that lifts activity selection from a priority-ladder of alerts to an explicit **phase FSM** (DIAGNOSTIC ↔ INSTRUCTION ↔ MAINTENANCE) with information-theoretic concept selection and a hygiene gate. The pipeline is active by default; each stage exposes an opt-out flag for surgical rollback.

| # | Stage | Status | What it does |
|---|-------|--------|--------------|
| **[7]** | Threshold Resolver (`algorithms/thresholds.go`) | **Shipped** (default-on) | Unifies the three historical mastery thresholds (BKT, KST gating, mid-curriculum) at 0.85. Kill switch: `REGULATION_THRESHOLD=off`. |
| **[1]** | Goal Decomposer (`tools/goal_relevance.go`) | **Shipped** (default-on) | LLM-authored relevance vector over the concept graph; biases [4] toward goal-critical concepts. Kill switch: `REGULATION_GOAL=off`. |
| **[5]** | Action Selector (`engine/action_selector.go`) | **Shipped** (default-on) | Picks the activity *type* for a chosen concept from BKT/IRT signals: `PRACTICE`, `DEBUG_MISCONCEPTION`, `FEYNMAN_PROMPT`, `TRANSFER_PROBE`, `RECALL_EXERCISE`, `MASTERY_CHALLENGE`. ZPD anchored at `IRT_θ + 0.847`. Kill switch: `REGULATION_ACTION=off` (drops only the system-prompt appendix; `REGULATION_PHASE=off` is what disables the runtime). |
| **[4]** | Concept Selector (`engine/concept_selector.go`) | **Shipped** (default-on) | Phase-aware concept choice: max-info-gain on the KST fringe (INSTRUCTION), most-overdue under FSRS (MAINTENANCE), max-binary-entropy untouched concept (DIAGNOSTIC). Kill switch: `REGULATION_CONCEPT=off` (prompt appendix only; same caveat). |
| **[3]** | Gate Controller (`engine/gate.go`) | **Shipped** (default-on) | Hygiene gate that may override the selection: anti-repeat window (no concept repeated within last *N*=3 activities), 45-min session-budget escape (`CLOSE_SESSION`), no-fringe escape (`REST`). Kill switch: `REGULATION_GATE=off` (prompt appendix only). |
| **[2]** | Phase Controller (`engine/orchestrator.go` + `engine/phase_fsm.go`) | **Shipped** (default-on) | Pure-FSM orchestrator wiring [4]→[5]→[3]. Transitions are observation-driven: DIAGNOSTIC→INSTRUCTION on entropy reduction *ΔH ≥ 0.2 bits* or *N ≥ 8* diagnostic items; INSTRUCTION→MAINTENANCE on full-graph mastery; MAINTENANCE→INSTRUCTION on FSRS retention drop. Kill switch: `REGULATION_PHASE=off` falls back to legacy `engine.Route`. |
| **[6]** | Fade Controller | **Pending** | Gradual handover of pacing decisions to the learner once autonomy thresholds are met. Not implemented yet. |

The pure functions (`SelectAction`, `SelectConcept`, `ApplyGate`, `EvaluatePhase`) are individually unit-tested (~90 dedicated tests). The orchestrator is exercised by SQLite in-memory tests and migration safety tests for the new `domains.phase`, `domains.phase_changed_at`, `domains.phase_entry_entropy` columns.

### Feature flags — default-on, opt-out via `=off`

All regulation flags are **active by default**. Setting any of them to the literal `off` disables that stage; any other value (including unset) leaves it enabled. Typos in the off-direction (`Off`, `OFF`, ` off`) leave the stage active — operators must type `off` exactly to disable, which makes accidental rollback impossible.

Only two flags change the runtime; the rest control the system-prompt appendix only. The appendix tells the LLM about the new activity types and routing semantics, but the actual selection logic ships behind `REGULATION_PHASE`.

| Flag | Default | Effect when set to `off` | Touches |
|------|---------|--------------------------|---------|
| `REGULATION_THRESHOLD` | on | Reverts to legacy split thresholds (BKT 0.85, KST 0.70, Mid 0.80). | runtime |
| `REGULATION_PHASE` | on | Routes `get_next_activity` through the legacy `engine.Route` priority cascade instead of `engine.Orchestrate`. The orchestrator already auto-falls-back to legacy on internal error; this flag is the explicit operator override. | runtime |
| `REGULATION_GOAL` | on | Hides `set_goal_relevance` / `get_goal_relevance` from the MCP tool list and drops the goal-aware system-prompt section. | runtime + prompt |
| `REGULATION_ACTION` | on | Drops the action-selector appendix (the LLM no longer sees the new activity types documented). The selector itself keeps running under `REGULATION_PHASE`. | prompt only |
| `REGULATION_CONCEPT` | on | Drops the concept-selector appendix. Selector keeps running. | prompt only |
| `REGULATION_GATE` | on | Drops the gate appendix. Gate keeps running. | prompt only |

## MCP Tools (30)

### Core Learning

| Tool | Description |
|------|-------------|
| `get_learner_context` | Session context: active domain, concept states, recent history, active misconceptions |
| `get_pending_alerts` | Critical alerts requiring immediate action |
| `get_next_activity` | Next optimal activity + metacognitive mirror + tutor mode + motivation brief |
| `record_interaction` | Log result; updates BKT/FSRS/IRT/PFA; tracks hints, initiative, proactive reviews, error type, misconception type/detail |
| `check_mastery` | Check if a concept is eligible for a mastery challenge |
| `get_cockpit_state` | Full dashboard: progress, retention, autonomy score, calibration bias, affect history |
| `open_cockpit` | Returns an MCP resource link that opens the cockpit UI inside compatible clients (Claude Desktop, claude.ai) |
| `pick_concept` | Pin a specific concept as the next focus, overriding the router for one activity |
| `get_olm_snapshot` | Open Learner Model snapshot: per-concept mastery, retention, last-seen, fringe membership, anti-repeat status |
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
| `set_goal_relevance` | LLM-decomposed goal-relevance vector over the concept graph (0 = orthogonal, 1 = goal-critical) — biases the concept selector toward what matters for the learner's stated objective. Disabled by `REGULATION_GOAL=off`. |
| `get_goal_relevance` | Read current goal-relevance vector with staleness flags (graph-version drift, uncovered concepts). Disabled by `REGULATION_GOAL=off`. |

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
├── algorithms/      BKT, FSRS, IRT, KST, PFA + thresholds + BKT info-gain (all with tests)
│   ├── bkt.go / fsrs.go / irt.go / kst.go / pfa.go   Five core learning-science algorithms
│   ├── thresholds.go                                  Unified 0.85 mastery threshold (REGULATION_THRESHOLD)
│   └── bkt_info_gain.go                               Binary entropy + info-gain helpers for [4] concept selector
├── engine/
│   ├── alert.go                Learning + metacognitive alert computation
│   ├── router.go               Legacy activity routing with priority-based alert handling
│   ├── metacognition.go        Autonomy score, mirror detection, tutor mode
│   ├── motivation.go           Brief selection + composition (6 kinds, Hidi-Renninger phase)
│   ├── scheduler.go            Cron jobs: critical alerts, reviews, queued nudges, cleanup
│   ├── olm.go / olm_graph.go   Open Learner Model snapshot (per-concept + global)
│   ├── action_selector.go      [5] Pure activity-type selection (PRACTICE / DEBUG_MISCONCEPTION / …)
│   ├── concept_selector.go     [4] Pure phase-aware concept selection (info-gain, FSRS overdue, max-entropy)
│   ├── gate.go                 [3] Pure hygiene gate (anti-repeat, session-budget, no-fringe escape)
│   ├── phase_fsm.go            [2] Pure phase FSM (DIAGNOSTIC ↔ INSTRUCTION ↔ MAINTENANCE)
│   ├── phase_config.go         [2] Tunable thresholds (ΔH, N_diagnostic_max, anti-repeat window, …)
│   └── orchestrator.go         [2] Impure coordinator wiring [4]→[5]→[3] with FSM transitions
├── models/
│   ├── learner.go              Learner, ConceptState, Interaction, RefreshToken
│   ├── domain.go               AlertType, Activity, KnowledgeSpace, Domain (+ phase fields, value framings)
│   ├── metacognition.go        AffectState, CalibrationRecord, MirrorMessage, AutonomyMetrics
│   ├── motivation.go           MotivationBrief, RecapBrief, WebhookQueueItem, interest phases
│   └── regulation.go           Phase enum (DIAGNOSTIC / INSTRUCTION / MAINTENANCE)
├── db/
│   ├── store.go                     SQLite persistence: learners, domains, concepts, interactions
│   ├── metacognition.go             Affect, calibration, transfer, autonomy queries
│   ├── misconceptions.go            Misconception aggregation and status tracking
│   ├── motivation_queries.go        Brief-engine signals (failures, session counts, self-init ratio)
│   ├── implementation_intentions.go If-then commitments (Gollwitzer)
│   ├── webhook_queue.go             LLM-authored nudge queue with dequeue/dedup/expire
│   ├── goal_relevance.go            Goal-relevance vector storage + staleness checks
│   ├── phase.go                     Phase transitions, action history, anti-repeat queries
│   ├── schema.sql                   Table definitions (embedded)
│   └── migrations.go                Idempotent migrations for existing databases
└── tools/                30 MCP tool handlers + system prompt + flag-gated appendices
```

## Language conventions

The codebase mixes English and French on purpose. The split is along a single
axis: **who reads the string?**

- **English** — everything developers and operators read: source code,
  identifiers, comments, log messages, error returns surfaced to clients/CI,
  this README and every doc under `docs/`. Keep `slog` calls, panics, and Go
  errors in English.
- **French** — everything the **learner** ends up seeing, directly or via the
  LLM's routing/prompting layer:
  - MCP tool descriptions and `jsonschema` parameter descriptions
    (`tools/*.go`) — the LLM uses these to choose which tool to call, and
    paraphrases them back to the learner.
  - The system prompt (`tools/prompt.go`).
  - OLM (open learner model) messages (`engine/olm.go`).
  - Rationales the orchestrator surfaces to the learner
    (`engine/orchestrator.go`).
  - DB-stored literal strings that flow through to learner-facing UI
    (`db/store.go`).

When in doubt: if a string is destined for the learner, write it in correct
French with full diacritics (`é`, `è`, `à`, `ç`, `ô`, `î`, …). The lint test
in `tools/registration_test.go` enforces diacritic consistency across tool
descriptions; widen it if you add new learner-facing surfaces.

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
- Wide test coverage across the five algorithms, the regulation pipeline (action / concept / gate / phase-FSM / orchestrator), legacy router and alert engine, motivation selection, misconception aggregation, OLM snapshots, goal-relevance staleness, and schema migrations

## Operations

For database backup, restore, off-host copy and service control recipes, see [OPERATIONS.md](./OPERATIONS.md). Single-user systemd-user setup is documented end-to-end.

## License

MIT — see [LICENSE](./LICENSE).

## Author

**Arnaud Guiovanna**
- Website: [aguiovanna.fr](https://www.aguiovanna.fr)
- GitHub: [@ArnaudGuiovanna](https://github.com/ArnaudGuiovanna)
