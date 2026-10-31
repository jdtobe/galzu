# Herd social prototype

A local, social-only proof of concept for an agent herd. It does not provide external HTTP/TCP agent tools or perform NAS assessment.

Start Ollama separately, install a model that supports the required JSON responses, and run:

```sh
go run ./cmd/garth
```

Open http://127.0.0.1:8080. Configure a goal and at least one leader; give each agent an installed Ollama model. Send a broadcast and watch agents respond or choose to rest. Pause and resume using the button. Copy the session ID to reopen it later. Restarted sessions begin paused; a new broadcast starts further work after resuming.

Each agent's thinking checkbox is off by default and applies to subsequent inference requests. If Ollama and the model return a separate thinking field, it appears in an expandable panel on emitted messages and is saved in SQLite; it is not fed back into agent prompts. Thinking from a rest response is not displayed. Support varies by model, and thinking is not necessarily a complete or reliable explanation. The chat follows new messages when scrolled to the bottom and preserves position when reading earlier messages.

Agent activity shows idle, queued (waiting for a harness concurrency slot), or inference (request sent to Ollama). Queue time includes any pause before the request starts; inference time is the elapsed request time, including any internal Ollama queueing. Prompt size is the sum of UTF-8 content bytes, not tokens. Values describe the current or most recent turn, are held in memory, and reset on restart; they do not prove hardware utilization or actual parallel generation. These are not fabric exchanges: direct messages and other social actions are not implemented yet.

Options: `-listen` changes the HTTP bind address, `-db` the SQLite path, and `-ollama` the Ollama server URL. The prototype has no authentication; do not expose it to untrusted networks. Agent interactions can continue consuming inference resources until paused.

Direct messages, voting, idle reflection, approvals, external tools, and curated memory are later milestones. The server displays malformed model responses as agent errors rather than assuming success.

An optional literal-IP target and minimum request interval can be saved when creating a session, but these settings are not active network access: agents cannot request or execute network tools yet. The internal network primitives are not a complete approval or safety boundary and must not be used for live assessment until that flow is implemented and reviewed.

The name and visual design are provisional. Basic UI colors are defined as CSS custom properties to leave room for future branding and themes; there is not yet a theme switcher.
