let current = '', stream, status = '';
let refreshSequence = 0;
let currentAgents = [];
const $ = id => document.getElementById(id);
const drafts = new Map();
let libraryPage = 0, librarySequence = 0, navigationSequence = 0;
const library = document.createElement('section');
library.id = 'library';
library.innerHTML = `<div class="heading"><h2>Sessions</h2><button id="new-session" type="button">New session</button></div>
  <p>Runs are saved automatically on this computer. Opening a run does not resume it.</p>
  <p>Leaving a running session does not pause it. Pause it before leaving if you want it to stop scheduling turns.</p>
  <div id="session-list" aria-live="polite"></div>
  <div class="library-actions"><button id="sessions-previous" type="button">Previous</button><span id="sessions-page"></span><button id="sessions-next" type="button">Next</button><button id="sessions-refresh" type="button">Refresh</button></div>`;
$('setup').before(library);
$('setup').hidden = true;
const nav = document.createElement('button');
nav.type = 'button'; nav.textContent = 'Sessions'; nav.onclick = () => showLibrary().catch(report);
document.querySelector('header').append(nav);
const nameLabel = document.createElement('label');
nameLabel.textContent = 'Session name (optional)';
const nameInput = document.createElement('input');
nameInput.name = 'name'; nameInput.maxLength = 160;
nameLabel.append(nameInput); $('create').prepend(nameLabel);
const rename = document.createElement('form');
rename.id = 'rename-session';
rename.innerHTML = '<label>Session name<input id="session-name" name="name" maxlength="160" required></label><button>Rename</button><span id="rename-result" role="status"></span>';
$('sid').after(rename);
rename.onsubmit = async event => {
  event.preventDefault();
  const id = current;
  try {
    const result = await api(`/api/sessions/${encodeURIComponent(id)}/name`, post({name:$('session-name').value}));
    if (current === id) { $('session-name').value = result.name; $('rename-result').textContent = 'Name saved'; }
  } catch (error) { report(error); }
};
function leaveSession() {
  ++navigationSequence;
  if (current) drafts.set(current, {body:$('send').elements.body.value, recipient:$('recipient').value});
  if (stream) stream.close();
  stream = null; current = ''; ++refreshSequence;
  $('session').hidden = true;
  $('error').textContent = '';
}
async function showLibrary() {
  leaveSession(); $('setup').hidden = true; library.hidden = false;
  await refreshLibrary();
}
async function refreshLibrary() {
  const sequence = ++librarySequence;
  const result = await api(`/api/sessions?offset=${libraryPage * 50}`);
  if (sequence !== librarySequence || library.hidden) return;
  const list = $('session-list'); list.replaceChildren();
  if (!result.sessions.length) list.textContent = libraryPage ? 'No sessions on this page. Go back or refresh.' : 'No saved sessions yet. Create your first run.';
  for (const entry of result.sessions) {
    const card = document.createElement('article'); card.className = 'session-card';
    const title = document.createElement('h3'); title.textContent = entry.name || entry.goal.slice(0,80);
    const goal = document.createElement('p'); goal.className = 'session-goal'; goal.textContent = entry.goal;
    const meta = document.createElement('p'); meta.className = 'agent-meta';
    meta.textContent = `${entry.status} · ${entry.agent_count} ${entry.agent_count === 1 ? 'agent' : 'agents'} · Last activity: ${entry.last_activity ? new Date(entry.last_activity).toLocaleString() : 'unknown'}`;
    const open = document.createElement('button'); open.type = 'button'; open.textContent = 'Open';
    open.onclick = async () => { open.disabled = true; try { await openSession(entry.id); } catch(error) { report(error); } finally { open.disabled = false; } };
    card.append(title,goal,meta,open); list.append(card);
  }
  $('sessions-page').textContent = `Page ${libraryPage + 1}`;
  $('sessions-previous').disabled = libraryPage === 0;
  $('sessions-next').disabled = !result.more;
}
$('sessions-previous').onclick = () => { libraryPage = Math.max(0,libraryPage-1); refreshLibrary().catch(report); };
$('sessions-next').onclick = () => { ++libraryPage; refreshLibrary().catch(report); };
$('sessions-refresh').onclick = () => { libraryPage = 0; refreshLibrary().catch(report); };
$('new-session').onclick = () => { leaveSession(); library.hidden = true; $('setup').hidden = false; };
async function api(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}
function report(error) { $('error').textContent = error.message; if (current) checkStorage().catch(() => {}); }
async function checkStorage() {
  const id = current;
  const state = await api(`/api/sessions/${encodeURIComponent(id)}/storage`);
  if (id !== current) return;
  $('storage-warning').textContent = state.error;
  $('storage-recover').hidden = !state.error;
}
function post(body) { return {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)}; }

