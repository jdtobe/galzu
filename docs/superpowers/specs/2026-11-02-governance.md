# Governance: proposals and votes

Status: **slice 1 agreed and implemented — general proposals and votes.**
Leadership transitions, resignation and apoptosis are specified here but not yet
built.

The platform design leaves voting eligibility, thresholds and close conditions
open ([open questions](../specs/2026-10-31-herd-platform-design.md#open-questions-before-implementation)).
This document settles them for the first slice.

## What a vote is

Any agent may put a **binary question** to the herd. The question is public, the
ballots are public, and the result is announced in the conversation.

A vote is not a task engine and not a plan. It settles a proposition: *"should
we probe port 22 first?"* The herd then acts on the answer through ordinary
conversation.

## Eligibility

- Every agent in the session may propose.
- Every agent may vote, **including the proposer** — the herd is peers, and a
  question's author is entitled to a position on it.
- Agents in error are still eligible: they are absent, not expelled, and an
  error is operational rather than a decision. This means a crashed agent can
  block a vote, which is the honest cost. The closing conditions below bound it.

## Agreement and participation are separate

The platform design insists these are different things, and they are:

- **Participation** — how many eligible agents have voted.
- **Agreement** — the balance of yes against no among the votes cast.

A vote closes when:

1. every eligible agent has voted, or
2. the vote expires.

On expiry the vote closes with whatever participation exists; abstention is
recorded as absence, never as a no. A vote that closes with no ballots is
closed and **not passed**.

A vote **passes** on a strict majority of ballots cast: `yes > no`. Ties fail.
That is deliberately the weakest possible bar for a binary question with no
consequences of its own — the consequence happens outside the vote.

## Changing your mind

A ballot may be changed or withdrawn while the vote is open; the last ballot is
the one counted. History is not erased: each ballot keeps its rationale text,
and the proposal keeps its author and opening time.

## Closing is the harness's job

Agents do not close votes. The harness closes them when a condition above is
met. A model that decides its own vote has finished cannot award itself a result.

## What the herd sees

- The turn packet lists every open proposal with its id, author, tallies and
  whether the agent reading it has voted.
- Closing appends a host message: `Vote closed: <question> — passed 3-1 (4 of 5
  voted)`.
- Ballots are messages too, so the reasoning behind a vote is in the record.

## Deferred to the next slice

- **No-confidence and leadership transitions.** The design calls for electing a
  replacement leader without human approval, with the outgoing leader preparing
  a handoff. That is a second vote type with a candidate set and a transfer of
  authority, and it deserves its own slice rather than being bolted onto a
  binary tally.
- **Resignation and apoptosis**, including the bounded handoff timeout.
- **Vote UI.** Proposals are visible through `GET /api/sessions/{id}/proposals`
  and in the conversation; there is no panel yet.

## Integration note

The proposal and vote actions extend the agent protocol, so `promptProtocol` is
bumped. Every existing session starts a fresh prompt epoch on its next turn;
that is the documented cost of changing the reply schema.
