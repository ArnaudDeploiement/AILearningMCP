# [1] GoalDecomposer — Design (Phase 1)

> Composant 2/7 du pipeline de régulation. Produit le vecteur
> `goal_relevance: map[concept]float64 ∈ [0,1]` qui matérialise la
> promesse README *« calibrated in real time to the learner's mastery,
> ability, affect, and personal goal »* (F-4.5).
>
> Référence architecture : `docs/regulation-architecture.md` §3 [1],
> §6 Q1, §6 Q2.

---

## 1. Nature du composant

Le runtime **ne génère pas** le vecteur `goal_relevance` (cohérent avec
le garde-fou « LLM = content engine »). Le composant `[1]` est :

1. Un **schéma de persistance** (colonne JSON versionnée sur `domains`).
2. Un **outil MCP** `set_goal_relevance` que le LLM appelle pour écrire
   le vecteur après avoir lu le `personal_goal` et la liste de concepts.
3. Un **accessor de lecture** côté runtime (`Domain.GoalRelevance()`)
   avec **fallback uniforme** quand le vecteur est absent ou stale.
4. Une **instruction dans la réponse de `init_domain` et `add_concepts`**
   qui indique au LLM d'appeler `set_goal_relevance` (asynchrone, non
   bloquant — Q2 = B).

Pas de signal cognitif consommé. Pas de boucle de décision propre. Le
composant est *amont* du pipeline de régulation : il pose un signal
qui sera lu par `[4] ConceptSelector` quand celui-ci sera implémenté.

### Pourquoi ce composant en deuxième

La séquence d'implémentation `7 → 1 → 5 → 4 → 3 → 2 → 6` (validée Q6)
place `[1]` après `[7]` et avant `[5]`. Raison :

- `[5] ActionSelector` n'a pas besoin de `goal_relevance` (il décide un
  type d'activité étant donné un concept déjà choisi).
- `[4] ConceptSelector` consomme `goal_relevance`. Il a besoin du
  signal pour fonctionner.
- En implémentant `[1]` avant `[4]`, on dispose du signal en prod
  pendant ~3-4 PRs avant qu'il ait un consommateur. Ça permet
  d'observer le comportement LLM (les valeurs envoyées,
  l'idempotence, l'absence d'appel) en conditions réelles avant que
  ces données nourrissent une décision.

---

## 2. Signal consommé

| Source | Champ | Localisation actuelle |
|--------|-------|------------------------|
| `Domain.PersonalGoal` | texte libre, ≤ 2000 chars (cap existant) | `models/domain.go:73`, persisté `domains.personal_goal` (`db/schema.sql`) |
| `Domain.Graph.Concepts` | `[]string`, ≤ 500 concepts (cap existant) | `models/domain.go:74`, persisté `domains.graph_json` |

Aucun nouveau signal cognitif. Aucune dépendance aux autres composants.

## 3. Décision produite

```go
type GoalRelevance struct {
    ForGraphVersion int                `json:"for_graph_version"`
    Relevance       map[string]float64 `json:"relevance"` // concept → score ∈ [0,1]
    SetAt           time.Time          `json:"set_at"`
}
```

- `Relevance` : pour chaque concept du domaine, score de pertinence
  vis-à-vis du `personal_goal`. 1.0 = goal-critique, 0.0 = orthogonal.
  Concepts manquants → fallback uniforme (= 1.0) à la lecture.
- `ForGraphVersion` : numéro de version du graphe au moment du set.
  Permet de détecter si le vecteur est devenu stale (un `add_concepts`
  ultérieur a augmenté la version du graphe).
- `SetAt` : horodatage UTC du dernier set, pour observabilité.

### Signature des accessors

```go
// Côté store (db/store.go ou nouveau db/goal_relevance.go) :
func (s *Store) UpdateDomainGoalRelevance(domainID string, rel map[string]float64) error
func (s *Store) GetDomainGoalRelevance(domainID string) (*models.GoalRelevance, error)

// Côté domain model (helper sur Domain) :
func (d *Domain) GoalRelevance() map[string]float64
//   - Returns nil si JSON vide ou parse fails (silent fallback).
//   - Caller treats nil as "all concepts uniform = 1.0".

// Côté domain model (helper de staleness) :
func (d *Domain) IsGoalRelevanceStale() bool
//   - True si GoalRelevanceVersion < GraphVersion.
```

---

## 4. Outil MCP `set_goal_relevance`

Nouveau handler dans `tools/goal_relevance.go`.

### Schéma d'entrée

