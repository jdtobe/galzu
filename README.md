# Herd social prototype

A local, social-only proof of concept for an agent herd. It does not provide external HTTP/TCP agent tools or perform NAS assessment.

Start Ollama separately, install a model that supports the required JSON responses, and run:

```sh
go run ./cmd/garth
```

Open http://127.0.0.1:8080. Configure a goal and at least one leader; give each agent an installed Ollama model. Send a broadcast or select an individual recipient and watch agents respond or choose to rest. Pause and resume using the button. Copy the session ID to reopen it later. Restarted sessions begin paused; after resuming, agents process messages not yet handled. Replies and per-agent checkpoints are saved together; an inference interrupted before its reply is saved can be retried. Existing sessions without checkpoints start at the end of their history when first reopened after upgrading.

The session setup checkbox requests thinking for all agents from their first turn; it is off by default. Individual agent checkboxes can be changed during a session and apply to subsequent inference requests. If Ollama and the model return a separate thinking field, it appears in an expandable panel on emitted messages and is saved in SQLite; it is not fed back into agent prompts. Thinking from a rest response is not displayed. Support varies by model, and thinking is not necessarily a complete or reliable explanation. The chat follows new messages when scrolled to the bottom and preserves position when reading earlier messages.

Agent activity shows idle, queued (waiting for a harness concurrency slot), or inference (request sent to Ollama). Agents also receive an advisory snapshot of peer activity in their roster, not a guarantee of availability. Queue time includes any pause before the request starts; inference time is the elapsed request time, including any internal Ollama queueing. Prompt size is the sum of UTF-8 content bytes, not tokens. Values describe the current or most recent turn, are held in memory, and reset on restart; they do not prove hardware utilization or actual parallel generation.

Options: `-listen` changes the HTTP bind address, `-db` the SQLite path, and `-ollama` the Ollama server URL. The prototype has no authentication; do not expose it to untrusted networks. Agent interactions can continue consuming inference resources until paused.

Agents receive a roster and can broadcast, send direct messages, or request help publicly or from one agent. Direct content is included only in the sender's and recipient's contexts; other agents see an activity notice, not the content, when they next process recent history. The human operator can see private content in the chat and database. Invalid or unavailable recipients are rejected and retried with host feedback. These are lightweight messages, not tracked assignments or guaranteed replies; models may not reliably use the available actions. Voting, idle reflection, approvals, external tools, and curated memory are later milestones. Agents that exhaust recovery attempts appear as errors; use Retry agent to retry from the last checkpoint after addressing the cause. Retrying while the herd is paused waits for resume.

Inference failures and invalid model actions receive up to three automatic retries after the initial attempt, with jittered delays of approximately 2, 10, and 30 seconds. Attempts time out after 10 minutes. Retry state and bounded failure history persist across restarts; prompts are rebuilt for each attempt, and validation feedback is visible only to the affected agent. The operator can expand failure details, including rejected output that may contain private information. If all attempts fail, the agent needs manual retry and its messages remain pending. Context-window errors recognized from Ollama trigger smaller message batches and a notice that older history is saved; direct historical search is not yet available. Context-limit detection depends on Ollama's error wording and cannot guarantee recognition of every model or server response. Retry schedules and timeouts are not configurable yet. Storage-failure protection and per-message delivery indicators are not yet implemented.

An optional literal-IP target and minimum request interval can be saved when creating a session, but these settings are not active network access: agents cannot request or execute network tools yet. The internal network primitives are not a complete approval or safety boundary and must not be used for live assessment until that flow is implemented and reviewed.

The name and visual design are provisional. Basic UI colors are defined as CSS custom properties to leave room for future branding and themes; there is not yet a theme switcher.
