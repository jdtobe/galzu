# Append-only prompt format

Status: partly implemented. The L0–L4 layout, canonical ID-based entries, stable
preamble, final turn packet, and a persisted byte-budget cut live in
`internal/herd/prompt.go`, `internal/herd/runner.go`, and
`internal/store/prompt_state.go`. The cut currently omits history without a
model-generated summary, and the prefix-drift guard and full telemetry are still
to come.

## Decision

An agent's prompt is a **byte-for-byte prefix of its next prompt**, followed by a
small, explicitly volatile turn packet. Within a compaction epoch the prefix is
only ever appended to: no block is inserted, edited, reordered, re-rendered, or
removed.

That single rule is what lets Ollama's KV cache work. Everything else in this
document is bookkeeping to keep the rule true across memory updates, retries,
renames, roster changes, and compaction.

## Why this is needed

Ollama 0.40.13 reuses the KV cache for the longest matching prompt prefix.
Measured locally with a 1,234-token prompt:

| Request | Prompt eval |
|---|---|
| First send | 2.69 s |
| Identical again | 0.05 s |
| One word changed in the first system line | 2.68 s |
| Identical again | 0.05 s |

Cache reuse therefore depends only on the prefix being identical, and reuse is
rounded down to a 256-token boundary (1,234 tokens reused 1,024 of them). The
request's `format` (JSON), `think` flag, and decoding options do not invalidate
the prefix; changing `num_ctx` does, because the model is reloaded.

Today Garth defeats this on every turn:

1. The first system message contains the roster **with live activity**, so it
   changes continuously. The prefix dies at roughly token 200.
2. The previous working memory is injected *before* the conversation. Memory is
   rewritten almost every turn, so even if the roster were fixed the history
   would still be re-evaluated.
3. History is a sliding window. Dropping its oldest message changes the first
   history byte, invalidating everything after it.
4. History lines embed peer **names**. Renaming an agent changes every line that
   mentions it.
5. Compaction has no representation: when the window shrinks, the prompt silently
   mutates rather than starting a new, explicitly cold epoch.

The result is a full re-evaluation of 6–27 KB prompts per turn, which is exactly
the prompt-eval cost dominating large-prompt agents.

## The invariant

For agent `A`, let `Fixed(n)` be the prompt rendered at turn `n` up to the end of
the last log entry, and `Tail(n)` the turn packet after it.

```
prompt(n) = Fixed(n) + Tail(n)
Fixed(n+1) = Fixed(n) + appended log entries
```

Constraints:

- `Fixed` contains no timestamps, no names, no counts, no activity, no memory.
- `Tail` is rebuilt every turn and is never reused.
- A change to any byte of `Fixed` is a **new epoch** and is recorded as such.

Cache reuse per turn is at least `len(Fixed(n))`, typically 80–95% of the prompt.
The tail is the only part that is always evaluated.

## Format

The protocol version is 8 at the time of writing. The action list is
maintained in `internal/herd/prompt.go`; actions added since this note (findings,
reviews, recall, plans, votes, capacity offers) are deliberately not duplicated
here, because a second copy would drift.

Levels are ordered from most stable to least stable. Each level is a separate
message with a fixed role.

### L0 — session preamble (shared by every agent on the model)

```
GALZU v8
Session <id>
Goal: <verbatim session goal>

You are one member of a cooperative herd. Be honest, welcome dissent, and value
correctness over popularity. Ask for help when evidence is missing. Private
messages are visible to the human operator.

Reply with exactly one JSON object and nothing else:
{"action":"message","text":"..."}                 public chat
{"action":"direct","recipient":"<agent id>","text":"..."}  private message
{"action":"help","recipient":"<agent id or empty>","text":"..."}  help request
{"action":"rest","text":""}                        stay quiet
Include "memory": a JSON string with your updated private working memory,
at most 2048 UTF-8 bytes.
```

Same bytes for all agents in a session, so switching agents still reuses L0.
Excludes: roster, counts, names, status, time, activity.

### L1 — agent preamble

```
You are agent <agent id>. Role: <role>.
```

IDs only. A name change must not touch this line.

### L2 — epoch header (frozen when the epoch is created)

```
EPOCH <n> (protocol 8)
Messages before id <cut id> are folded into the summary below.
Summary: <host-generated, <= 2048 bytes, or "none">
Your working memory at this point: <accepted memory, or "none">
```

