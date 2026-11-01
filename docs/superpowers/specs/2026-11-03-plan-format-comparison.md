# Plan format: prose against structure

The same plan, written both ways, so the choice is concrete rather than
theoretical. Both examples describe the same work: a Scope panel in the web UI.

## Prose

```
Plan v4

1. Add a Scope panel to the session view. It lists each envelope entry with its
   resolved IPs, the remaining budget per tier, and whether the session is
   armed. Include a Revoke control.

2. Wire grant, arm and revoke to the endpoints that already exist. Keep the
   damage line as a required field before arming.

Done when: a human can grant a scope, arm it, and revoke it from the browser
without using curl.

Acceptance:
- With no scope, the panel says "No scope granted" and offers Grant.
- Arming is refused until a damage line is written, with the reason shown.
- Revoke stops new requests immediately; an in-flight request is reported and
  allowed to finish.
- The panel shows remaining budget per tier after every request.
```

## Structured

```yaml
version: 4
steps:
  - id: s1
    changes: "Scope panel in the session view"
    detail: "Envelope entries with resolved IPs, budget per tier, armed state, Revoke control"
  - id: s2
    changes: "Wire grant, arm, revoke to existing endpoints"
    detail: "Damage line stays required before arming"
done:
  - "A human can grant, arm and revoke a scope from the browser without curl"
acceptance:
  - id: a1
    given: "no scope granted"
    when: "the session is opened"
    then: "the panel shows 'No scope granted' with a Grant control"
  - id: a2
    given: "a scope with an empty damage line"
    when: "the human presses Arm"
    then: "arming is refused and the reason is shown"
  - id: a3
    given: "an armed scope"
    when: "the human presses Revoke"
    then: "no new request is dispatched; any in-flight request is reported and finishes"
  - id: a4
    given: "an allowed request"
    when: "the panel refreshes"
    then: "remaining budget per tier is shown"
```

## The trade-off, honestly

**Prose**

- Models write it well and without instruction; it is how they already answer.
- A human reads it as a plan, which is what it is.
- No parser, so no parse failure, and no silent gap between what was written and
  what the harness thinks was written.
- Acceptance criteria are only as checkable as a human reading them.

**Structured**

- Acceptance criteria become data: the UI can show per-criterion state, and a
  later step can attach evidence to `a1`.
- The model must return valid YAML or JSON inside a JSON string field — two
  escaping layers, and a real failure rate even for good models.
- Every schema change is a protocol change; adding an optional field is easy,
  changing the shape is not.
- A partial parse is worse than no parse: it gives the harness a false list of
  criteria that nobody actually agreed to.

## Recommendation

**Prose for the plan body, with an optional structured acceptance list later.**

The plan's job today is to be read and approved by a human before anything
happens. Structure earns its cost when something *uses* it — per-criterion
evidence, or a gate that refuses a step without a matching criterion. Neither
exists yet, and building the parser first would be the same mistake as building
a protocol nobody speaks.

If structure is wanted, the cheapest useful step is not YAML: it is asking for
one acceptance line per bullet, so `acceptance` is a list of short strings that
a human can tick off. That is parseable with a five-line rule and readable
without one.
