# Agentic Context Register

This file is important, read all of it.

## 1. Zettelkasten Availability

This repository utilizes a **Zettelkasten knowledge base**, an abstracted
documentation system focusing on "the why" and "the what" of the codebase rather
than raw implementation details. It captures atomic, intent-driven concepts to
help you understand the architectural decisions and patterns.

A zettel looks like this:

```
YYYYMMDDHHMM-slug.md        ← filename
# Declarative Title          ← H1 heading
                             ← opening paragraph (summary of the main idea)
                             ← body (continuous prose with contextual links)
Tags: #tag1 #tag2            ← single Tags line (enables set operations via grep)
## References                ← labeled lateral links
## Backlinks                 ← machine-managed (never edit by hand)
```

**Location:** The Zettelkasten is located at `zk/`. All file paths and links in
this document are relative to the execution directory.

This Zettelkasten federates knowledge from the following external sources. To
resolve these links locally, you must fetch them into the corresponding
namespace directories.

**Before reading any federated zettel, verify its namespace directory exists. If
it does not, run the corresponding materialization script below.**

### Namespace: `bolts`

```bash
if [ ! -d "zk/bolts" ]; then
  git clone --no-checkout --depth 1 --sparse -b zk github.com/bitromortac/bolts /tmp/temp_bolts
  cd /tmp/temp_bolts && git sparse-checkout set zk && git checkout && cd -
  cp -r /tmp/temp_bolts/zk zk/bolts
else
  echo "Namespace 'zk/bolts' already exists. Skipping materialization."
fi
```

### Namespace: `zk-dev`

```bash
if [ ! -d "zk/zk-dev" ]; then
  git clone --depth 1 github.com/bitromortac/zk-dev /tmp/temp_zk-dev
  cp -r /tmp/temp_zk-dev zk/zk-dev
else
  echo "Namespace 'zk/zk-dev' already exists. Skipping materialization."
fi
```

### Namespace: `zk-spec`

```bash
if [ ! -d "zk/zk-spec" ]; then
  git clone --depth 1 github.com/bitromortac/zk-spec /tmp/temp_zk-spec
  cp -r /tmp/temp_zk-spec zk/zk-spec
else
  echo "Namespace 'zk/zk-spec' already exists. Skipping materialization."
fi
```

## 2. Capabilities at Your Disposal (Graph Queries)

When specialized tooling (`zktool`) is missing, emulate graph operations with
standard Unix tools like `grep`.

### Discovery (Finding Terms)

If you are unsure what concepts exist or need to find the correct terminology,
browse the tags or extract titles.
- **Extracting Titles:** `grep -l "#tag" zk/**/*.md | xargs grep -m 1 "^# "`
- **Tag Vocabulary:** Review the available tags to identify the right
  terminology:

