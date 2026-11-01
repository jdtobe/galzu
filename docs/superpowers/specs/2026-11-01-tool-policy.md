# Tool policy and approvals

Status: **decisions settled 2026-11-01. The gate is implemented; execution is
not.** `internal/toolgate` evaluates requests, and the scope, arm, disarm,
approval and audit endpoints exist. No agent action reaches the gate yet.

This specifies the boundary that `internal/networktool` must sit behind before any
agent can use it. It answers the first open question in
[the platform design](2026-10-31-herd-platform-design.md#open-questions-before-implementation):
scope, rate limiting, approval and audit semantics for external requests.

Nothing here authorises a live assessment. The NAS run needs this implemented,
reviewed, and then armed deliberately.

## Why this exists

The PoC success test requires that a herd can *route external tool requests
through its leader*. Today it cannot: the network primitives exist, the
`network_policies` table stores a target IP and interval, and no code path reads
either. Agents can message, help and rest; they cannot act.

The gap is not the transport. It is the decision: who may ask for what, who
answers, what is refused regardless of who asks, and what survives afterwards.

## Threat model

The policy is designed against **adversarial drift**: a model that is
compromised, prompt-injected, over-eager, or simply convinced it is right. That
case subsumes honest accident, because a harness that refuses out-of-policy
action regardless of argument also refuses it when the model is confused.

**Operator error** is real but is answered by interface discipline — deliberate
acts, no approve-all button, one line of written intent — not by policy rules.

Explicitly out of scope: an operator with database access, a compromised host,
a malicious model provider, and supply-chain attacks on Ollama.

## Non-goals

- No shell, no file writing, no UDP. HTTP and TCP only.
- Not a guarantee of safety on the target. The tool can only do what the
  envelope allows and rate-limits; it cannot know that `POST /restart` restarts.
- Not proof that an assessment was authorised. Research on a host is distinct
  from authorisation to assess it, and only the human can hold that distinction.

## The envelope

The envelope is the scope a human grants to one session. Nothing outside it
exists as far as agents are concerned.

An envelope is a list of entries, each of which is:

| Field | Meaning |
|---|---|
| `name` | The hostname agents may reference. Approved names are the only names ever resolved |
| `ports` | Allowed ports |
| `methods` | Allowed verbs, or "all" |
| `path_prefix` | Optional. Without one, any path on that host is reachable |
| `ips` | Optional. The IPs the name may resolve to; defaults to whatever the human saw at grant time |

Alongside the entries, the envelope carries budgets:

- total request count for the session
- a separate count per risk tier
- the minimum interval between requests (already stored as `rate_interval_ms`)
- a maximum request body size

The envelope is **versioned**. Any grant, tightening, or revocation increments
the version. Requests record the version they ran under, so a past decision can
always be read against the scope in force when it happened.

Agents see only the current version and their remaining budget — never the full
list of entries, which would leak the map of the operator's network into every
prompt.

## Enforcement point

The gate is a wrapper in front of `networktool.Executor`, not a rule in the
prompt. The prompt states the policy as *guidance*, so a well-behaved model
plans sensibly; the refusal is structural, so a model that argues gets nowhere.

Every request passes the same ordered check, and any failure is a refusal:

1. Is the session armed? (An envelope can exist while unarmed.)
2. Does the request name an approved host? An unapproved name is refused
   **before it is resolved**, so no lookup happens at all.
3. Resolve the name **once**; validate that the resolved IP is inside the
   entry's IP set.
4. Does the method match the entry's allowed methods and its tier's remaining
   budget?
5. Has the minimum interval elapsed?
6. Is the body within the size cap?
7. Dispatch to the **pinned IP**, with the original hostname used for the
   `Host` header and TLS SNI.

Redirects are never followed. `networktool` already returns them unfollowed; a
redirect is a *result*, and following it is a new request that must clear the
gate again.

### Why names, not addresses

An earlier position was literal IPs only, as the code enforces today. It was
rejected: connecting to an IP with `https` breaks certificate verification and
sends no useful SNI, so the safest form of request was the one that did not
work. Names are allowed instead, with the dangers handled explicitly:

- **Rebinding** — a name that resolves harmlessly at approval time and to the
  NAS at request time is defeated by resolving once, validating the IP, and
  dialling the pinned address. Go's default transport re-resolves at dial time,
  so this requires a custom `DialContext`.
- **Exfiltration by query** — a tool that resolves a model-chosen name leaks
  through the lookup itself, even when the request fails. Names outside the
  envelope are therefore never resolved.

## Methods are tiers, not a whitelist

The harness cannot see consequence: `POST /search` and `POST /restart` are the
same eleven bytes to a policy engine. The method is a risk signal, and the
policy treats it as one.

| Tier | Verbs | Treatment |
|---|---|---|
| Safe | `GET`, `HEAD`, `OPTIONS` | Dispatch inside the envelope |
| Unsafe | `POST`, `PUT`, `PATCH`, `DELETE` | Dispatch inside the envelope, separate budget, recorded as itself |

No verb is banned. `DELETE` is not special-cased by prohibition but by
**approval**: the first `DELETE` against each host in a session requires the
human, once. That is the one verb that can violate the platform design's own
line — *avoiding lasting damage is a goal, not a guarantee* — and one extra
pause per host is cheap.

## Approval authority

| Situation | Who decides |
|---|---|
| Inside the envelope, safe or unsafe method | Leader, including its own requests |
| First `DELETE` against a host in this session | Human |
| Outside the envelope | Human |
| Budget exhausted | Nobody — refused |
| Leader escalates with a recommendation | Human |

**Self-review is kept.** The leader reviewing its own requests is a conflict of
interest, but the envelope, the budget and the `DELETE` gate bound what it can
grant itself, and the herd holds the leader to account through no-confidence.
Note the dependency honestly: **no-confidence is a later milestone and takes
turns to run**, so on the first live run the real backstop is one-click
revocation, not governance.

When the leader is unavailable, requests **queue**. If the leader is in an error
state, escalate immediately rather than waiting. Otherwise escalate to the human
after a timeout. Pausing the session **freezes** that timer rather than letting
it run against a leader that has been stopped.

Requests never block the leader's own loop: approval happens at a safe point
between turns, so a request queue cannot starve the herd.

## Request shape

An agent declares:

- which **envelope entry** it is using (`name`, `method`, `path`)
- `purpose`: one line, required, bounded in size
- `expected`: what it believes will happen

The harness computes everything decision-relevant — resolved IP, tier, remaining
budget, requester, envelope version, timestamps — and **never trusts the model's
claims**. The agent's stated purpose and expectation are recorded and shown to
the human; they carry no authority.

A request is a single action, never a batch. Parallel bursts are what budgets and
intervals exist to make visible.

## Budgets and refusal

When a budget is exhausted the request is **refused, never escalated**. Escalating
after exhaustion invites a human to say yes under pressure, which is exactly the
operator-error failure the interface is meant to prevent.

Refusals are recorded like successes, because a refused request is evidence about
what the herd attempted.

## Unknown outcomes

A request can leave the machine and return unknown: a connection dropped after
the bytes went out, a timeout mid-response, a `PUT` that may or may not have
landed.

- The outcome is recorded as **unknown**, not as failure.
- Non-idempotent methods are **never retried automatically**.
- Only `GET` and `HEAD` may be retried automatically, and they still consume
  budget.
- Any other retry is a fresh request that clears the whole gate again.

## Audit

Every request produces an append-only record:

- request: entry, method, path, purpose, expectation, requester, envelope version
- decision: allowed, refused (with reason), or escalated, and who decided
- execution: resolved IP, bytes sent and received, duration, status, outcome
- body handling: size, digest, and a bounded sanitised excerpt of at most 1 KB

Bodies may contain private content, so the excerpt follows the same rule as
validation excerpts: visible to the operator, never written to ordinary logs.

Audit records live in **their own table and survive session deletion**. Deleting
a session destroys its messages, memory and checkpoints — the point of that
button is that nothing is recoverable — but it must not also erase the record of
what its agents attempted against the network. The delete confirmation says this
plainly, because it is the one exception to "no undo".

## The live-run gate

Four conditions, all required, before a single request leaves the machine:

1. **Arming is separate from creating a session.** A session may record a scope;
   nothing executes until the human arms it, and arming is audited with a
   timestamp.
2. **The human writes one line of what counts as lasting damage** for this run.
   The platform design asks agents to avoid lasting damage; the harness cannot
   evaluate that, so a human states the bar once and it is shown to every agent.
3. **A dry run first.** Agents may submit requests that resolve, validate and
   budget-check, and the UI shows exactly what would be sent, including resolved
   IPs — with nothing leaving the machine. The human reviews that list before
   arming.
4. **The backup precondition is the human's.** A recent NAS snapshot is recorded
   as a checkbox they tick. The tool cannot verify it.

## Interface

The creation form's "Future network scope · inactive" section becomes a live
**Scope panel**:

- envelope entries and their resolved IPs
- budgets and what remains, per tier
- armed or unarmed, and by whom
- **Revoke**, one click

Revocation is the emergency brake and matters more than any approval queue: it
takes one click where a no-confidence vote takes turns. Revoking stops new
requests immediately; a request already in flight is allowed to finish and is
recorded, matching how pause treats inference.

The approval queue shows pending requests with the leader's recommendation and
reasoning, the agent's stated purpose, and what remained in the budget. Pending
attention is surfaced in the UI; browser notifications are a later addition.

## Failure modes

| Situation | Behaviour |
|---|---|
| Session deleted mid-request | In-flight result discarded, request recorded as unknown, audit survives |
| Envelope revoked mid-request | In-flight completes and is recorded; no new requests |
| Session paused mid-request | Completion is held like an inference result |
| App restart | Session returns paused and **unarmed**; arming never survives a restart |
| Leader in error state | Requests escalate to the human immediately |
| Resolution returns an out-of-set IP | Refused before the request is sent |
| Budget exhausted | Refused; recorded |
| Redirect received | Returned as a result; following it is a new request |

## Implementation notes

Mapping onto what exists today, so this is buildable in slices:

- **`internal/networktool`** — accept hostnames with a custom `DialContext` that
  validates then dials the pinned IP; add tier classification to `HTTP`; add a
  body size cap; keep the shared interval limiter and add per-tier counters.
- **`internal/store`** — `network_policies` holds one target and one interval and
  is too narrow. Replace with `envelopes` (versioned) and `envelope_entries`,
  plus `tool_requests` (queue and decisions) and `tool_audit` (append-only).
  There is no migration framework; this is a series of `CREATE TABLE IF NOT
  EXISTS` statements and a documented upgrade path.
- **`internal/herd`** — a fifth action, `tool`, added to the whitelist in
  `ollama/client.go`, the kind list in `store.go`, and validation in
  `runner.go`, with `promptProtocol` bumped.
- **`internal/herd/prompt.go`** — policy text in the session preamble, which is
  cache-stable and identical across agents; current budget, envelope version and
  the human's lasting-damage line in the turn packet, which is volatile.
- **`internal/server`** — arm, revoke, and approve/deny endpoints; the Scope
  panel and approval queue in the web UI.

## Success criteria for the first slice

The first implementable slice is the gate, not the transport. It is done when:

1. An unarmed session cannot send anything, and a test proves it.
2. A request naming a host outside the envelope is refused without a DNS lookup.
3. A name resolving outside its entry's IP set is refused before the request.
4. The first `DELETE` against a host blocks on human approval and the second does
   not.
5. Exhausting a tier's budget refuses rather than escalates.
6. Deleting a session leaves its audit records readable.
7. No test reaches a live network.

## Remaining detail, decided at implementation

- Exact interval semantics per tier versus the current global interval.
- Body size cap default.
- What "unknown" means for a TCP connection that was accepted but returned
  nothing.
- How request purpose length is bounded.

## Rejected alternatives

- **Literal IPs only.** Broke TLS, and the safest form of request was the one
  that did not work.
- **Banning `DELETE` and other unsafe verbs.** The harness cannot see consequence,
  so the prohibition would block legitimate work and provide no safety; budget,
  audit and the per-host human gate do the actual work.
- **A single flat approval for every request.** Removed the notion of a granted
  scope, and with it any meaningful answer to "expansion beyond the approved
  list".
- **Escalating exhausted budgets.** Invites approval under pressure.
- **Audit records inside the session transaction.** Makes session deletion erase
  the evidence of what its agents did.
