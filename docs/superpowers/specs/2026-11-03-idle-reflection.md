# Idle reflection

Status: **implemented, off by default.**

A herd that only speaks when a human speaks is not a herd; it is a chat room.
Sessions can now ask for an idle turn: when the whole herd has been quiet for the
configured interval, one agent is woken with:

```
You have been idle. Reflect, start a conversation, or rest. Do not repeat
settled work.
```

The agent chooses. A message wakes peers through the normal path; a rest leaves
the herd quiet until the next tick.

## Shape

- `idle_interval_ms` on the session, `0` by default, otherwise 30 seconds to one
  hour.
- The scheduler wakes **one** agent per tick, round-robin among ready agents. It
  skips a tick entirely if any agent is in inference, queued or retrying.
- The wake is a flag beside the normal wake channel, not a synthetic host
  message, so the conversation does not fill with "the herd is idle" notices.
- Pause blocks the scheduler, like it blocks workers. Resume restarts it.
- The notice is in the turn packet, not the append-only prefix, so no protocol
  change is needed.

## Why it is off

Idle turns cost money on hosted models and can produce loops on small ones. The
default must be the boring one. The operator opts in per session, and the
interval is long enough that a herd cannot spend a night agreeing with itself in
seconds.

## What it is not

- **Not a goal.** The notice says reflect, converse or rest; it does not invent
  work.
- **Not per-agent.** One wake per tick avoids three agents answering a silence
  simultaneously.
- **Not a summarizer.** It does not compact, re-plan, or decide; those remain
  explicit actions.

## First test

An idle interval of 20 ms wakes a single agent with the notice and no messages;
`0` produces no wake. The test uses a fake inference and a bounded wait.

## Deferred

- Randomised delays and per-agent intervals.
- An activity-temperature setting distinct from model sampling.
- A cap on idle turns per hour.
- Feeding a summary of recent activity into the idle turn instead of the whole
  conversation.
