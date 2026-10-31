# Social Herd Vertical Slice Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `executing-plans` to implement this plan task-by-task, or `subagent-driven-development` if explicitly selected by the user. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run and observe a small, locally hosted herd that exchanges messages and offers help through Ollama, with a web interface and durable pause/resume.

**Architecture:** A single Go server persists sessions, agents, messages, and an activity log in SQLite. Per-agent loops call Ollama's native chat API and exchange structured social actions through a server-owned fabric; an HTTP API and server-sent events power a minimal embedded web UI.

**Tech Stack:** Go 1.24+, SQLite via `modernc.org/sqlite`, Go standard-library HTTP and `embed`, Ollama `/api/chat`, plain HTML/CSS/JavaScript.

**Spec:** `docs/superpowers/specs/2026-10-31-herd-platform-design.md`

## Milestone boundary

This first slice deliberately excludes external HTTP/TCP agent tools, leader approvals, voting/elections, formal findings, curated memory/search, random idle reflection, system notifications, model downloads, and NAS assessment. A leader is an initial role and cultural instruction, not yet an authority mechanism. Agents communicate through public herd messages only; private messaging is a later milestone. Human input is broadcast only. Inference is local/configurable through Ollama, without another provider adapter yet. Resume means restarting the same persisted session with its same agents and message history, not restoring hidden model state or interrupted inference.

These exclusions are intentional sequencing, not changes to the approved overall vision. Do not connect this slice to the live NAS or claim it satisfies the complete PoC until later milestones add the agreed plan review and external tools.

## Global Constraints

- Single-user, single-machine Go server; bind to `127.0.0.1` by default.
- Use an append-only activity record and SQLite-backed current state; preserve committed records across restart.
- Agents have individual names, roles, and model identifiers; a session has a configurable concurrency limit.
- Human messages and agent messages are durably recorded and visible in the UI.
- Global pause blocks new inference calls and message publication immediately; already running calls may finish, but their results are held until resume.
- No dependency on real Ollama in automated tests; use an HTTP test server or a fake inference client.
- No git commits are prescribed: the current workspace is not a Git repository.

## Review Focus

1. Ollama unavailable or returning malformed output: pause only the affected agent, persist the failure, and display it.
2. Pause while inference is in flight: never publish the result until resumed; do not start another call.
3. Restart during an in-flight call: do not assume the call completed; resume from durable messages without duplicating a committed message.
4. Empty, oversized, or malformed HTTP input: reject with a useful 4xx response without changing state.
5. Slow or disconnected event-stream clients: do not block persistence or other clients; reconnect by fetching the current snapshot.

## File map

- `go.mod`, `go.sum`: module and SQLite driver.
- `cmd/garth/main.go`: flags, composition, server lifecycle.
- `internal/store/store.go`, `internal/store/store_test.go`: SQLite schema, atomic mutations, and ordered events.
- `internal/ollama/client.go`, `internal/ollama/client_test.go`: native Ollama request and response handling.
- `internal/herd/runner.go`, `internal/herd/runner_test.go`: agent scheduling, social actions, concurrency, and pause/resume.
- `internal/server/server.go`, `internal/server/server_test.go`: validated HTTP API and SSE notifications.
- `internal/server/web/index.html`, `app.js`, `style.css`: embedded minimal interface.
- `README.md`: setup, limitations, and manual smoke test.

## Shared contracts

- `store.Session`: `ID string`, `Goal string`, `Status string` (`running` or `paused`), `Concurrency int`.
- `store.Agent`: `ID string`, `SessionID string`, `Name string`, `Role string` (`leader` or `helper`), `Model string`, `Status string` (`ready` or `error`), `Error string`.
- `store.Message`: `ID int64`, `SessionID string`, `AgentID string` (empty for human), `Body string`, `CreatedAt time.Time`.
- `store.Event`: `ID int64`, `SessionID string`, `Kind string`, `Payload json.RawMessage`, `CreatedAt time.Time`.
- `ollama.Client.Chat(ctx context.Context, model string, messages []ollama.Message) (ollama.Reply, error)`; `Message` has `Role` and `Content` strings; `Reply` has `Action` (`message` or `rest`) and `Text string`.
- `herd.Runner.Start(ctx context.Context, sessionID string) error`, `Pause(sessionID string) error`, `Resume(sessionID string) error`, and `Stop()`.
- Social output is a JSON object in the assistant message: `{"action":"message","text":"..."}` or `{"action":"rest","text":""}`. A `message` is a public contribution, including offers of help; `rest` means no contribution. Reject other actions, empty messages, or messages over 4096 bytes. Include this contract and shared cultural instructions in each agent's system prompt.

