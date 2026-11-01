# Galzu roadmap

Status is honest: this is a pre-alpha, single-operator application. The list is
what to do next, not a promise.

## The loop today

1. A human starts a session with a goal, an optional brief (up to 16 KiB, with
   browser-read reference files), agents and models.
2. Agents converse, ask for help or capacity, publish findings, review one
   another, recall shared findings, propose questions, vote, elect leaders and
   propose plans.
3. The human reads the evidence and the plan in the Decisions panel, approves or
   rejects it, and later marks it implemented.
4. Everything is saved to SQLite, exportable as Markdown, and findings are
   browsable across sessions.

The harness enforces what it can: one leader, one pending plan, a passed proposal
before a plan may cite one, budgets and approvals around the network gate, and a
storage boundary. It does not enforce agreement, truth, or good ideas.

## Next, in order

1. **Ground the herd in the repository.** The brief can already carry selected
   files. The next step is a bounded, human-approved retrieval of source context
   so plans stop inventing functions and tables. No agent filesystem access
   unless the tool policy gains a read-only, scoped variant.
2. **Make review meaningful.** Review requests exist, a finding is labelled peer
   reviewed at two non-author positions, and the operator can mark a finding
   verified. What remains is deciding whether a plan should be gated on a review
   and how a human mark interacts with a disagreement.
3. **Retrieval that ranks.** `recall` returns literal matches in one session;
   the library searches across sessions. Add relevance ranking and, if wanted,
   recall across sessions, with the same rule: a retrieved claim is evidence, not
   authority.
4. **Vote closure controls.** A paused session leaves votes open; the timer only
   runs while the herd does. Decide whether an operator may close a vote with
   current participation, and record who closed it.
5. **Apphood, if the herd earns it.** Authentication, a deploy story, migrations
   and a second user are deliberately absent. Decide after the next comparison
   whether to build a product around the herd or keep it a research instrument.
6. **Activate the tool gate only with the approval flow reviewed.** The gate,
   scopes, budgets and audit exist; no agent action reaches them. This is a
   safety boundary, not a missing feature.

## Open question

The first briefed run fixed the domain but not the judgment: five findings
produced one position, four questions produced one ballot, and the approved plan
implemented a candidate its own leader had argued against. The solo 9B control
was more coherent than the three-agent herd.

The next experiment should change the *material*, not add agents: a stronger
model, a role that requires verification, or agents with different sources. More
small agents added turns, not evidence. See
`docs/research/herd-run-2026-11-03.md`.

## Known limits

- Small local models; mixed model sizes can serialize or swap on one Ollama
  instance.
- No authentication, deploy story, migration framework, or backup beyond the
  database file. CI runs vet, the race tests and the UI unit tests.
- The database must not live in a cloud-synced folder; the default avoids it.
- Agreement is not verification, and a plan is prose; neither is parsed into
  authority.
