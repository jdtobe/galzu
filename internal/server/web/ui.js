// Pure display rules, shared with the UI tests. No model output is interpreted as HTML.
const GalzuUI = {
  agentState(agent, activity = {}, paused = false) {
    if (agent.status === 'error') return {label:'Needs attention', tone:'warning'};
    if (activity.state === 'retrying') return {label:'Retrying', tone:'warning'};
    if (activity.state === 'inference') return {label:'In inference', tone:'active'};
    if (activity.state === 'queued') return {label:paused ? 'Paused · queued' : 'Queued', tone:''};
    if (activity.last_outcome === 'rested') return {label:'Resting', tone:''};
    return {label:paused ? 'Paused' : 'Idle', tone:''};
  },
  // The bulk retry appears only when there is more than one failed agent;
  // with a single failure the inspector's own control is the right one.
  retryAllLabel(agents) {
    const failed = agents.filter(a => a.status === 'error').length;
    return failed > 1 ? `Retry all failed · ${failed}` : '';
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
  },
  cacheReuse(activity = {}) {
    if (!activity.prompt_bytes) return 'No prompt yet';
    if (!activity.reused_bytes) return 'Cold start';
    const bytes = n => n.toLocaleString();
    return `${activity.reuse_percent || 0}% · ${bytes(activity.reused_bytes)} of ${bytes(activity.prompt_bytes)} bytes`;
  }
};
if (typeof module !== 'undefined') module.exports = GalzuUI;
