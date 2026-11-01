# Plan artifact and human review

Status: **slice 1 implemented** — agents propose a plan, the human approves or
rejects it, and the approved plan is what the herd is told to work against.

The PoC success test requires that a herd "propose a plan for human review"
before executing. The platform design already says the goal, plan, definition of
done and acceptance criteria are session artifacts rather than a rigid workflow;
this makes the plan one.

## What a plan is

A **versioned** text artifact attached to a session. It is the thing the human
approves, and the thing agents are told they are working against.

A plan should contain three things, and the prompt asks for them explicitly:

1. the steps the herd intends to take,
2. a definition of done,
3. acceptance criteria — how the human can tell it worked.

The harness does not parse these. Structured extraction from model prose is a
guessing game, and a wrong parse would give false authority to text that was
never actually agreed. The human reads the plan and approves the whole thing.

## Lifecycle

| State | Meaning |
|---|---|
| `pending` | Proposed, waiting for the human |
| `approved` | The current plan the herd works against |
| `rejected` | Seen and refused, with the human's note |
| `superseded` | A later proposal replaced it before a decision |

- Any agent may propose a plan.
- **Only one plan is pending at a time.** A new proposal supersedes the previous
  pending one rather than queueing, because a pile of competing plans is how a
  herd stalls.
- An approved plan stays approved until a new one is approved. Rejection never
  removes the previous approved plan, so the herd always has *some* stated plan
  or none.
- The human decides with a note; the note is shown to the herd.

## What approval gates

Nothing yet, and that is deliberate. External tools are not wired to agents, so
there is no execution to gate. What approval does today is establish the
agreement and put it in the prompt: every agent sees whether the plan is
approved, pending, or absent, and sees the human's note on a rejection.

When tools are wired, the gate will check that an **approved** plan exists
before an action can be dispatched. That check is specified in
[the tool policy](2026-11-01-tool-policy.md) and belongs there, not here.

## Visibility

- The turn packet carries the current status: `Plan: approved (v2)` or
  `Plan: pending review (v3)` or `Plan: none`, plus the body of the approved
  plan.
- Proposing a plan posts it to the conversation as well, so the reasoning is
  visible next to the plan.
- `GET /api/sessions/{id}/plan` and `POST /api/sessions/{id}/plan/decide` are the
  operator's side.

## Deferred

- Editing a plan in the UI, and structured step tracking.
- Per-step acceptance, and evidence attached to each criterion.
- Requiring a plan before *conversation* continues — the design says planning is
  not a tool-free phase, and gating talk on approval would make the herd useless
  while waiting.