Written once per epoch in a single transaction and never regenerated. This is the
only place where compaction appears in the prompt, which is why a context-exceeded
retry needs no extra notice.

### L3 — log entries (append-only)

One message renders to exactly one entry, including messages the agent cannot
read. Unexpanded visibility is rendered as a content-free notice, so the log
advances at the same rate for every agent.

```
<m> <sender id> public: <body verbatim>
<m> <sender id> private-to:<recipient id>: <body verbatim>
<m> <sender id> private-to:<recipient id> (notice): content is private
<m> host public: <body verbatim>
```

- Sender is an **ID**, never a name. The human is `host`.
- The entry number `m` is the stored message ID, stable and monotonic.
- A message from the agent itself is rendered with role `assistant`; everything
  else is role `user`. Both are deterministic at append time.
- The body is the stored, already-normalized text. No escaping, trimming, or
  re-wrapping is applied at render time.

### L4 — turn packet (volatile, rebuilt every turn)

```
TURN
Working memory (yours; may be stale): <accepted memory>
Roster (id | status | activity): <one line per agent>
Pending: <n> new messages
Host feedback: <validation reason and rejected excerpt, if any>
Compaction: <summary_rejected reason, if any>
Reply with one JSON object.
```

The tail is appended after the last log entry, not inserted before history. That
is the whole trick: memory and roster are the most volatile content, so they sit
where changing them costs nothing.

## Worked example

Turn 1, after messages 1–3:

```
system   [L0] GALZU v8 ... Reply with exactly one JSON object ...
system   [L1] You are agent a1. Role: leader.
system   [L2] EPOCH 0 ... Summary: none ...
user     [L3] 1 host public: Let's plan the release.
user     [L3] 2 a2 public: I'll check the tests.
assistant[L3] 3 a1 public: Good. I'll take the notes.
user     [L4] TURN ... memory ... roster ... Reply now.
```

Turn 2, after messages 4–5 arrive:

```
system   [L0] GALZU v8 ...                   (identical bytes)
system   [L1] You are agent a1 ...           (identical bytes)
system   [L2] EPOCH 0 ...                    (identical bytes)
user     [L3] 1 host public: ...             (identical bytes)
user     [L3] 2 a2 public: ...               (identical bytes)
assistant[L3] 3 a1 public: ...               (identical bytes)
user     [L3] 4 a3 private-to:a1: ...        (new)
user     [L3] 5 a2 public: ...               (new)
user     [L4] TURN ... updated memory ...    (new tail)
```

The new entries land exactly where the old tail was, so everything through
message 3 is a byte prefix and is served from cache.

## Canonical rendering rules

These are invariants, not style preferences. A change to any of them is a
protocol version bump and a new epoch.

- UTF-8, LF only, no CR, no BOM, no trailing whitespace.
- No timestamps, durations, percentages, or locale-formatted numbers.
- No map iteration; every block is built from an explicit template.
- IDs in the log, names only in the tail roster.
- Message bodies are stored normalized once (valid UTF-8, LF) and emitted verbatim.
- Stable separators; no blank line between entries.
- Integer message IDs in decimal, no padding.

Implementation status: the contents below L0/L1 are enforced by tests, but the
epoch machinery is not built. There is no `prompt_epochs` table and no
`prefix_hash` check; the only persisted state is `agent_prompt_state.cut_message_id`,
which moves forward at compaction. Changing the rendered bytes of the prefix —
including bumping `promptProtocol` — therefore costs a cold cache once and is
recorded nowhere. The hash check is a good idea and is not implemented.

## Lifecycle

| Event | Effect on the prompt | Cache |
|---|---|---|
| New message | append one L3 entry | reused |
| Agent replies | reply appended at next turn (assistant entry) | reused |
| Memory rewritten | tail only | reused |
| Roster/activity changes | tail only | reused |
| Agent renamed | tail only | reused |
| Session renamed | nothing (name is not in the prompt) | reused |
| Validation retry | tail gets host feedback | reused |
| Batch larger than 50 | extra entries next turn | reused |
| Pause/resume | tail; model may have unloaded | maybe cold |
| Turn skipped (rest/no trigger) | no prompt | n/a |
| Compaction | new L2 header, log cut at `cut id` | cold once |
| Context-limit error | force new epoch with smaller budget | cold once |
| Protocol/role/goal/model change | new epoch | cold once |
| `think` toggled | tail unchanged | reused |
| `num_ctx` changed | model reload | cold |
| Deleted session | epochs deleted with it | n/a |