```go
type SetGoalRelevanceParams struct {
    DomainID  string             `json:"domain_id,omitempty" jsonschema:"ID du domaine cible (optionnel)"`
    Relevance map[string]float64 `json:"relevance" jsonschema:"Map concept → score [0,1]. Concepts manquants traités comme uniformes."`
}
```

### Schéma de sortie

```json
{
  "domain_id": "dom_abc",
  "for_graph_version": 3,
  "concepts_set": 14,
  "concepts_clamped": 1,        // valeurs hors [0,1] ramenées à la borne
  "concepts_unknown_ignored": 0, // valeurs pour des concepts hors graph (silencieusement ignorées)
  "stale_after_set": false       // true si le graph_version a entre-temps avancé
}
```

### Algorithme du handler

```
1. Auth → learner_id
2. Resolve domain (`resolveDomain`, comme tous les autres tools)
3. Si REGULATION_GOAL != "on" → return error "feature flag disabled"
4. Validate input :
   - len(Relevance) ≤ 500 (cap concepts)
   - Pour chaque (k, v) : k non-vide, len(k) ≤ 200
5. Filter + clamp :
   - Concepts non présents dans Domain.Graph.Concepts → ignorer (compté)
   - Valeurs hors [0,1] → clamp (compté)
6. Persist :
   - tx := store.Begin()
   - Construire JSON {for_graph_version: domain.GraphVersion,
     relevance: filtered_clamped, set_at: now}
   - UPDATE domains SET goal_relevance_json = ?, goal_relevance_version =
     goal_relevance_version + 1 WHERE id = ?
   - tx.Commit()
7. Return response payload (avec stale_after_set calculé : si
   pendant la tx un autre client a appelé add_concepts, GraphVersion a
   pu avancer — peu probable mais possible)
```

### Description outil (system prompt)

À ajouter dans `tools/prompt.go` :

> `set_goal_relevance(domain_id, relevance)` — **Appelle ce tool une
> fois après `init_domain` ou `add_concepts`** pour décomposer le
> `personal_goal` du learner contre la liste des concepts du domaine.
> Pour chaque concept, fournis un score ∈ [0,1] : 1.0 si le concept
> est central au goal de l'apprenant, 0.0 s'il est orthogonal. Le
> runtime utilisera ce vecteur pour prioriser les concepts goal-relevants
> dans le routage. Si tu n'appelles pas ce tool, le système supposera
> tous les concepts également pertinents.

---

## 5. Persistance — schéma et migration

### Colonnes ajoutées à `domains`

```sql
ALTER TABLE domains ADD COLUMN graph_version INTEGER NOT NULL DEFAULT 1;
ALTER TABLE domains ADD COLUMN goal_relevance_json TEXT NOT NULL DEFAULT '';
ALTER TABLE domains ADD COLUMN goal_relevance_version INTEGER NOT NULL DEFAULT 0;
```

### Migration semantics

- **`graph_version`** :
  - Initialisé à 1 sur les domaines existants.
  - Incrémenté à `init_domain` (création → 1) et `add_concepts`
    (incrément +1).
- **`goal_relevance_json`** :
  - `''` (vide) sur les domaines existants → lecture retourne `nil` →
    accessor renvoie fallback uniforme. Comportement identique à
    pré-PR.
- **`goal_relevance_version`** :
  - 0 sur les domaines existants → est < 1 (graph_version par défaut)
    → `IsGoalRelevanceStale()` retourne `true`. Cohérent : ces
    domaines n'ont jamais eu de décomposition.

Migration ajoutée à `db/migrations.go` (idempotent par convention via
`_, _ = db.Exec(...)` du codebase, cf F-5.9 hors scope ici).

### Modèle Go

```go
// models/domain.go (extension)
type Domain struct {
    // ... champs existants ...
    GraphVersion           int       `json:"graph_version"`
    GoalRelevanceJSON      string    `json:"-"`  // raw, parsed via helper
    GoalRelevanceVersion   int       `json:"goal_relevance_version"`
}

type GoalRelevance struct {
    ForGraphVersion int                `json:"for_graph_version"`
    Relevance       map[string]float64 `json:"relevance"`
    SetAt           time.Time          `json:"set_at"`
}
```

---

## 6. Comportement async + fallback uniforme + replacement à chaud (Q2)

### Async

- `init_domain` retourne **immédiatement**.
- La réponse JSON contient un champ `next_action` dirigé au LLM :
  ```json
  {
    "domain_id": "...",
    "concept_count": 14,
    "message": "...",
    "next_action": {
      "tool": "set_goal_relevance",
      "reason": "Décompose le personal_goal contre les 14 concepts pour activer le goal-aware routing.",
      "required": false
    }
  }
  ```