---

### Task 1: Durable session and activity store

**Files:** Create `go.mod`, `internal/store/store.go`, `internal/store/store_test.go`.

**Interfaces:** Produce `store.Open(path string) (*Store, error)`, `Close() error`, `CreateSession(ctx context.Context, session Session, agents []Agent) error`, `GetSession(ctx context.Context, id string) (Session, []Agent, error)`, `SetSessionStatus(ctx context.Context, id, status string) error`, `SetAgentStatus(ctx context.Context, id, status, detail string) error`, `AppendMessage(ctx context.Context, sessionID, agentID, body string) (Message, error)`, `Messages(ctx context.Context, sessionID string, afterID int64) ([]Message, error)`, and `Events(ctx context.Context, sessionID string, afterID int64) ([]Event, error)`.

- [ ] Write tests for transactional creation of a session and its agents; append human and agent messages; reopen a file-backed database and assert the same ordered messages and increasing activity event IDs. Assert a failed creation rolls back all agents and the session.
- [ ] Run `go test ./internal/store -v`; expect failure because the module/store do not exist.
- [ ] Initialize module `github.com/jdtobe/garth`, add `modernc.org/sqlite`, and implement the interfaces. Store each status change and message with its activity event in the same SQLite transaction; create schema on open. Return an error for unknown IDs and invalid statuses.
- [ ] Run `go test ./internal/store -v`; expect all tests to pass.

### Task 2: Ollama social inference client

**Files:** Create `internal/ollama/client.go`, `internal/ollama/client_test.go`.

**Interfaces:** Produce `ollama.New(baseURL string, client *http.Client) *Client` and the shared `Chat` contract. POST `/api/chat` with the configured model, `stream: false`, `format: "json"`, and supplied messages; decode the assistant content as the social JSON contract.

- [ ] Write tests using `httptest.Server` asserting model, request path, and `stream: false`; return a valid message and assert it decodes. Add cases for non-2xx, invalid JSON, invalid action, empty text, oversized text, and context cancellation; each must return an error.
- [ ] Run `go test ./internal/ollama -v`; expect failure because the client does not exist.
- [ ] Implement `New` and `Chat`; bound response reads and return descriptive errors without inventing an action.
- [ ] Run `go test ./internal/ollama -v`; expect all tests to pass.

### Task 3: Social runner and global pause

**Files:** Create `internal/herd/runner.go`, `internal/herd/runner_test.go`.

**Interfaces:** Produce `herd.New(store *store.Store, inference Inference) *Runner` and the shared runner methods; `Inference` is the `Chat` method from the shared contract. Calls to `Start` and `Resume` are idempotent for an already running session; `Stop` releases all workers.

- [ ] Write deterministic tests with fake inference: a human message triggers each ready agent once; a returned social message is stored and becomes available to other agents; a `rest` reply does not publish a message. Use per-agent message cursors held in memory for the current process, advancing after processing, and on restart initialize cursors to the last committed message to avoid replay; new human input triggers new work. Ensure agents do not respond to their own messages and cap scheduling to prevent unbounded response loops: an agent processes each new message ID at most once per process.
- [ ] Add tests that block an inference call, pause, release the call, and assert no result is published and no new call starts until resume; then assert the held result is published once. Simulate restart during an unfinished call and assert no result is fabricated or duplicated. Verify configured concurrency is never exceeded.
- [ ] Add a test that inference failure sets only that agent to `error`, records its error, and leaves other agents able to proceed. A restart does not automatically retry agents in `error`.
- [ ] Run `go test ./internal/herd -v`; expect failure because the runner does not exist.
- [ ] Implement the runner, using an execution lock to serialize the pause transition with result publication. Pause persists session status before returning. Hold results in memory while paused; if the process exits, discard uncommitted results and treat their remote outcome as unknown. Persist committed output only once. Provide the goal, cultural instructions, and a bounded recent history of up to 50 messages to inference; prefix messages with the sender name.
- [ ] Run `go test -race ./internal/herd -v`; expect all tests to pass without race reports.