```
#ai (6)
  [Related: #zettelkasten (5), #productivity (2), #writing (2), #automation (1), #content-generation (1)]
#ai-interaction (6)
  [Related: #workflow (2), #ambiguity (1), #best-practices (1), #communication (1), #efficiency (1)]
#architecture (31)
  [Related: #invoices (8), #htlc (7), #lightning-network (7), #on-chain (7), #security (5)]
#automation (4)
  [Related: #architecture (2), #zettelkasten (2), #ai (1), #code-review (1), #graph-theory (1)]
#best-practices (13)
  [Related: #zettelkasten (6), #standards (4), #writing (4), #entry-point (3), #formatting (3)]
#clean-code (14)
  [Related: #formatting (6), #golang (6), #readability (6), #example (4), #logging (3)]
#code-review (2)
  [Related: #architecture (1), #automation (1), #human-ai-collaboration (1), #zettel-coding (1), #zettelkasten (1)]
#database (2)
  [Related: #architecture (2), #storage (2), #channel-state (1), #invoices (1)]
#design-decision (2)
  [Related: #graph-theory (2), #zettelkasten (2), #best-practices (1)]
#diagram (4)
  [Related: #entry-point (3), #architecture (2), #best-practices (1), #dispute-resolution (1), #formatting (1)]
#dispute-resolution (5)
  [Related: #architecture (4), #on-chain (4), #diagram (1), #entry-point (1), #htlc (1)]
#documentation (3)
  [Related: #clean-code (2), #best-practices (1), #entry-point (1), #golang (1), #readability (1)]
#entry-point (12)
  [Related: #architecture (3), #best-practices (3), #diagram (3), #zettelkasten (3), #maintenance (2)]
#example (6)
  [Related: #clean-code (4), #formatting (4), #golang (3), #logging (2), #readability (2)]
#formatting (11)
  [Related: #clean-code (6), #readability (6), #example (4), #best-practices (3), #golang (2)]
#git (4)
  [Related: #workflow (3), #clean-code (1), #entry-point (1), #process (1), #security (1)]
#golang (10)
  [Related: #clean-code (6), #example (3), #workflow (3), #formatting (2), #logging (2)]
#graph-theory (6)
  [Related: #zettelkasten (4), #design-decision (2), #pathfinding (2), #routing (2), #ai (1)]
#htlc (10)
  [Related: #invoices (9), #architecture (7), #lightning-network (4), #settlement (2), #dispute-resolution (1)]
#invoices (12)
  [Related: #htlc (9), #architecture (8), #lightning-network (5), #rpc (3), #settlement (2)]
#knowledge-management (3)
  [Related: #zettelkasten (2), #ai (1), #graph-theory (1), #modular-design (1), #zettel-coding (1)]
#lightning-network (9)
  [Related: #architecture (7), #invoices (5), #htlc (4), #automation (1), #daemon (1)]
#logging (3)
  [Related: #clean-code (3), #example (2), #golang (2), #formatting (1)]
#maintenance (2)
  [Related: #entry-point (2), #zettelkasten (2), #best-practices (1), #clean-zettel (1), #refactoring (1)]
#methodology (2)
  [Related: #zettelkasten (2), #naming (1), #productivity (1), #software-architecture (1), #zettel-coding (1)]
#navigation (2)
  [Related: #zettelkasten (2), #best-practices (1), #sorting (1), #style (1), #ux (1)]
#networking (4)
  [Related: #architecture (2), #zettelkasten (2), #multiplexing (1), #protocol (1), #security (1)]
#on-chain (7)
  [Related: #architecture (7), #dispute-resolution (4), #diagram (1), #entry-point (1), #security (1)]
#pathfinding (4)
  [Related: #routing (4), #graph-theory (2), #architecture (1), #diagram (1), #entry-point (1)]
#payment (4)
  [Related: #routing (3), #architecture (2), #algorithm (1), #htlc (1), #pathfinding (1)]
#planning (2)
  [Related: #best-practices (2), #ai-interaction (1), #entry-point (1), #process (1), #protocol (1)]
#privacy (2)
  [Related: #routing (2), #architecture (1), #blinded-paths (1), #cryptography (1)]
#process (3)
  [Related: #workflow (2), #best-practices (1), #entry-point (1), #git (1), #golang (1)]
#productivity (4)
  [Related: #zettelkasten (4), #ai (2), #methodology (1), #naming (1), #rag (1)]
#protocol (2)
  [Related: #ai-interaction (1), #architecture (1), #best-practices (1), #multiplexing (1), #networking (1)]
#readability (9)
  [Related: #clean-code (6), #formatting (6), #best-practices (2), #example (2), #golang (2)]
#refactoring (3)
  [Related: #zettelkasten (2), #analogy (1), #best-practices (1), #clean-code (1), #complexity (1)]
#routing (9)
  [Related: #pathfinding (4), #architecture (3), #payment (3), #graph-theory (2), #privacy (2)]
#rpc (4)
  [Related: #invoices (3), #architecture (2), #authentication (1), #htlc (1), #lightning-network (1)]
#security (6)
  [Related: #architecture (5), #authentication (1), #diagram (1), #entry-point (1), #git (1)]
#settlement (2) [Related: #architecture (2), #htlc (2), #invoices (2)]
#software-architecture (2)
  [Related: #architecture (1), #diagram#entry-point (1), #lightning-network (1), #methodology (1), #zettel-coding (1)]
#standard (2)
  [Related: #best-practices (2), #entry-point (1), #planning (1), #process (1), #style (1)]
#standards (4)
  [Related: #best-practices (4), #writing (3), #formatting (2), #readability (2), #linter (1)]
#state-machine (3)
  [Related: #architecture (2), #invoices (2), #channel-establishment (1), #channel-management (1), #htlc (1)]
#storage (2)
  [Related: #architecture (2), #database (2), #channel-state (1), #invoices (1)]
#style (2)
  [Related: #best-practices (2), #zettelkasten (2), #navigation (1), #standard (1), #writing (1)]
#testing (2) [Related: #golang (2), #clean-code (1), #workflow (1)]
#tools (3)
  [Related: #formatting (2), #clean-code (1), #golang (1), #llformat (1), #readability (1)]
#workflow (10)
  [Related: #git (3), #golang (3), #ai-interaction (2), #entry-point (2), #process (2)]
#writing (6)
  [Related: #best-practices (4), #zettelkasten (4), #standards (3), #ai (2), #formatting (2)]
#zettel-coding (4)
  [Related: #ai (1), #code-review (1), #human-ai-collaboration (1), #human-computer-interaction (1), #knowledge-management (1)]
#zettelkasten (25)
  [Related: #best-practices (6), #ai (5), #graph-theory (4), #productivity (4), #writing (4)]
```

### Topical Search (Building Context)

When you need to understand a specific domain, search for notes by tag. Always
search across all namespaces using recursive globs (`zk/**/*.md`).
- **Tag Intersection (AND):** `grep -rl "#tag1" zk/**/*.md | xargs grep -l "#tag2"`
- **Tag Union (OR):** `grep -rlE "#tag1|#tag2" zk/**/*.md`
- **Exclusion (NOT):** `grep -rl "#tag1" zk/**/*.md | xargs grep -L "#tag2"`

