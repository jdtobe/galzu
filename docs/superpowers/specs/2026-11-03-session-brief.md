# The session brief

Status: **implemented.**

A session carries two pieces of context:

| Field | Question it answers | Where it goes |
|---|---|---|
| `goal` | What should the group do? | First message of the conversation, and the preamble |
| `brief` | What world is this work in? | The preamble only |

The goal is a task. The brief is the domain. They were conflated, and the first
unattended run showed what that costs.

## The failure it fixes

The run's goal said Galzu was "the herd platform you are running inside". The
agents cannot read the repository, so all they had was the noun *herd*. They
designed livestock disease surveillance: *herd health* 26 times, *anomaly* 28,
*outbreak* 20, and a chosen feature about a 12-month series of animal vitals.

That is not a model failure. It is a context failure. A word with a strong
everyday meaning needs a sentence saying which meaning is in play, and no agent
had one.

## Shape

- Optional, at most 16384 UTF-8 bytes, whitespace-trimmed.
- The setup form can append the text of files the operator selects. The browser
  reads them; the server only receives the resulting brief, so this is not a
  file-access tool.
- Supplied at creation and immutable. Changing the world mid-session would
  invalidate every agent's cached prefix and, worse, mean different agents read
  different briefs from the same session.
- Rendered once, in the shared preamble, after the goal:

```
GALZU v3
Session s
Goal: pick the most valuable next feature
Brief: Galzu is a platform for running a herd of local AI agents. Here "herd"
means a group of agents, not livestock. Agents cannot read files or the network.

You are one member of a cooperative herd. ...
```

- Empty briefs render no line at all, so the preamble of an existing session is
  unchanged apart from the protocol version.

## Why the preamble

The brief is the same for every agent and never changes, which is exactly what
the preamble is for: it is shared byte-for-byte and sits in the cached prefix.
Putting it in the first message would work too, but then it competes with
conversation for compaction, and a compacted brief is no brief.

## Protocol

`promptProtocol` was 3 when the brief landed; later actions have bumped it, and
the current value is in `prompt.go`. Any existing session gets one cold prompt
per agent and no migration; nothing stores a version per session.

## What it does not do

- **Not a file dump.** Pasting a README is possible and the file picker makes it
  easier, but the budget is 16 KiB and every byte is re-read each turn. It is for
  orientation and a few key sources, not a repository index.
- **Not editable.** There is no endpoint to change it. If that is wanted later,
  the honest design is a new session rather than a moving world.
- **Not the goal.** A brief that says what to do is a goal in the wrong field;
  the forms keep them separate and the labels say why.