### Task 4: HTTP API and push updates

**Files:** Create `internal/server/server.go`, `internal/server/server_test.go`, `cmd/garth/main.go`.

**Interfaces:** Produce `server.New(store *store.Store, runner *herd.Runner) http.Handler`; expose `POST /api/sessions`, `GET /api/sessions/{id}`, `POST /api/sessions/{id}/messages`, `POST /api/sessions/{id}/pause`, `POST /api/sessions/{id}/resume`, and `GET /api/sessions/{id}/events`. Session creation accepts `{goal, concurrency, agents:[{name, role, model}]}`; snapshot returns `{session, agents, messages}`. Message creation accepts `{body}`.

- [ ] Write API tests creating a session with one leader and one helper, retrieving its snapshot, broadcasting a human message, pausing, and resuming. Reject missing goal/model, no agents, anything other than exactly one leader, unknown roles, concurrency outside 1–32, empty or over-4096-byte messages, malformed JSON, and bodies larger than 64 KiB with 4xx and no state mutation.
- [ ] Write an SSE test proving a committed message emits a notification; a disconnected or slow client must not block message creation or other clients. Event notifications carry event IDs and kinds only; reconnecting clients fetch a full snapshot, so dropped notifications are safe. Test unknown session IDs return 404 before opening a stream.
- [ ] Run `go test ./internal/server -v`; expect failure because the server does not exist.
- [ ] Implement endpoints and SSE. Use bounded per-client notification channels, dropping notifications for slow clients; the UI must refresh its snapshot on connect and after a notification. Generate IDs using `crypto/rand`. `cmd/garth` accepts `-listen` (default `127.0.0.1:8080`), `-db` (default `garth.db`), and `-ollama` (default `http://127.0.0.1:11434`); on startup restore running sessions as paused so work never silently resumes after restart.
- [ ] Run `go test -race ./internal/server ./internal/herd -v`; expect all tests to pass.

### Task 5: Minimal web UI and end-to-end smoke test

**Files:** Create `internal/server/web/index.html`, `internal/server/web/app.js`, `internal/server/web/style.css`, `README.md`; modify `internal/server/server.go` and its tests.

**Interfaces:** Embed the UI with `//go:embed web/*` and serve it at `/`. The UI uses Task 4's API and SSE only.

- [ ] Add a server test that `/` returns HTML containing `Herd` and `/app.js` returns JavaScript. Add an end-to-end test with a fake inference client: create a session, send a human message, observe an agent reply, pause, reconnect to events, resume, and verify the snapshot retains the conversation.
- [ ] Run `go test ./internal/server -v`; expect the new UI test to fail.
- [ ] Implement a session creation form for goal, concurrency, and configurable agent names, roles, and models; an agent roster with status/error; a public conversation; a broadcast composer; and global pause/resume controls. Include an existing-session ID field to reopen a persisted session. On SSE reconnect, fetch a fresh snapshot; escape message text by inserting it as text, not HTML. Label the interface as a social-only prototype and expose no NAS or external network controls.
- [ ] Document `go run ./cmd/garth`, Ollama configuration, the limitations in this plan, and a manual two-agent smoke test. Do not claim actual model names have been verified.
- [ ] Run `go test -race ./...` and `go vet ./...`; expect both to pass. Run `go run ./cmd/garth -h`; expect the documented flags. If Ollama is available, perform and report the manual smoke test; otherwise explicitly record that live inference was not verified.

## Follow-on milestones (not implemented by this plan)

1. Leader-reviewed external HTTP/TCP tools, target policy, human approvals, and plan review; define and test safety boundaries before any live NAS assessment.
2. Voting, no-confidence, leadership transitions, retirement/handoffs, and apoptosis.
3. Direct messages, threads, findings, richer memory, activity summaries/checkpoints, idle reflection, and the corresponding UI.
