# Reviews requested of a peer

Status: **implemented.**

A finding can be sent to one agent with a question:

```json
{"action":"review","finding":4,"recipient":"<agent id>","text":"Does the evidence survive a cold cache?"}
```

The recipient is woken by a direct message, and the turn packet lists open
reviews addressed to it:

```
Reviews requested of you (finding | requester | question):
4 | Ada | Does the evidence survive a cold cache?
Examining a finding with evidence, a position, or a revision answers the request.
```

## Lifecycle

- A request is `open` until the reviewer **revises the finding, adds evidence,
  takes a position, or withdraws it**. Any of those answers every open request
  from that reviewer for that finding.
- Requests are stored with the finding, so the session's Findings panel shows an
  open request and the export carries it.
- An agent cannot request a review from itself, and a reviewer must be a ready
  agent in the same session.

## What it is not

- **Not a task with a deadline.** There is no timer and no escalation to the
  leader. `available` has a two-minute clock because an idle agent is waiting for
  work; a review is a question a peer may decline by doing something else.
- **Not a gate.** A plan does not wait for reviews, and a finding does not become
  verified when a review is answered. The review is a reason-bearing look.
- **Not private.** The request is a direct message, visible to the human operator
  like every other direct message; the finding itself is shared.

## Why it exists

The briefed run published five findings and recorded one position. Agents
broadcast claims; they did not ask each other to examine them. A direct request
names a peer and a question, which is the smallest thing that turns a broadcast
into a review without inventing a task queue.

## Deferred

- Review deadlines and escalation.
- Requiring a review before a plan.
- Multiple reviewers on one request.