- `required: false` matérialise l'async : le LLM peut zapper sans
  casser la première session.

### Fallback uniforme

Quand `goal_relevance` est absent (`nil`) ou stale (graph version
ancienne) :

```go
// Pseudocode pour [4] ConceptSelector (à implémenter plus tard) :
relevance := domain.GoalRelevance()
if relevance == nil {
    // Fallback : 1.0 partout. Tous les concepts considérés également
    // goal-relevants. Le ConceptSelector dégénère en "argmax(1 - mastery)"
    // sur la frange.
    return uniformScore(1.0)
}
score := relevance[concept]
if !ok { score = 1.0 } // concepts non décomposés → uniform
return score
```

Cohérence : avant l'implémentation de `[4]`, le fallback uniforme est
le comportement actuel (concepts indifférenciés). Pas de régression.

### Replacement à chaud

- Chaque `set_goal_relevance` réécrit le JSON et incrémente
  `goal_relevance_version`.
- Pas de verrou : SQLite WAL sérialise l'UPDATE.
- La prochaine lecture retourne le nouveau vecteur. Pas de cache
  in-memory à invalider.
- Cohérent avec F-5.1 (lost-update sur record_interaction est un
  problème distinct dans la dette différée — pour set_goal_relevance,
  un seul UPDATE atomique suffit, pas de read-modify-write).

---

## 7. Cas dégénérés

| Cas | Comportement | Garantie |
|-----|---------------|----------|
| `personal_goal` vide à `init_domain` | `next_action.reason` adapté : « Goal vide — décomposition optionnelle, ignorer ce tool ». Tool reste appelable mais traité comme ON noop. | Pas d'instruction ambiguë au LLM. |
| LLM n'appelle jamais `set_goal_relevance` | `goal_relevance_json` reste vide → fallback uniforme indéfini → comportement identique à pré-PR. | Aucune dégradation. |
| LLM envoie un score = -0.3 ou 1.7 | Server clamp à [0,1], compté dans `concepts_clamped`. | Pas de NaN propagé. |
| LLM envoie un concept qui n'existe pas dans `Domain.Graph.Concepts` | Silencieusement ignoré, compté dans `concepts_unknown_ignored`. | Robustesse à la dérive LLM. |
| LLM envoie un score pour seulement 3 concepts sur 14 | Les 3 sont stockés, les 11 autres absents → fallback 1.0 à la lecture. | Pas de règle « tout ou rien ». |
| LLM envoie 1000 concepts (synthèse fictive) | Validation len ≤ 500 → erreur. | Cap respecté (cohérent `tools/domain.go`). |
| Domain avec 500 concepts, JSON ~10 KB | OK, sous le cap. | Pas de pression schema. |
| 2 appels `set_goal_relevance` simultanés | Last-writer-wins via UPDATE atomique. | Acceptable, idempotent jusqu'au bruit de race. |
| `add_concepts` après `set_goal_relevance` | `graph_version` avance, `goal_relevance_version` reste, donc stale. Lecture renvoie l'ancien vecteur (les nouveaux concepts → fallback 1.0 individuel). `IsGoalRelevanceStale()` retourne true. | Pas de plantage. Re-décomposition à la discrétion du LLM. |
| Goal de 2000 chars + 500 concepts | LLM doit produire 500 paires (k,v). Faisable en un tool call mais lourd. Si timeout, l'apprenant garde fallback uniforme. | Pas de mode dégradé silencieux. |
| LLM appelle avec `relevance: nil` | Validation refuse (len(nil) == 0 → erreur "empty relevance map"). | Sémantique « set with no data » non autorisée. |

---

## 8. Stratégie de test

### 8.1 Unit — store

```go
// db/goal_relevance_test.go
TestStore_UpdateDomainGoalRelevance_PersistsJSON
TestStore_GetDomainGoalRelevance_RoundtripWithVersion
TestStore_GetDomainGoalRelevance_EmptyReturnsNil
TestStore_UpdateDomainGoalRelevance_IncrementsVersion
TestStore_UpdateDomainGoalRelevance_AddConceptsBumpsGraphVersion
TestStore_IsGoalRelevanceStale_TrueAfterAddConcepts
```

### 8.2 Unit — model accessor

```go
// models/domain_goal_relevance_test.go
TestDomain_GoalRelevance_NilWhenJSONEmpty
TestDomain_GoalRelevance_ParsesValidJSON
TestDomain_GoalRelevance_NilOnMalformedJSON  // silent fallback
TestDomain_IsGoalRelevanceStale_VersionComparison
```

### 8.3 Integration — MCP tool