## Compaction

### Budget

Measure, don't guess:

```
tokens  ≈ bytes / calibrated_bytes_per_token(model)
budget  = 0.60 * num_ctx        (high water mark)
target  = 0.30 * num_ctx        (size after compaction)
reserve = max response (4096) + thinking headroom
```

`bytes_per_token` starts at 3.5 and is corrected from cold-start
`prompt_eval_count`, stored per model. If the estimate overshoots and the server
returns a context error, that is treated as a compaction trigger, not as a reason
to silently halve the batch.

### Sequence

1. Estimate the prompt. Below `budget`, do nothing.
2. Choose `cut`: the newest message such that the visible entries after it fit in
   `target`, leaving room for the tail.
3. Summarize entries in `[epoch_start, cut]`, folding in the previous summary, in a
   dedicated inference call with no agent turn consumed.
4. Validate: non-empty, ≤2048 bytes, valid UTF-8. On failure keep the old epoch,
   record `summary_rejected` with the reason, and retry after N messages with
   backoff.
5. Commit in one transaction: new `prompt_epochs` row with the frozen header
   (summary + accepted memory at `cut`), `cut id`, hash, reason.
6. Next turn renders `L0 + L1 + L2(new) + entries after cut + tail`.

Compaction is the one intentional cold start. At 60%→30% on a 64k window, it
happens after roughly 20k tokens of turnover, i.e. every ~20 turns at 1k
tokens/turn. Amortized cost is one full eval per epoch plus one summarization.

### Freezing

The header is immutable for the life of the epoch, even when:

- the agent's memory changes (memory lives in the tail; the header keeps the
  snapshot taken at the cut);
- the roster changes;
- a new message arrives that "belongs" to the summarized range (it can't: the cut
  is a message ID, and IDs only increase).

The only sanctioned way to change a header is a new epoch.

## Recovery paths

- **Validation failure.** Rejected output is not appended to the log; it appears in
  the tail as `Host feedback`, so the prefix through the batch is preserved.
- **Context limit.** Build a new epoch with a smaller target and `reason =
  context`, and let L2 state that earlier messages are summarized. Do not claim
  that the model can retrieve them; direct historical search still does not exist.
- **Storage failure.** Unchanged: stop scheduling and require a successful write
  probe. No prompt is rendered from uncommitted state.
- **Interrupted attempt.** The unfinished reply is not in the log; the next turn
  simply renders the same prefix plus a new tail.
- **Summary rejected.** Keep the current epoch and its prefix; retry later. Never
  apply a partial or oversized header.

## Scheduling and cache slots

Ollama keeps one KV cache per parallel slot. With 11 agents and 2 slots, agent
turns interleave and each slot's cache is overwritten, so measured reuse will be
lower than the format permits.

Mitigations, in order of preference:

1. Keep the shared L0 identical across agents, so an agent switch still reuses the
   session preamble.
2. Prefer running an agent's backlog to completion before switching (bounded by a
   fairness cap) instead of strict round-robin.
3. Keep `num_parallel` aligned with Garth's concurrency and pin `num_ctx` globally,
   since changing it reloads the model and drops every cache.

Slot assignment is not exposed by the API, so this has to be validated by
measurement rather than assumed.

## Data model

```sql
CREATE TABLE prompt_epochs(
  agent_id        TEXT NOT NULL REFERENCES agents(id),
  epoch           INTEGER NOT NULL,
  protocol        INTEGER NOT NULL,
  header          TEXT NOT NULL,      -- frozen L2 block
  cut_message_id  INTEGER NOT NULL,
  prefix_hash     TEXT NOT NULL,
  reason          TEXT NOT NULL,      -- initial | budget | context | settings
  created_at      TEXT NOT NULL,
  PRIMARY KEY (agent_id, epoch)
);
```

No rendered entries are stored: rendering from message rows is deterministic under
a fixed protocol. Existing sessions get epoch 0 (`header = "Summary: none"`,
`cut = 0`) lazily at their next turn, and compact immediately if over budget.

