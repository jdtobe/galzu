# Asking the herd for work

Status: **implemented.**

An agent that has capacity asks the **fabric first** and the **leader second**.
That ordering is the point: the herd is where work is discovered, and the leader
is a fallback for when the fabric is silent, not a dispatcher every request goes
through.

This follows the platform design's line that work coordination starts
informally through conversation, not a formal task engine.

## The action

```json
{"action":"available","text":"I have capacity; I can take documentation or test work.","memory":"..."}
```

It is a message of kind `available`, so it is visible in the conversation and in
the prompt log like any other, and it opens a **work request**.

`available` is deliberately distinct from `help`:

| Action | Meaning |
|---|---|
| `help` | I am stuck on work I already have |
| `available` | I have no work and want some |

Merging them would hide the difference between an agent that needs rescuing and
one that needs a task.

## What counts as an answer

Any message from **another agent** after the request: a peer offering to
collaborate, a peer handing over a task, or the leader allocating one. The
requester's own messages never count, or an agent could answer itself.

Host messages do not count either, and that is deliberate: the escalation
message *is* a host message, so counting it would make every escalation resolve
the request it was raised about.

## Escalation

After two minutes with no answer, the harness once appends:

> Beau has offered capacity for 2 minutes with no answer from the herd. Ada,
> allocate work or say why.

The request is then marked `escalated` and is not raised again. A second
escalation would be nagging, and the leader's silence is itself information the
human can see.

If no agent holds the leader role, the message says so instead of naming nobody.

## What this is not

- **Not a task queue.** No work item is created, claimed, leased or completed.
  The request is a message with a clock on it.
- **Not enforced.** The leader can ignore it; the escalation makes that visible
  rather than impossible.
- **Not a scheduler.** An escalated request does not resume the agent, assign it
  anything, or change its status.

## Why the timer is short

Two minutes is roughly a few turns of a small herd. Long enough that a peer
mid-inference can answer, short enough that an idle agent is not parked for the
length of a session. It is a single constant and easy to revisit once real runs
show what a "turn" costs in wall time.

## Deferred

- **Claiming.** A peer that offers to collaborate cannot formally take the work,
  so two agents may duplicate effort. A claim/lease would fix that and is the
  natural next step.
- **Withdrawal.** An agent cannot cancel its own offer; it can only say so in
  conversation.
- **Starvation visibility.** Nothing aggregates how often an agent's offers go
  unanswered. That is the kind of thing a human should see before it becomes a
  pattern.