```go
// tools/goal_relevance_test.go
TestSetGoalRelevance_Roundtrip
TestSetGoalRelevance_ClampsOutOfRange
TestSetGoalRelevance_IgnoresUnknownConcepts
TestSetGoalRelevance_PartialOverridesFallback
TestSetGoalRelevance_FlagOff_RejectsCall
TestSetGoalRelevance_EmptyMapRejected
TestSetGoalRelevance_OverwritesPriorVector
TestSetGoalRelevance_ConcurrentLastWriterWins
```

### 8.4 Integration — init_domain interaction

```go
TestInitDomain_ResponseIncludesNextActionWhenFlagOn
TestInitDomain_NoNextActionWhenFlagOff
TestInitDomain_NextActionEmptyGoalIsOptionalHint
TestAddConcepts_BumpsGraphVersion
```

### 8.5 Régression — fallback uniforme avant `[4]`

Aucun test runtime n'utilise `goal_relevance` aujourd'hui (pas de
consommateur). La PR ne doit pas casser de test existant. Vérification :
`go test ./...` PASS sans flag (le tool `set_goal_relevance` n'est pas
enregistré → invisible aux tests qui n'en parlent pas).

### 8.6 Pas de fixture-shift sur les tests existants

Comme pour `[7]` : on n'altère pas les fixtures de tests existants.
Toutes les nouvelles assertions vivent dans des fichiers `*_test.go`
nouveaux ou dans des sous-tests dédiés.

---

## 9. Interaction avec les composants amont/aval

### Amont

- `init_domain` (tools/domain.go) : modifié pour incrémenter
  `graph_version` à 1 et inclure `next_action` dans la réponse si
  `REGULATION_GOAL=on`.
- `add_concepts` (tools/domain.go) : modifié pour incrémenter
  `graph_version` (+1).

### Aval (futurs composants)

- `[4] ConceptSelector` : consommera `domain.GoalRelevance()`. Tant
  que `[4]` n'est pas implémenté, le signal est dormant.
- `tools/cockpit.go` (`get_cockpit_state`) : peut surfacer
  `goal_relevance_version` et un flag `goal_relevance_stale` pour
  observabilité — **pas dans cette PR**, à ouvrir comme issue
  dérivée si demandé.
- `tools/context.go` (`get_learner_context`) : idem, observabilité
  optionnelle, hors PR.

### Pas d'interaction avec [7]

Les seuils et la pertinence-au-goal sont des dimensions orthogonales.
`[1]` peut tourner indépendamment du profil `MasteryThreshold`.

---

## 10. Décisions ouvertes spécifiques au composant

### OQ-1.1 — Doit-on bloquer `add_concepts` quand `goal_relevance` est non-vide ?

**A.** Permettre `add_concepts` librement ; le vecteur devient stale, le
LLM peut re-set quand il veut. État stale documenté via
`IsGoalRelevanceStale()` et exposé en cockpit (futur).

**B.** Refuser `add_concepts` tant que `set_goal_relevance` n'a pas été
re-appelé pour le nouveau graphe. Force le LLM à toujours maintenir le
vecteur synchronisé.

**Mon défaut** : A. Cohérent avec Q2 (async, non bloquant). Refuser une
opération sur un état périphérique est trop strict. Le coût d'un vecteur
stale est limité au fallback uniforme sur les nouveaux concepts —
acceptable.

### OQ-1.2 — Le tool retourne-t-il un signal de staleness ?

**A.** `set_goal_relevance` retourne `stale_after_set: false` (ou true
en cas de race rare). Le cockpit/context expose `goal_relevance_stale`
sur lecture. Visible mais pas obtrusif.

**B.** Pas de signal. Le LLM doit lire le cockpit ou la doc pour
comprendre la staleness.

**Mon défaut** : A. Coût implementation minime, valeur observabilité
forte. Le LLM peut décider de re-set sur un signal explicit plutôt
que de faire de la magie.

### OQ-1.3 — `next_action` dans la réponse de `init_domain` : structuré ou texte libre ?

**A.** Champ structuré `next_action: {tool: "set_goal_relevance",
reason: "...", required: false}`. Lisible programmatiquement, le LLM
peut le parser.

**B.** Texte libre dans `message: "Domaine créé. Pense à appeler
set_goal_relevance pour activer le goal-aware routing."` Simple, mais
fragile si on traduit ou si on ajoute d'autres next-actions plus tard.

**Mon défaut** : A. Le champ structuré scale (on peut imaginer
`next_action: [{...}, {...}]` plus tard pour multi-actions). Et c'est
plus testable.

### OQ-1.4 — Format input map vs liste ?

**A.** `Relevance map[string]float64 {"concept_a": 0.9, ...}`. JSON map
naturelle, compacte.

**B.** `Relevance []struct{Concept string; Score float64}`. Plus
verbeux mais ordre préservé, et permet de futures extensions
(`Concept, Score, Confidence float64` par exemple).

**Mon défaut** : A. Map = simple à générer pour le LLM, simple à
parser côté serveur. Si on veut plus tard ajouter des champs (genre
`confidence`), on changera la valeur en struct (`map[string]struct{Score float64; Confidence float64}`)
sans changer la forme map.

---

## 11. Plan de PR

### 11.1 Fichiers touchés

| Action | Fichier | Notes |
|--------|---------|-------|
| **Création** | `tools/goal_relevance.go` | handler MCP `set_goal_relevance` |
| **Création** | `tools/goal_relevance_test.go` | ~150 lignes |
| **Création** | `db/goal_relevance.go` | `UpdateDomainGoalRelevance`, `GetDomainGoalRelevance` |
| **Création** | `db/goal_relevance_test.go` | round-trip, version, stale |
| **Modif** | `models/domain.go` | ajouter 3 champs + `GoalRelevance()`, `IsGoalRelevanceStale()` |
| **Modif** | `db/migrations.go` | 3 ALTER TABLE idempotents |
| **Modif** | `db/store.go` | `CreateDomain`, `UpdateDomainGraph` incrémentent `graph_version` |
| **Modif** | `tools/domain.go` | `init_domain` + `add_concepts` retournent `next_action` quand `REGULATION_GOAL=on` |
| **Modif** | `tools/tools.go` | enregistrer `set_goal_relevance` (gated par flag) |
| **Modif** | `tools/prompt.go` | documenter l'outil quand le flag est on |
| **Modif** | `db/schema.sql` | ajouter colonnes au CREATE TABLE pour les fresh installs |

### 11.2 Critères de merge

- [ ] `go test ./...` PASS sans flag (REGULATION_GOAL non défini)
- [ ] `REGULATION_GOAL=on go test ./...` PASS
- [ ] Migration appliquée idempotente sur DB existante (test migrations)
- [ ] Tool `set_goal_relevance` invisible dans la liste tools si flag off
- [ ] `init_domain` répond identique à pré-PR si flag off (no `next_action`)
- [ ] Doc design citée dans le commit message

### 11.3 Pas inclus dans cette PR (et pourquoi)

- Pas de consommateur runtime du vecteur (`[4] ConceptSelector` reste
  dormant). Le signal est *écrit* mais pas *lu* par les décisions.
  C'est l'objectif Q6 : observer le comportement LLM avant de brancher.
- Pas de surfaces d'observabilité dans cockpit/context (OQ-1.2 défaut
  A le mentionne mais reporte). Issue dérivée si demandée.