`agent_memory` keeps the live memory. Each epoch header stores the snapshot used
at its cut, so a memory update never rewrites history.

## Diagnostics

Parse `prompt_eval_count`, `prompt_eval_duration`, and `eval_count` from the
inference response and record per attempt:

- `fixed_bytes`, `tail_bytes`, `prompt_bytes`;
- `prefix_hash` and whether it matched the epoch;
- `reused_bytes` and `reuse_percent` — the byte prefix shared with the agent's
  previous prompt, which is an upper bound on KV reuse;
- `cold_reason` — `epoch_start`, `model_reload`, `slot_evicted`, or `prefix_drift`.

`prompt_eval_count` is the whole prompt, not the cache misses, so it does not
measure reuse. `prompt_eval_duration` is the only direct signal: an identical
prompt evaluates in tens of milliseconds, while a full evaluation costs roughly
one millisecond per token on the reference machine.

Show reuse, prompt tokens, and prompt-evaluation time in the agent inspector. A
prefix drift event is a regression, not a statistic.

## Measurements

Probing Ollama 0.40.13 directly (no harness) established the server's behaviour:

| Request | Prompt tokens | Prompt evaluation |
| --- | ---: | ---: |
| cold, first request | 6,725 | 14,515 ms |
| identical repeat | 6,725 | 72 ms |
| a different 6,839-token prompt | 6,839 | 14,833 ms |
| the first prompt again, after the second | 6,725 | 69 ms |
| the first prompt plus 5 trailing tokens | 6,730 | 1,264 ms |

So the server keeps more than one cached prefix, matches on the token prefix, and
re-evaluates the divergent suffix. Divergence at the end of a prompt is cheap but
not free, which is exactly the shape the turn packet produces.

An end-to-end run with two agents and growing history confirms this through the
proxy. The prompt grew 4.7x (1,955 → 9,291 tokens) while prompt evaluation stayed
between 0.7 s and 1.3 s, and the measured `reused_bytes` rose to 96–98%. A fixed
prefix has to hold, and it does.

## Tests

1. **Prefix monotonicity.** Render consecutive turns with new messages, a memory
   rewrite, roster changes, a rename, and a validation retry; assert
   `bytes.HasPrefix(next, prev)` up to the previous Fixed end.
2. **Rename invariance.** Rename an agent and a session; Fixed bytes are unchanged.
3. **Visibility.** Private messages render as notices for non-recipients and are
   byte-stable across turns.
4. **Compaction.** At the threshold, exactly one epoch is written; the header is
   stable on the following turns; entries before the cut disappear; entries after
   it remain byte-identical.
5. **Rejected summary.** Oversized or empty summaries leave the epoch and prefix
   untouched and record a reason.
6. **Context error.** Forces a new epoch with a smaller target, and the next prompt
   is monotonic again.
7. **Pause/resume and restart.** Rendering is deterministic across process
   restarts (no map iteration, no clock).
8. **Golden prompt.** A fixed fixture renders to a stored golden file; any change
   requires an explicit protocol bump.

## Rollout

1. Implement the renderer and the prefix test with no behavior change to the store
   (prompts still unbounded).
2. Move roster, activity, and memory into the tail; measure reuse before adding
   epochs. This alone should recover most of the cache.
3. Add `prompt_epochs`, compaction, and the byte budget.
4. Add diagnostics to the inspector and tune thresholds from real reuse data.
5. Only then consider historical retrieval, which is orthogonal: retrieval results
   would enter through the tail or a new epoch, never by editing history.

## Open questions

- Does Ollama guarantee the cache survives an idle period shorter than
  `keep_alive`? We should set `keep_alive` explicitly rather than rely on defaults.
- How does slot assignment behave with more agents than slots? Needs a measurement
  pass; the design does not depend on the answer, only on how much reuse we can
  expect.
- Should compaction summarize with the agent's own model (keeps perspective,
  consumes a turn) or a dedicated small model (cheaper, loses private context)? I
  lean toward the agent's model, run as a separate call that does not count as a
  conversational turn.
- Do we ever need to edit the log (message deletion, redaction)? If so it must
  force an epoch bump; silent rewrites would reintroduce the problem this design
  exists to remove.