async function refresh() {
  const id = current, sequence = ++refreshSequence;
  const data = await api(`/api/sessions/${encodeURIComponent(id)}`);
  if (id !== current || sequence !== refreshSequence) return;
  await checkStorage();
  if (id !== current || sequence !== refreshSequence) return;
  status = data.session.status;
  currentAgents = data.agents || [];
  $('goal').textContent = data.session.goal;
  if (document.activeElement !== $('session-name') && $('session-name').dataset.session !== current) {
    $('session-name').value = data.session.name || '';
    $('session-name').dataset.session = current;
  }
  $('sid').textContent = `Session ID: ${current}`;
  $('status').textContent = `Session: ${status}`;
  const serviceErrors = Object.entries(data.recoveries || {}).filter(([id, recovery]) => recovery.category === 'inference' && currentAgents.some(a => a.id === id)).length;
  if (serviceErrors >= 2) $('status').textContent += ' · Multiple agents are experiencing inference-service errors';
  $('pause').textContent = status === 'paused' ? 'Resume herd' : 'Pause herd';
  const openDiagnostics = new Set([...$('agents').querySelectorAll('details[open]')].map(node => node.dataset.key));
  $('agents').replaceChildren();
  const recipients = $('recipient');
  const selectedRecipient = recipients.value;
  recipients.replaceChildren(new Option('Everyone', ''));
  const names = {};
  for (const agent of data.agents || []) {
    names[agent.id] = agent.name;
    recipients.add(new Option(`${agent.name} (${agent.role})`, agent.id));
    const li = document.createElement('li');
    const heading = document.createElement('strong');
    heading.textContent = agent.name;
    const description = document.createElement('div');
    description.className = 'agent-meta';
    const activity = (data.activity || {})[agent.id];
    description.textContent = `${agent.role} · ${agent.model} · ${agent.status === 'error' ? 'error' : (activity?.state || 'idle')}${agent.error ? ' — ' + agent.error : ''}`;
    if (activity) description.textContent += ` · queue ${activity.queue_ms} ms · inference ${activity.inference_ms} ms · prompt ${activity.prompt_bytes} bytes`;
    if (activity?.last_outcome) description.textContent += ` · last turn ${activity.last_outcome} at ${activity.completed_at}`;
    const recovery = (data.recoveries || {})[agent.id];
    if (recovery) description.textContent += ` · failed attempts ${recovery.failures} · ${recovery.reason}`;
    const label = document.createElement('label');
    label.className = 'thinking-setting';
    const checkbox = document.createElement('input');
    checkbox.type = 'checkbox';
    checkbox.checked = agent.thinking;
    checkbox.onchange = async () => {
      checkbox.disabled = true;
      try { await api(`/api/sessions/${encodeURIComponent(current)}/agents/${encodeURIComponent(agent.id)}/settings`, post({thinking:checkbox.checked})); }
      catch (error) { checkbox.checked = !checkbox.checked; report(error); }
      finally { checkbox.disabled = false; }
    };
    label.append(checkbox, document.createTextNode(' Request model thinking (if supported)'));
    li.append(heading, description, label);
    if (agent.status === 'error' || activity?.state === 'retrying') {
      const retry = document.createElement('button');
      retry.type = 'button';
      retry.textContent = activity?.state === 'retrying' ? 'Retry now' : 'Retry agent';
      retry.onclick = async () => {
        retry.disabled = true;
        try { await api(`/api/sessions/${encodeURIComponent(current)}/agents/${encodeURIComponent(agent.id)}/retry`, post({})); await refresh(); }
        catch (error) { report(error); retry.disabled = false; }
      };
      li.append(retry);
    }
    const failures = (data.failures || []).filter(f => f.agent_id === agent.id);
    if (failures.length) {
      const details = document.createElement('details');
      details.dataset.key = `failures-${agent.id}`;
      details.open = openDiagnostics.has(details.dataset.key);
      const summary = document.createElement('summary');
      summary.textContent = `Failure history (${failures.length})`;
      details.append(summary);
      for (const [index, failure] of failures.entries()) {
        const line = document.createElement('p');
        line.textContent = `${failure.created_at} · attempt ${failure.attempt} · ${failure.category}: ${failure.reason}`;
        details.append(line);
        if (failure.excerpt) {
          const excerpt = document.createElement('details');
          excerpt.dataset.key = `failure-${agent.id}-${index}`;
          excerpt.open = openDiagnostics.has(excerpt.dataset.key);
          const label = document.createElement('summary'); label.textContent = 'Rejected output (untrusted; may contain private data)';
          const content = document.createElement('pre'); content.textContent = failure.excerpt;
          excerpt.append(label, content); details.append(excerpt);
        }
      }
      li.append(details);
    }
    $('agents').append(li);
  }
  recipients.value = [...recipients.options].some(option => option.value === selectedRecipient) ? selectedRecipient : '';
  updateRecipientWarning();
  const container = $('messages');
  const follow = container.scrollHeight - container.scrollTop - container.clientHeight < 48;
  const previousScroll = container.scrollTop;
  const openThoughts = new Set([...container.querySelectorAll('details[open]')].map(node => node.dataset.id));
  container.replaceChildren();
  for (const message of data.messages || []) {
    const item = document.createElement('article');
    item.className = 'chat-message';
    const header = document.createElement('div');
    header.className = 'message-header';
    header.textContent = names[message.agent_id] || 'You';
    if (message.recipient_id) header.textContent += ` → ${names[message.recipient_id] || message.recipient_id} · private`;
    if (message.kind === 'help') header.textContent += ' · help request';
    item.append(header);
    if (message.thinking) {
      const thought = document.createElement('details');
      thought.className = 'thought';
      thought.dataset.id = message.id;
      thought.open = openThoughts.has(String(message.id));
      const summary = document.createElement('summary');
      summary.textContent = 'Model thinking';
      const content = document.createElement('p');
      content.textContent = message.thinking;
      thought.append(summary, content);
      item.append(thought);
    }
    const body = document.createElement('p');
    body.className = 'message-body';
    body.textContent = message.body;
    item.append(body);
    if (!message.agent_id) {
      const targets = currentAgents.filter(agent => !message.recipient_id || agent.id === message.recipient_id);
      const groups = {processed: [], pending: [], 'needs attention': [], unknown: []};
      for (const agent of targets) {
        const progress = (data.delivery || {})[agent.id];
        let state;
        if (!progress || message.id <= progress.baseline) state = 'unknown';
        else if (progress.checkpoint != null && progress.checkpoint >= message.id) state = 'processed';
        else if (agent.status === 'error') state = 'needs attention';
        else state = 'pending';
        groups[state].push(agent.name);
      }
      const delivery = document.createElement('small');
      delivery.textContent = Object.entries(groups).filter(([, names]) => names.length).map(([state, names]) => `${state}: ${names.join(', ')}`).join(' · ');
      delivery.title = 'Processed means included in a completed turn, not necessarily understood or answered.';
      item.append(delivery);
    }
    container.append(item);
  }
  if (follow) container.scrollTop = container.scrollHeight;
  else container.scrollTop = previousScroll;
}
function updateRecipientWarning() {
  const recipient = $('recipient').value;
  const failed = currentAgents.filter(a => a.status === 'error' && (!recipient || a.id === recipient));
  $('recipient-warning').textContent = failed.length ? `Needs attention: ${failed.map(a => a.name).join(', ')}. Messages will remain pending until retry.` : '';
}
$('recipient').onchange = updateRecipientWarning;
async function openSession(id) {
  // Validate before leaving the current view; simply opening must never start a run.
  const navigation = ++navigationSequence;
  await api(`/api/sessions/${encodeURIComponent(id)}`);
  if (navigation !== navigationSequence) return;
  leaveSession();
  current = id;
  const draft = drafts.get(id);
  $('send').elements.body.value = draft?.body || '';
  $('recipient').replaceChildren(new Option('Everyone',''));
  if (draft?.recipient) { $('recipient').add(new Option(draft.recipient,draft.recipient)); $('recipient').value = draft.recipient; }
  $('rename-result').textContent = '';
  $('session-name').dataset.session = '';
  await refresh();
  if (current !== id) return;
  library.hidden = true;
  $('setup').hidden = true;
  $('session').hidden = false;
  $('messages').scrollTop = $('messages').scrollHeight;
  if (stream) stream.close();
  stream = new EventSource(`/api/sessions/${encodeURIComponent(id)}/events`);
  stream.addEventListener('update', () => { if (current === id) refresh().catch(report); });
  stream.onopen = () => { if (current === id) refresh().catch(report); };
}
$('create').onsubmit = async e => { e.preventDefault(); try { const f = new FormData(e.target); const agents = f.get('agents').split('\n').filter(x=>x.trim()).map(line=>{ const [name,role,model] = line.split(',').map(x=>x.trim()); return {name,role,model}; }); const result = await api('/api/sessions',post({name:f.get('name'),goal:f.get('goal'),concurrency:Number(f.get('concurrency')),thinking:f.has('thinking'),inference_timeout_ms:Number(f.get('inference_timeout_ms')),retry_delays_ms:f.get('retry_delays_ms').trim() ? f.get('retry_delays_ms').split(',').map(x=>Number(x.trim())) : [],agents,target_ip:f.get('target_ip').trim(),rate_interval_ms:Number(f.get('rate_interval_ms'))})); await openSession(result.session.id); } catch(err) {report(err);} };
$('open').onsubmit = async e => {e.preventDefault(); try {await openSession(new FormData(e.target).get('id'));} catch(err){report(err);} };
$('send').onsubmit = async e => {
  e.preventDefault();
  const id = current, form = e.target, f = new FormData(form), body = f.get('body');
  const button = form.querySelector('button'); button.disabled = true;
  try {
    await api(`/api/sessions/${encodeURIComponent(id)}/messages`,post({body,recipient_id:f.get('recipient_id')}));
    if (drafts.get(id)?.body === body) drafts.delete(id);
    if (current === id) {
      if (form.elements.body.value === body) form.elements.body.value = '';
      await refresh();
    }
  } catch(err) { report(err); }
  finally { button.disabled = false; }
};
$('pause').onclick = async () => {try {await api(`/api/sessions/${encodeURIComponent(current)}/${status==='paused'?'resume':'pause'}`,post({})); await refresh();} catch(err){report(err);} };
$('storage-recover').onclick = async () => { try { await api(`/api/sessions/${encodeURIComponent(current)}/storage/recover`, post({})); $('error').textContent = ''; await refresh(); } catch (err) { report(err); } };
refreshLibrary().catch(report);