- Pas de validation cyclique du graphe (F-4.6 reste dans la dette
  différée).

### 11.4 Test d'intégration goal-aware (réservé pour PR `[4]`)

Conformément à la décision Q6 : *un apprenant simulé sur 20 sessions
avec `goal_relevance` non-uniforme produit une trajectoire sensiblement
différente d'un apprenant identique avec relevance uniforme. Sinon,
signal mort, on stoppe avant `[3]`.* Ce test ne peut pas exister tant
que `[4] ConceptSelector` n'est pas implémenté (pas de consommateur).
Sera ajouté dans la PR `[4]`.

---

## 12. Récapitulatif

| Aspect | Décision |
|--------|----------|
| **API** | 1 tool MCP (`set_goal_relevance`), 2 helpers store, 2 helpers model |
| **Persistance** | colonne JSON `goal_relevance_json` + 2 colonnes `int` (graph_version, goal_relevance_version) sur `domains` |
| **Flag** | `REGULATION_GOAL=on` gate registration du tool + injection de `next_action` |
| **Async** | Q2=B : init_domain retourne immédiatement, instruction non bloquante |
| **Fallback** | uniforme (1.0 partout) si JSON vide, parse error ou stale |
| **Hot-swap** | UPDATE atomique, pas de cache, last-writer-wins sur race |
| **Findings résolus** | F-4.5 (sous flag ON, **et** sous condition `[4]` implémenté pour que le signal soit lu) |
| **Tests** | ~6 fichiers de test, ~400 lignes |

---

**STOP.** Design `[1] GoalDecomposer` complet. En attente de validation,
ou amendements sur les 4 décisions ouvertes (OQ-1.1 add_concepts
blocking, OQ-1.2 staleness signal, OQ-1.3 next_action structuré,
OQ-1.4 map vs liste). Composant suivant après validation+implémentation :
`[5] ActionSelector`.
