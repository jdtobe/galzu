// Pure display rules, shared with the UI tests. No model output is interpreted as HTML.
const GarthUI = {
  agentState(agent, activity = {}, paused = false) {
    if (agent.status === 'error') return {label:'Needs attention', tone:'warning'};
    if (activity.state === 'retrying') return {label:'Retrying', tone:'warning'};
    if (activity.state === 'inference') return {label:'In inference', tone:'active'};
    if (activity.state === 'queued') return {label:paused ? 'Paused · queued' : 'Queued', tone:''};
    if (activity.last_outcome === 'rested') return {label:'Resting', tone:''};
    return {label:paused ? 'Paused' : 'Idle', tone:''};
  },
  visibleMessage(message, filter) {
    if (filter === 'public') return !message.recipient_id;
    if (filter === 'direct') return !!message.recipient_id;
    return true;
  },
  deliveryState(message, agent, progress) {
    if (!progress || message.id <= progress.baseline) return 'unknown';
    if (progress.checkpoint != null && progress.checkpoint >= message.id) return 'processed';
    return agent.status === 'error' ? 'needs attention' : 'pending';
  },
  duration(ms) {
    return ms >= 1000 ? `${(ms / 1000).toFixed(1)} s` : `${ms || 0} ms`;
  }
};
if (typeof module !== 'undefined') module.exports = GarthUI;
