# Herd platform: initial design

Status: **proposed; all decisions remain in flux**. This document describes the initial proof of concept (PoC), not an implementation plan.

## Purpose and success

Build a host framework for a community of AI agents that can explore problems, communicate, seek and offer help, share knowledge, and coordinate decisions. The central experiment is whether the interactions feel like a useful herd rather than a collection of isolated chatbots. Agents should have room to guide their own work; a human remains available for intervention and consequential decisions.

The PoC succeeds when a small herd can start from a human-provided goal, converse and volunteer help, route external tool requests through its leader, propose a plan for human review, and pause and resume, while the human can observe and intervene. A single-agent benchmark is optional future work, not a PoC gate.

## Initial scenario

The first scenario is an assessment of the operator's live NAS, starting with its host address. The agents are expected to investigate within configured limits, develop their own definition of done and acceptance criteria, and present a plan for human review before executing it. They can use available tools while preparing, subject to policy; planning is not a tool-free phase.

The intended flow is initial analysis and attack options; human approval of a list of validation options; leader authorization of probes within that list; reporting on evidence, viability, and opportunities; and a final attack directed by the human. Expansion beyond the approved validation list goes to the human. A plan and its criteria may evolve, with consequential changes brought to the human.

The target boundary, rate limits, global pause, and applicable approvals are harness-enforced. Research on other hosts is distinct from authorization to assess them. The leader decides on external requests within policy and escalates as needed. Avoiding lasting damage is a goal, not a guarantee; agents should promptly report mistakes to the herd and the human. The exact initial network policy and treatment of ambiguous targets need design and testing before a live run.

## Agents and culture

The human selects the starting agent count and model configuration. Initially one agent is leader and several are helpers; they share the goal and cultural instructions, with the leader additionally responsible for facilitation and decisions when necessary. Everyone is a peer: honest questions and dissent are welcome, evidence and correctness matter more than popularity, and the group shares responsibility.

Agents independently pursue work, solicit help, volunteer, negotiate and review one another's contributions. Assignment is normally negotiable, including when requested by the leader; the leader may override with a recorded rationale, and the recipient can still raise concerns. The leader has room to coordinate beyond the original plan, but cannot bypass enforced safety controls or required human approvals. Work is reported back to the requester and useful findings can be shared more broadly.

The leader may resolve deadlocks on explicitly low-consequence matters when confident; otherwise it escalates. It decides whether acceptance criteria are met, preserving dissent and evidence for human review. Agents can suggest policy improvements; changes to authority or safety boundaries require human approval.

Agents are ephemeral individuals for now. Session resume restores the agents belonging to an ongoing effort; retiring an individual ends that instance. Reusable individual templates or saved starting states, such as mentor or guide, are later possibilities.

## Fabric and knowledge

Provide a common social fabric for broadcasts, direct messages, discoverable open threads, requests and offers of help, findings, votes, and conversation. Agents can share knowledge voluntarily or in response to a request; they cannot directly search each other's unshared memories. Public threads and published findings are discoverable by the herd. Relationship-based visibility and bounded contexts are future experiments.

Record activity for human review. Agents receive intermittent summaries of relevant public activity, not every event injected into every context. Each agent has a persisted checkpoint for updates it has received; summaries arrive at a safe pause. Other agents can see that private actions occurred without receiving private contents, and can ask the actor for details. How summaries are selected and what metadata is visible remain design questions.

A shared finding starts with a claim, author, supporting or opposing evidence, and links to relevant conversations. Agents can challenge, revise, and vote on it without erasing history. Agreement is distinct from verification; votes do not by themselves make a claim true.

Work coordination starts informally through conversation, not a formal task engine. A lightweight, inferred availability indicator is advisory rather than a commitment. Agents can request help or approach colleagues; others can volunteer. The goal, plan, definition of done, and acceptance criteria are visible session artifacts rather than a rigid workflow.

## Agent loop and memory

The host runs each agent's loop, connecting to a configured inference endpoint. Agents continue their chosen work until they consider it complete, subject to harness scheduling and controls. Between steps, they check messages; ordinary messages need not cause a task switch. Explicit interrupts receive prompt attention at the earliest safe opportunity.

While idle, agents receive randomized opportunities to reflect, initiate conversation, or remain silent. A herd-wide activity-temperature setting influences wake-up frequency; this is distinct from model sampling temperature. Messages and tool results can wake agents as well. The harness limits agent count and inference concurrency; endpoint queues manage physical capacity. Time and token budgets are not required initially.

Persist session history separately from model context. The initial context strategy retains recent activity, lets agents explicitly mark useful memories, and permits searching one's own older history and retrieving relevant shared findings. The harness manages context size; agents help choose what to retain. Selection, compaction, model-specific limits, and backlog handling need further design.

## Voting, leadership and departure

Any agent can propose a vote and participate, change, or retract its vote while open. Configurable rules distinguish sufficient participation from sufficient agreement. Closing a vote records a settled result; later change requires another vote. Vote aging is a future idea. Quorum mechanics, electorate definition, closing conditions, and deadlock handling remain to be specified.