### Specific Retrieval (Reading Notes)

If you see a reference to a note ID, read the full content.
- **Immediate Reading:** `grep -rl "#tag1" zk/**/*.md | xargs grep -l "#tag2" | xargs cat`
- **Direct Reading:** `cat zk/<filename_or_id>`

## 3. Interaction Best Practices

- **Zettelkasten-First:** Before doing any action, ask yourself if you can fetch
  knowledge from the zettelkasten by using grep commands.
- **Context Efficiency (Sparsity):** Avoid blind listing. Do not run `ls zk/` or
  similar commands to view all files. Always rely on targeted queries, tags, or
  core entry points. Only fetch full zettel content once you are sure they are
  relevant.
- **Prose-First Abstraction:** Zettels intentionally avoid raw code snippets to
  remain relevant across refactors. Read them for conceptual flow and logic.
- **Contextual Linking:** Pay attention to embedded links within the prose, as
  they provide semantic context and define relationships between different
  components.
- **Atomic Concepts:** Expect each note to represent a single, focused idea.

### Core Entry Points
Find system boundaries, high-level standards, and interaction protocols here.
Entries under federated namespaces require the namespace to be materialized first
(see Section 1).

- [[zk/zk-dev/202602091210-AI-Interaction-Protocols.md]]
- [[zk/zk-dev/202601141400-Codebase-standards.md]]
- [[zk/202603251003-Contract-Court-Resolution.md]]
- [[zk/zk-dev/202601281200-Developer-guides.md]]
- [[zk/zk-dev/202603171000-Git-Contribution-Conventions.md]]
- [[zk/zk-dev/202601271000-Implementation-Plan-Guidelines.md]]
- [[zk/202603250830-Invoices.md]]
- [[zk/202603181000-Lnd-Architecture.md]]
- [[zk/202603181010-Pathfinding-Router.md]]
- [[zk/202603251000-Watchtower-Architecture.md]]
- [[zk/zk-spec/202602011500-Zettel-Health.md]]
- [[zk/zk-spec/202601170934-Zettel-smells.md]]
- [[zk/zk-spec/202301021042-Zettelkasten.md]]

### High-Centrality Hubs
Reference these to locate the most interconnected atomic concepts:

- [[zk/zk-spec/202603111200-anatomy-of-a-zettel.md]] (Degree: 10)
- [[zk/zk-spec/202601021011-zettelkasten-software-isomorphism.md]] (Degree: 6)
- [[zk/202603181007-funding-manager.md]] (Degree: 5)
- [[zk/202603250837-invoice-settlement-flow.md]] (Degree: 5)
- [[zk/202603181003-lightning-wallet-abstraction.md]] (Degree: 5)
- [[zk/202603181013-payment-session-pathfinding.md]] (Degree: 5)
- [[zk/zk-spec/202603150800-80-char-width-readability.md]] (Degree: 4)
- [[zk/202603181015-blinded-paths-privacy.md]] (Degree: 4)
- [[zk/202603181004-channel-state-database.md]] (Degree: 4)
- [[zk/zk-spec/202603171101-entry-point-parent-references.md]] (Degree: 4)

## 4. Usage Examples

**Scenario: Understanding Go Coding Standards**
*Context:* You need to implement a new feature and want to follow project
conventions.
*Action:* `cat zk/zk-dev/202601141400-Codebase-standards.md`
*Result:* You find links to all coding standards: function signatures,
docstrings, paragraphing, nesting limits, and line width rules.

**Scenario: Zettel Best Practices (Excluding Code Standards)**
*Context:* You need to write or refactor zettels but want only the
knowledge-management rules, not the Go coding standards.
*Action:* `grep -rl "#best-practices" zk/**/*.md | xargs grep -L "#golang"`
*Result:* You get notes on prose-first style, zettel smells, tagging
strategy, and formatting — without the Go-specific standards mixed in.

**Scenario: Investigating Federation Architecture**
*Context:* You need to modify how remote Zettelkastens are linked.
*Action:* `grep -rl "#federation" zk/**/*.md | xargs grep -l "#architecture"`
*Result:* You find notes covering namespaced indexing, remote federation,
and unified note transport.

**Scenario: Following a Prose Reference**
*Context:* A zettel mentions "as defined by the
[prose-first style](202601171400-prose-first-style.md)."
*Action:* `cat zk/zk-spec/202601171400-prose-first-style.md`

**Scenario: Planning a New Feature**
*Context:* You are about to write significant new code and need both the
backlog and the plan structure.
*Action:* `grep -rlE "#planning|#backlog" zk/**/*.md | xargs cat`
*Result:* You get the full content of the Feature Backlog and the
Implementation Plan Guidelines in one pass — covering what to build and
how to structure the plan.
