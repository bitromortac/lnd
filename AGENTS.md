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
  git clone --no-checkout --depth 1 --sparse github.com/bitromortac/bolts /tmp/temp_bolts
  cd /tmp/temp_bolts && git sparse-checkout set zk && git checkout && cd -
  cp -r /tmp/temp_bolts/zk zk/bolts
else
  echo "Namespace 'zk/bolts' already exists. Skipping materialization."
fi
```

### Namespace: `lnd`

```bash
if [ ! -d "zk/lnd" ]; then
  git clone --no-checkout --depth 1 --sparse [ -b <version> ] https://github.com/<org>/<repo> /tmp/temp_lnd
  cd /tmp/temp_lnd && git sparse-checkout set <subfolder> && git checkout && cd -
  cp -r /tmp/temp_lnd/<subfolder> zk/lnd
else
  echo "Namespace 'zk/lnd' already exists. Skipping materialization."
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
  git clone --depth 1 github.com/bitromortac/zk-dev /tmp/temp_zk-spec
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
#ai-interaction (12)
  [Related: #workflow (4), #ambiguity (2), #best-practices (2), #communication (2), #efficiency (2)]
#ambiguity (2) [Related: #ai-interaction (2), #requirements (2)]
#api-design (2) [Related: #clean-code (2), #golang (2), #readability (2)]
#architecture (30)
  [Related: #invoices (8), #htlc (7), #on-chain (7), #lightning-network (6), #security (5)]
#automation (4)
  [Related: #makefile (2), #workflow (2), #ai (1), #architecture (1), #lightning-network (1)]
#backup (2)
  [Related: #peer-storage (2), #protocol (2), #message (1), #redundancy (1)]
#best-practices (9)
  [Related: #entry-point (4), #planning (4), #ai-interaction (2), #documentation (2), #formatting (2)]
#bigsize (2)
  [Related: #serialization (2), #encoding (1), #requirement (1), #test-vector (1)]
#blinded-paths (2)
  [Related: #bolt-12 (1), #invoices (1), #lnd (1), #privacy (1), #routing (1)]
#bolt (3)
  [Related: #protocol (3), #specification (2), #channel (1), #entry-point (1), #index (1)]
#bolt-1 (5)
  [Related: #requirement (5), #messaging (2), #tlv (2), #encoding (1), #entry-point (1)]
#bolt-11 (7)
  [Related: #invoices (6), #requirement (4), #protocol (3), #lifecycle (2), #design-decision (1)]
#bolt-12 (33)
  [Related: #lnd (16), #protocol (16), #feature-request (12), #requirement (10), #invoices (8)]
#bolt-2 (17)
  [Related: #requirement (17), #channel-establishment (5), #operation (5), #v1 (4), #dual-funding (3)]
#channel (3)
  [Related: #protocol (3), #identifier (2), #v1 (2), #v2 (2), #bolt (1)]
#channel-closing (4)
  [Related: #protocol (4), #message (3), #fee-negotiation (2), #lifecycle (1), #mutual-close (1)]
#channel-establishment (16)
  [Related: #protocol (10), #v1 (9), #message (6), #bolt-2 (5), #requirement (5)]
#channel-management (3)
  [Related: #protocol (2), #architecture (1), #channel-establishment (1), #error-handling (1), #incentives (1)]
#channel-state (3)
  [Related: #protocol (2), #architecture (1), #database (1), #maintenance (1), #storage (1)]
#clean-code (28)
  [Related: #formatting (12), #golang (12), #readability (12), #example (8), #logging (6)]
#closing (2) [Related: #bolt-2 (2), #lifecycle (2), #requirement (2)]
#collaboration (2)
  [Related: #protocol (2), #transaction (2), #dual-funding (1), #rbf (1)]
#commitment (3)
  [Related: #bolt-2 (1), #counter (1), #message (1), #operation (1), #protocol (1)]
#communication (2) [Related: #ai-interaction (2), #efficiency (2)]
#complexity (2) [Related: #clean-code (2), #readability (2), #refactoring (2)]
#core-concept (2)
  [Related: #channels (1), #htlc (1), #off-chain (1), #payments (1)]
#cryptography (5)
  [Related: #bolt-12 (3), #privacy (2), #signature (2), #architecture (1), #derivation (1)]
#database (2)
  [Related: #architecture (2), #storage (2), #channel-state (1), #invoices (1)]
#diagram (3)
  [Related: #entry-point (3), #architecture (2), #dispute-resolution (1), #on-chain (1), #pathfinding (1)]
#discovery (2)
  [Related: #bootstrapping (1), #dns (1), #gossip (1), #protocol (1), #topology (1)]
#dispute-resolution (7)
  [Related: #on-chain (6), #architecture (4), #lightning-network (2), #security (2), #channel-lifecycle (1)]
#documentation (6)
  [Related: #clean-code (4), #best-practices (2), #entry-point (2), #golang (2), #readability (2)]
#dual-funding (6)
  [Related: #bolt-2 (3), #establishment (3), #protocol (3), #requirement (3), #channel-establishment (2)]
#efficiency (2) [Related: #ai-interaction (2), #communication (2)]
#encoding (5)
  [Related: #protocol (3), #bolt-12 (2), #tlv (2), #bigsize (1), #bolt-1 (1)]
#entry-point (22)
  [Related: #architecture (4), #best-practices (4), #requirement (4), #workflow (4), #diagram (3)]
#error-handling (7)
  [Related: #protocol (7), #message (5), #htlc (2), #bolt-12 (1), #channel-management (1)]
#establishment (3) [Related: #bolt-2 (3), #dual-funding (3), #requirement (3)]
#example (12)
  [Related: #clean-code (8), #formatting (8), #golang (6), #logging (4), #readability (4)]
#extensibility (2)
  [Related: #protocol (1), #serialization (1), #tlv (1), #versioning (1)]
#feature-request (12)
  [Related: #bolt-12 (12), #lnd (12), #workflow (4), #invoices (3), #rpc (3)]
#fee-negotiation (2)
  [Related: #channel-closing (2), #message (2), #protocol (2)]
#formatting (18)
  [Related: #clean-code (12), #readability (10), #example (8), #golang (4), #tools (4)]
#git (8)
  [Related: #workflow (6), #clean-code (2), #entry-point (2), #process (2), #security (2)]
#golang (20)
  [Related: #clean-code (12), #example (6), #workflow (6), #formatting (4), #logging (4)]
#gossip (2)
  [Related: #architecture (1), #discovery (1), #lightning-network (1), #topology (1)]
#graph-theory (2) [Related: #pathfinding (2), #routing (2), #architecture (1)]
#htlc (20)
  [Related: #invoices (9), #architecture (7), #protocol (6), #lightning-network (4), #message (4)]
#identifier (2) [Related: #channel (2), #protocol (2), #v1 (2), #v2 (2)]
#invoice-request (3)
  [Related: #bolt-12 (3), #protocol (3), #requirement (2), #message (1)]
#invoices (27)
  [Related: #htlc (9), #architecture (8), #bolt-12 (8), #bolt-11 (6), #requirement (6)]
#lifecycle (8)
  [Related: #protocol (5), #requirement (3), #bolt-11 (2), #bolt-2 (2), #closing (2)]
#lightning-network (10)
  [Related: #architecture (6), #invoices (5), #htlc (4), #dispute-resolution (2), #security (2)]
#llformat (2)
  [Related: #clean-code (2), #formatting (2), #readability (2), #tools (2)]
#lnd (16)
  [Related: #bolt-12 (16), #feature-request (12), #invoices (5), #storage (4), #workflow (4)]
#logging (7)
  [Related: #clean-code (6), #example (4), #golang (4), #formatting (2), #error-handling (1)]
#maintenance (2)
  [Related: #ai (1), #automation (1), #channel-state (1), #protocol (1), #synchronization (1)]
#makefile (2) [Related: #automation (2), #workflow (2)]
#message (39)
  [Related: #protocol (39), #transaction-construction (9), #channel-establishment (6), #error-handling (5), #htlc (4)]
#messaging (3)
  [Related: #bolt-1 (2), #protocol (2), #requirement (2), #bolt (1), #specification (1)]
#negotiation (2)
  [Related: #protocol (2), #channel-establishment (1), #error-handling (1)]
#network (2) [Related: #participant (2), #node (1), #peer (1)]
#networking (2)
  [Related: #architecture (2), #multiplexing (1), #protocol (1), #security (1)]
#offer (4)
  [Related: #bolt-12 (4), #protocol (4), #requirement (2), #message (1), #workflow (1)]
#on-chain (10)
  [Related: #architecture (7), #dispute-resolution (6), #security (3), #channel-lifecycle (1), #diagram (1)]
#onion-message (2)
  [Related: #bolt-12 (2), #feature-request (2), #lnd (2), #routing (1)]
#operation (6)
  [Related: #bolt-2 (5), #requirement (5), #htlc (4), #commitment (1), #lifecycle (1)]
#participant (2) [Related: #network (2), #node (1), #peer (1)]
#pathfinding (4)
  [Related: #routing (4), #graph-theory (2), #architecture (1), #diagram (1), #entry-point (1)]
#payment (7)
  [Related: #htlc (3), #routing (3), #architecture (2), #message (2), #protocol (2)]
#payment-request (2)
  [Related: #invoices (2), #bolt-11 (1), #encoding (1), #protocol (1), #user-interface (1)]
#peer-storage (3)
  [Related: #protocol (3), #backup (2), #message (2), #recovery (1), #redundancy (1)]
#planning (4)
  [Related: #best-practices (4), #ai-interaction (2), #entry-point (2), #process (2), #protocol (2)]
#privacy (6)
  [Related: #protocol (3), #routing (3), #cryptography (2), #security (2), #architecture (1)]
#process (6)
  [Related: #workflow (4), #best-practices (2), #entry-point (2), #git (2), #golang (2)]
#protocol (86)
  [Related: #message (39), #bolt-12 (16), #requirement (12), #channel-establishment (10), #transaction-construction (9)]
#protocols (2) [Related: #ai-interaction (2), #entry-point (2), #workflow (2)]
#quiescence (2)
  [Related: #protocol (2), #bolt-2 (1), #message (1), #requirement (1), #state-machine (1)]
#rbf (3)
  [Related: #protocol (3), #message (2), #transaction-construction (2), #collaboration (1), #transaction (1)]
#readability (16)
  [Related: #clean-code (12), #formatting (10), #example (4), #golang (4), #api-design (2)]
#recovery (2)
  [Related: #message (2), #protocol (2), #peer-storage (1), #reconnection (1), #state-sync (1)]
#refactoring (2) [Related: #clean-code (2), #complexity (2), #readability (2)]
#requirement (37)
  [Related: #bolt-2 (17), #protocol (12), #bolt-12 (10), #invoices (6), #bolt-1 (5)]
#requirements (3)
  [Related: #ai-interaction (2), #ambiguity (2), #entry-point (1), #specification (1)]
#revocation (4)
  [Related: #bolt-2 (1), #cryptography (1), #derivation (1), #enforcement (1), #message (1)]
#routing (12)
  [Related: #pathfinding (4), #architecture (3), #payment (3), #privacy (3), #graph-theory (2)]
#rpc (7)
  [Related: #invoices (5), #bolt-12 (3), #feature-request (3), #lnd (3), #architecture (2)]
#safety (2) [Related: #ai-interaction (2), #workflow (2)]
#security (17)
  [Related: #architecture (5), #protocol (4), #on-chain (3), #dispute-resolution (2), #git (2)]
#serialization (3)
  [Related: #bigsize (2), #encoding (1), #extensibility (1), #requirement (1), #test-vector (1)]
#settlement (2) [Related: #architecture (2), #htlc (2), #invoices (2)]
#setup (2)
  [Related: #protocol (2), #connection (1), #handshake (1), #initialization (1), #message (1)]
#signature (2)
  [Related: #bolt-12 (2), #cryptography (2), #privacy (1), #protocol (1), #requirement (1)]
#signing (2)
  [Related: #protocol (2), #concurrency (1), #message (1), #transaction (1), #transaction-construction (1)]
#specification (3)
  [Related: #bolt (2), #protocol (2), #channel (1), #entry-point (1), #lifecycle (1)]
#standard (2)
  [Related: #best-practices (2), #entry-point (2), #planning (2), #process (2)]
#standards (2)
  [Related: #best-practices (2), #formatting (2), #readability (2), #writing (2)]
#state-machine (4)
  [Related: #architecture (2), #invoices (2), #channel-establishment (1), #channel-management (1), #htlc (1)]
#state-update (2)
  [Related: #message (2), #protocol (2), #commitment (1), #revocation (1)]
#storage (7)
  [Related: #bolt-12 (4), #lnd (4), #feature-request (3), #architecture (2), #database (2)]
#template (2) [Related: #ai-interaction (2), #example (2)]
#testing (7)
  [Related: #golang (4), #bolt-12 (2), #clean-code (2), #requirement (2), #workflow (2)]
#tlv (4)
  [Related: #bolt-1 (2), #encoding (2), #requirement (2), #bolt-12 (1), #extensibility (1)]
#tools (6)
  [Related: #formatting (4), #clean-code (2), #golang (2), #llformat (2), #readability (2)]
#transaction (6)
  [Related: #protocol (5), #collaboration (2), #channel-state (1), #concurrency (1), #dual-funding (1)]
#transaction-construction (9)
  [Related: #message (9), #protocol (9), #rbf (2), #error-handling (1), #signing (1)]
#v1 (11)
  [Related: #channel-establishment (9), #protocol (7), #bolt-2 (4), #message (4), #requirement (4)]
#v2 (5)
  [Related: #protocol (5), #channel-establishment (3), #channel (2), #identifier (2), #message (2)]
#workflow (24)
  [Related: #git (6), #golang (6), #bolt-12 (5), #ai-interaction (4), #entry-point (4)]
#writing (2)
  [Related: #best-practices (2), #formatting (2), #readability (2), #standards (2)]
#zettelkasten (2)
  [Related: #ai (1), #automation (1), #best-practices (1), #maintenance (1), #mapping (1)]
(Untagged) (1)
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

- [[zk/zk-spec/202602091210-AI-Interaction-Protocols.md]]
- [[zk/zk-dev/202602091210-AI-Interaction-Protocols.md]]
- [[zk/bolts/202602141540-Bolt-1-requirements.md]]
- [[zk/bolts/202602151010-Bolt-11-requirements.md]]
- [[zk/bolts/202603251215-Bolt-12-offers-protocol.md]]
- [[zk/bolts/202602141530-Bolt-2-requirements.md]]
- [[zk/bolts/202602131200-Bolt-Defines-Layer-2-Protocol.md]]
- [[zk/bolts/202602141545-Bolt-protocol-requirements.md]]
- [[zk/zk-dev/202601141400-Codebase-standards.md]]
- [[zk/zk-spec/202601141400-Codebase-standards.md]]
- [[zk/lnd/202603251003-Contract-Court-Resolution.md]]
- [[zk/zk-dev/202601281200-Developer-guides.md]]
- [[zk/zk-spec/202601281200-Developer-guides.md]]
- [[zk/202603251500-Feature-Backlog.md]]
- [[zk/zk-dev/202603171000-Git-Contribution-Conventions.md]]
- [[zk/zk-spec/202603171000-Git-Contribution-Conventions.md]]
- [[zk/zk-spec/202601271000-Implementation-Plan-Guidelines.md]]
- [[zk/zk-dev/202601271000-Implementation-Plan-Guidelines.md]]
- [[zk/lnd/202603250830-Invoices.md]]
- [[zk/lnd/202603181000-Lnd-Architecture.md]]
- [[zk/lnd/202603181010-Pathfinding-Router.md]]
- [[zk/lnd/202603251000-Watchtower-Architecture.md]]

### High-Centrality Hubs
Reference these to locate the most interconnected atomic concepts:

- [[zk/202603261030-bolt-12-offer-to-payment-flow.md]] (Degree: 27)
- [[zk/202603261300-bolt12-implementation-strategy.md]] (Degree: 25)
- [[zk/bolts/202602131272-bolt-2-peer-protocol-manages-channels.md]] (Degree: 16)
- [[zk/202603261230-bolt12-mvp-direct-peers.md]] (Degree: 13)
- [[zk/202603261245-bolt12-micro-mvp.md]] (Degree: 12)
- [[zk/bolts/202602131261-interactive-transaction-construction-enables-collaboration.md]] (Degree: 12)
- [[zk/bolts/202602131271-bolt-1-base-protocol-defines-messaging.md]] (Degree: 11)
- [[zk/202603251505-request-invoice-rpc.md]] (Degree: 11)
- [[zk/bolts/202602131262-channel-establishment-v2-supports-collaboration.md]] (Degree: 10)
- [[zk/bolts/202602131245-normal-operation-uses-htlcs.md]] (Degree: 8)

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
