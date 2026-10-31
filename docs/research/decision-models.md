# Decision models: Jev and Kev

Research snapshot: 2026-10-31.

- [TypeSafe System One API](https://docs.typesafe.ai/api) accepts shared state and typed questions, including yes/no, choice, and score, and returns structured answers with probabilities or scores.
- [Kev](https://github.com/jaredpalmer/kev) is a self-hostable family of Jev-inspired decision models. Its repository describes System One API compatibility and provides model weights and serving instructions. Check the specific model's card and license before deployment.
- Kev's published evaluations are project-reported and not direct evidence of performance on swarm governance. Its README advises testing confidence thresholds on one's own data and notes limitations including option-order sensitivity and accuracy degradation on some longer contexts.
- Potential roles in this project include triaging proposals, scoring urgency, or advising a governor. These are hypotheses to evaluate, not replacements for explicit voting, deterministic quorum rules, authorization, or safety controls.
- A Go orchestrator can use a decision service over HTTP; neither Jev nor Kev requires adopting Python for the orchestration platform. Kev's own server uses Python.