Agents may propose and vote on team changes; initially the human approves those changes. Agents can propose a no-confidence vote, and the herd can elect a replacement leader without human approval. In a normal leadership transition, the current leader prepares a handoff and remains in place until a successor is selected where possible.

A leader may propose retirement with a justification and optionally recommend a successor with separate justification. The herd votes separately on retirement and successor; neither is guaranteed. If retirement is rejected, the leader may re-propose with better justification.

Any agent may initiate emergency self-termination, called **apoptosis**, as an absolute last resort after trying to resolve the situation or obtain help. It need not win a retirement vote. The reason and available handoff are preserved, and the herd and human are notified. If the leader terminates, the herd initiates successor election; leader-only decisions wait or go to the human. Operational failure that prevents an agent from acting must be surfaced rather than misrepresented as a deliberate decision.

Normal individual retirement is graceful but bounded by a configurable timeout. The agent gets an opportunity to finish its in-flight action and prepare a handoff. On timeout the harness proceeds with a clearly marked incomplete handoff based on available records. The leader reviews and decides whether to reallocate work. An external operation that cannot be interrupted may still complete; the harness must track its outcome.

## External tools and approvals

The initial external tools are HTTP, supporting all methods, and direct TCP sockets. UDP is a later possibility. No general shell or file-writing tools are included initially. The NAS scenario is limited by what these tools can reach.

The leader reviews every external tool request for the PoC, including its own. A request includes target, intended action, purpose, and known risks. The leader can approve, reject, ask for clarification, or escalate to the human with its recommendation and reasoning. Human approvals initially apply to the specific action, unless broader permission is explicitly granted. Leader judgment cannot override harness boundaries, the approved validation list, or other required approval gates.

The requester and leader receive the full result. The herd sees an activity notice, may request details, and may publish relevant findings. A failed external tool call goes to the leader before a retry. Individual cancellation of an approved but queued request is deferred; global pause still prevents new actions from starting. Agent inference failure pauses the affected agent and notifies the human; unrelated agents can continue if possible.

HTTP methods alone do not prove an action is read-only, and raw socket traffic can have arbitrary effects. A future implementation must enforce restrictions consistently across tools and avoid treating leader approval as permission to assess unauthorized targets. Exact policy semantics are unresolved and require explicit review before live use.

## Human interaction and pause

The human can inspect an agent's observable activity, messages, decisions, tool calls, and available rationale; hidden model reasoning is not assumed to be accessible. The human can message an individual or broadcast to all. Ordinary messages arrive at the next safe pause; an explicit interrupt requests urgent attention. Group-targeted communication is a future feature.

The harness enforces global pause independently of agent compliance: stop scheduling new work or tool calls immediately, notify all agents, and attempt to interrupt in-flight operations where possible. Operations that cannot be interrupted may finish; their results are held without follow-on work. Clearly distinguish pausing from fully paused. The human can inspect, resume, or redirect; a new effort can abandon the prior paused execution.

The web UI exposes pending attention items and supports browser notifications when available. The in-app queue remains the place to review pending requests. Rough UI directions have been explored for agent inspection and messaging, leader-escalated requests, and threaded discussions with votes; they are interaction sketches, not final visual designs.

## Architecture and persistence

Start with a modular Go monolith containing the agent loops, fabric, governance, persistence, documented HTTP API, and web server. The browser is a client; authority resides in the server. Server-sent events are the provisional browser push mechanism, with HTTP for commands. Internal event delivery should not require agents to poll continually.

Use SQLite as the initial local source of truth, behind a persistence boundary. Store an append-only activity record alongside queryable current state; full event sourcing is not required. Persist enough state during an effort to support resume and represent uncertain external operations. Startup offers resume an effort, start a new effort with an existing team configuration, or start fresh.

Each agent has independent model/provider configuration. Connect to the existing local Ollama server first, with a provider adapter boundary for later services. The user's initial machine is a Mac Studio M2 Max with 32 GB unified memory; `lfm3:1.7b` is a candidate for social experiments. Verify model availability and tool-calling behavior rather than assuming either. The host is provider-independent and does not manage model downloads or inference engines in the PoC.

Initial deployment is one human on one machine, with server and UI co-located. Networked teams and human/agent responsibility boundaries are future work. No account or authentication decision is settled by that deployment assumption.

Jev and self-hosted Kev are possible future decision-service integrations. They may advise a governor or assess structured questions, but do not replace votes, deterministic quorum rules, authorization, or safety controls. See `docs/research/decision-models.md`.

## Open questions before implementation

- Define precise scope, rate limiting, approval, and audit semantics for HTTP, TCP, redirects, DNS, and research versus assessment targets; review them before use on the live NAS.
- Specify voting eligibility, participation and agreement thresholds, close conditions, election transitions, and handling of leader unavailability.
- Decide how the host coordinates the leader's review with concurrent agent loops and tool execution without creating deadlocks.
- Specify activity visibility, checkpoints, summary generation, memory retrieval, context compaction, and protection of sensitive logs.
- Define persistence and recovery behavior, especially operations with uncertain outcomes and tool results arriving during pause or retirement.
- Explore UI flows and responsive behavior in more detail; existing mockups are illustrative, not a final design.
- Decide what minimum slice of the broader governance and memory behavior is needed to demonstrate the agreed PoC success criteria.
