let current = '', stream, status = '', snapshot;
let refreshSequence = 0, navigationSequence = 0, librarySequence = 0, libraryPage = 0;
let currentAgents = [], selectedAgent = '', messageFilter = 'all', messageSignature = '';
let seenLastMessage = 0;
let librarySearchTimer;
let armedDeletion = '';
const $ = id => document.getElementById(id);
const drafts = new Map(), pendingControls = new Set();
const library = $('library');

function element(tag, className, text) {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text != null) node.textContent = text;
  return node;
}
function avatar(name, index = 0, human = false) {
  const icon = element('div', `avatar ${human ? 'human' : `tone-${index % 5}`}`, Array.from(name || '?').slice(0,2).join('').toUpperCase());
  icon.setAttribute('aria-hidden','true');
  return icon;
}
function timeLabel(value, full = false) {
  const date = new Date(value);
  if (!value || Number.isNaN(date.getTime())) return 'Unknown';
  return full ? date.toLocaleString() : date.toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
}
function badge(value) { return element('span', `status-badge ${value === 'running' ? 'running' : ''}`, value); }
function post(body) { return {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)}; }
async function api(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}
function clearError() { $('error').hidden = true; $('error').textContent = ''; }
function report(error) {
  $('error').textContent = error.message; $('error').hidden = false;
  if (current) checkStorage().catch(() => {});
}
async function checkStorage() {
  const id = current;
  const state = await api(`/api/sessions/${encodeURIComponent(id)}/storage`);
  if (id !== current) return;
  $('storage-warning').textContent = state.error;
  $('storage-banner').hidden = !state.error;
  $('storage-recover').hidden = !state.error;
}
function leaveSession() {
  disarmDeletion();
  ++navigationSequence;
  if (current) drafts.set(current, {body:$('send').elements.body.value, recipient:$('recipient').value});
  if (stream) stream.close();
  stream = null; current = ''; snapshot = null; ++refreshSequence;
  $('session').hidden = true;
  $('session').classList.remove('inspector-open');
  clearError();
}
async function showLibrary() {
  clearTimeout(librarySearchTimer);
  leaveSession(); $('setup').hidden = true; library.hidden = false;
  document.title = 'Garth · Sessions';
  await refreshLibrary();
}
async function refreshLibrary() {
  disarmDeletion();
  clearTimeout(librarySearchTimer);
  const sequence = ++librarySequence;
  const search = $('session-query').value.trim(), filterStatus = $('session-status').value;
  const params = new URLSearchParams({offset:libraryPage * 50, q:search, status:filterStatus});
  setLibraryBusy();
  let result;
  try {
    result = await api(`/api/sessions?${params}`);
  } catch(error) {
    if (sequence === librarySequence && !library.hidden) {
      $('session-list').replaceChildren();
      $('session-results').textContent = 'Could not load sessions. Use Refresh list to retry.';
      report(error);
    }
    return;
  } finally {
    if (sequence === librarySequence) {
      $('session-list').setAttribute('aria-busy','false');
      $('sessions-previous').disabled = libraryPage === 0;
    }
  }
  if (sequence !== librarySequence || library.hidden) return;
  const list = $('session-list'); list.replaceChildren();
  const filtered = !!(search || filterStatus);
  if (!result.sessions.length) list.append(element('p','empty-state',libraryPage ? 'No sessions on this page. Go back or refresh.' : filtered ? 'No sessions match these filters. Try another search or clear the filters.' : 'No experiments yet. Start a session to bring your group together.'));
  const count = result.sessions.length;
  $('session-results').textContent = `${count} ${filtered ? 'matching ' : ''}${count === 1 ? 'session' : 'sessions'} on this page${result.more ? ' · more available' : ''}`;
  for (const entry of result.sessions) {
    const card = element('article','session-card');
    const top = element('div','card-top');
    top.append(badge(entry.status),element('span','',`${entry.agent_count} ${entry.agent_count === 1 ? 'agent' : 'agents'}`));
    const title = element('h3','',entry.name || entry.goal.slice(0,80));
    const goal = element('p','session-goal',entry.goal);
    const meta = element('p','card-meta',`Last activity · ${timeLabel(entry.last_activity,true)}`);
    const open = element('button','', 'Open session →'); open.type = 'button';
    open.onclick = async () => {
      open.disabled = true;
      try { await openSession(entry.id); } catch(error) { report(error); } finally { open.disabled = false; }
    };
    const actions = element('div','card-actions'), remove = element('button','delete-session');
    const question = element('span','delete-question','?'); question.setAttribute('aria-hidden','true');
    const icon = document.createElementNS('http://www.w3.org/2000/svg','svg');
    for (const [name,value] of Object.entries({viewBox:'0 0 20 20',width:'18',height:'18',fill:'none',stroke:'currentColor','stroke-width':'1.7','stroke-linecap':'round','stroke-linejoin':'round','aria-hidden':'true'})) icon.setAttribute(name,value);
    const path = document.createElementNS('http://www.w3.org/2000/svg','path');
    path.setAttribute('d','M4 6 H16 M8 3.5 H12 L13 6 M6 6 L7 16.5 H13 L14 6 M9 9 V14 M11 9 V14');
    icon.append(path); remove.append(question,icon);
    remove.dataset.session = entry.id;
    remove.type = 'button'; remove.disabled = entry.status !== 'paused';
    remove.title = remove.disabled ? 'Pause this session before deleting it' : 'Permanently delete this session and its history';
    remove.setAttribute('aria-label',`Delete session ${entry.name || entry.goal.slice(0,80)}${remove.disabled ? ' (pause first)' : ''}`);
    const warning = element('p','delete-warning','Click the trash can again to permanently delete this run, including private messages, memory, and all history. No undo.');
    warning.hidden = true; warning.setAttribute('role','status');
    remove.onclick = async event => {
      // A double-click must not turn the initial arm action into deletion.
      if (event?.detail > 1) return;
      if (armedDeletion !== entry.id) {
        disarmDeletion(); armedDeletion = entry.id;
        remove.classList.add('armed'); warning.hidden = false;
        remove.setAttribute('aria-label',`Confirm permanent deletion of session ${entry.name || entry.goal.slice(0,80)}`);
        remove.title = 'Click again to permanently delete. Click elsewhere or press Escape to cancel.';
        return;
      }
      remove.disabled = true; open.disabled = true;
      try {
        await api(`/api/sessions/${encodeURIComponent(entry.id)}`,{...post({confirm_id:entry.id}),method:'DELETE'});
        drafts.delete(entry.id); libraryPage = 0; await refreshLibrary();
      } catch(error) { report(error); disarmDeletion(); remove.disabled = false; open.disabled = false; }
    };
    actions.append(open,remove); card.append(top,title,goal,meta,actions,warning); list.append(card);
  }
  $('sessions-page').textContent = `Page ${libraryPage + 1}`;
  $('sessions-previous').disabled = libraryPage === 0;
  $('sessions-next').disabled = !result.more;
}
function disarmDeletion() {
  armedDeletion = '';
  document.querySelectorAll('.delete-session.armed').forEach(button=>{
    button.classList.remove('armed');
    const name = button.closest('.session-card').querySelector('h3').textContent;
    button.setAttribute('aria-label',`Delete session ${name}`);
    button.title = 'Permanently delete this session and its history';
  });
  document.querySelectorAll('.delete-warning').forEach(warning=>{ warning.hidden = true; });
}
function setLibraryBusy() {
  $('session-list').setAttribute('aria-busy','true');
  $('session-results').textContent = 'Searching sessions…';
  $('sessions-previous').disabled = true;
  $('sessions-next').disabled = true;
  $('clear-session-filters').disabled = !$('session-query').value && !$('session-status').value;
}
function filterLibrary(debounce = false) {
  clearTimeout(librarySearchTimer); clearError();
  libraryPage = 0;
  // Invalidate an older response immediately, not after the debounce delay.
  ++librarySequence; setLibraryBusy();
  if (debounce) librarySearchTimer = setTimeout(()=>refreshLibrary().catch(report),200);
  else return refreshLibrary().catch(report);
}

async function refresh() {
  if (!current) return;
  const id = current, sequence = ++refreshSequence;
  const data = await api(`/api/sessions/${encodeURIComponent(id)}`);
  if (id !== current || sequence !== refreshSequence) return;
  await checkStorage();
  if (id !== current || sequence !== refreshSequence) return;
  snapshot = data;
  status = data.session.status;
  currentAgents = data.agents || [];
  const title = data.session.name || data.session.goal.slice(0,80);
  $('session-title').textContent = title;
  document.title = `${title} · Garth`;
  $('goal').textContent = data.session.goal;
  if (document.activeElement !== $('session-name') && $('session-name').dataset.session !== current) {
    $('session-name').value = data.session.name || '';
    $('session-name').dataset.session = current;
  }
  $('sid').textContent = `Session ID: ${current}`;
  $('status').textContent = status;
  $('status').className = `status-badge ${status === 'running' ? 'running' : ''}`;
  $('pause').textContent = status === 'paused' ? 'Resume session' : 'Pause session';
  const serviceErrors = Object.values(data.recoveries || {}).filter(r => r.category === 'inference').length;
  $('service-warning').hidden = serviceErrors < 2;
  $('service-warning').textContent = `${serviceErrors} agents are experiencing inference-service errors. Select an agent to inspect recovery details.`;
  if (!currentAgents.some(a => a.id === selectedAgent)) selectedAgent = (currentAgents.find(a => a.role === 'leader') || currentAgents[0])?.id || '';
  renderRoster(); renderAgentDetail();
  const recipients = $('recipient'), selectedRecipient = recipients.value;
  recipients.replaceChildren(new Option('Everyone',''));
  for (const agent of currentAgents) recipients.add(new Option(`${agent.name} (${agent.role})`,agent.id));
  recipients.value = [...recipients.options].some(option => option.value === selectedRecipient) ? selectedRecipient : '';
  updateRecipientWarning();
  const activity = data.activity || {};
  const active = currentAgents.filter(a => activity[a.id]?.state === 'inference');
  const queued = currentAgents.filter(a => activity[a.id]?.state === 'queued').length;
  $('slot-count').textContent = `${active.length} of ${data.session.concurrency} inference slots active`;
  $('slots').replaceChildren();
  for (let i = 0; i < data.session.concurrency; ++i) $('slots').append(element('i',i < active.length ? 'on' : ''));
  const resting = currentAgents.filter(a => GarthUI.agentState(a,activity[a.id]).label === 'Resting').length;
  $('queue-count').textContent = `${queued} queued · ${resting} resting`;
  $('inference-note').textContent = active.length ? `${active.slice(0,3).map(a => a.name).join(', ')}${active.length > 3 ? ` + ${active.length-3} more` : ''} ${active.length === 1 ? 'is' : 'are'} in inference${status === 'paused' ? ' · session paused' : ''}` : status === 'paused' ? 'Session paused · pending messages are preserved' : 'No requests currently in inference';
  renderMessages();
}

function renderRoster() {
  const focused = document.activeElement?.dataset.agent;
  const scroll = $('agents').scrollTop;
  $('agent-count').textContent = currentAgents.length;
  $('agents').replaceChildren();
  currentAgents.forEach((agent,index) => {
    const li = element('li'), button = element('button','agent-button');
    const state = GarthUI.agentState(agent,snapshot.activity?.[agent.id],status === 'paused');
    button.type = 'button'; button.dataset.agent = agent.id;
    button.setAttribute('aria-pressed',String(agent.id === selectedAgent));
    button.setAttribute('aria-controls','agent-detail');
    const copy = element('span','agent-copy');
    copy.append(element('strong','',agent.name),element('span','agent-role',agent.role),element('div',`agent-state ${state.tone}`,state.label));
    button.append(avatar(agent.name,index),copy);
    button.onclick = () => selectAgent(agent.id);
    li.append(button); $('agents').append(li);
    if (focused === agent.id) button.focus({preventScroll:true});
  });
  $('agents').scrollTop = scroll;
}
function selectAgent(id) {
  selectedAgent = id;
  renderRoster(); renderAgentDetail();
  $('agent-detail').parentElement.scrollTop = 0;
  $('session').classList.add('inspector-open');
  if (matchMedia('(max-width: 1150px)').matches) $('close-inspector').focus();
}
function definition(list,term,value) { list.append(element('dt','',term),element('dd','',value)); }
function details(label,key,openKeys) {
  const node = element('details'); node.dataset.key = key; node.open = openKeys.has(key);
  node.append(element('summary','',label)); return node;
}
function renderAgentDetail() {
  const panel = $('agent-detail');
  const openKeys = new Set([...panel.querySelectorAll('details[open]')].map(n=>n.dataset.key));
  const scroll = panel.parentElement.scrollTop;
  // Preserve keyboard focus across activity updates, along with expanded diagnostics.
  const focusKey = panel.contains(document.activeElement) ? document.activeElement.dataset.control : null;
  panel.replaceChildren();
  const agent = currentAgents.find(a => a.id === selectedAgent);
  if (!agent) { panel.append(element('p','muted','Select an agent to inspect its activity and settings.')); return; }
  const sid = current, aid = agent.id, index = currentAgents.indexOf(agent);
  const activity = snapshot.activity?.[aid], recovery = snapshot.recoveries?.[aid];
  const state = GarthUI.agentState(agent,activity,status === 'paused');
  const icon = avatar(agent.name,index); icon.classList.add('detail-avatar');
  panel.append(icon,element('h2','',agent.name),element('p','detail-role',`${agent.role} · ${state.label}`));
  const overview = element('section','detail-section'), info = element('dl');
  definition(info,'Model',agent.model);
  definition(info,'Agent ID',aid);
  definition(info,'Last completed turn',activity?.last_outcome ? `${activity.last_outcome === 'rested' ? 'Rested' : 'Responded'} · ${timeLabel(activity.completed_at,true)}` : 'No completed turn recorded since server start');
  if (agent.error) definition(info,'Error',agent.error);
  overview.append(info); panel.append(overview);
  const controls = element('section','detail-section');
  controls.append(element('h3','','Agent controls'));
  const label = element('label','thinking-setting'), checkbox = element('input');
  const thinkingKey = `${sid}:${aid}:thinking`;
  checkbox.type = 'checkbox'; checkbox.checked = agent.thinking;
  checkbox.dataset.control = 'thinking'; checkbox.disabled = pendingControls.has(thinkingKey);
  checkbox.onchange = async () => {
    pendingControls.add(thinkingKey); checkbox.disabled = true;
    try {
      await api(`/api/sessions/${encodeURIComponent(sid)}/agents/${encodeURIComponent(aid)}/settings`,post({thinking:checkbox.checked}));
      if (current === sid) await refresh();
    } catch(error) { checkbox.checked = !checkbox.checked; report(error); }
    finally { pendingControls.delete(thinkingKey); if (current === sid) renderAgentDetail(); }
  };
  label.append(checkbox,document.createTextNode('Request model thinking (if supported)')); controls.append(label);
  const actions = element('div','detail-actions'), message = element('button','','Message privately');
  message.type = 'button'; message.dataset.control = 'message';
  message.onclick = () => {
    $('recipient').value = aid; updateRecipientWarning();
    $('session').classList.remove('inspector-open'); $('message-body').focus();
  };
  actions.append(message);
  if (agent.status === 'error' || activity?.state === 'retrying') {
    const retry = element('button','',activity?.state === 'retrying' ? 'Retry now' : 'Retry agent');
    const retryKey = `${sid}:${aid}:retry`;
    retry.type = 'button'; retry.dataset.control = 'retry'; retry.disabled = pendingControls.has(retryKey);
    retry.onclick = async () => {
      pendingControls.add(retryKey); retry.disabled = true;
      try { await api(`/api/sessions/${encodeURIComponent(sid)}/agents/${encodeURIComponent(aid)}/retry`,post({})); if (current === sid) await refresh(); }
      catch(error) { report(error); }
      finally { pendingControls.delete(retryKey); if (current === sid) renderAgentDetail(); }
    };
    actions.append(retry);
  }
  controls.append(actions); panel.append(controls);
  const diagnosticSection = element('section','detail-section');
  const diagnostic = details('Timing & prompt diagnostics',`diagnostics-${aid}`,openKeys), timings = element('dl');
  if (activity) {
    definition(timings,'Harness queue',activity.state === 'queued' ? 'Waiting for a slot' : GarthUI.duration(activity.queue_ms));
    definition(timings,'Inference request',activity.state === 'inference' ? 'In progress · duration available after completion' : GarthUI.duration(activity.inference_ms));
    definition(timings,'Prompt size',`${activity.prompt_bytes.toLocaleString()} bytes`);
  } else definition(timings,'Activity','No timing recorded since server start');
  diagnostic.append(timings,element('p','','Request time includes any Ollama queueing. Prompt size is UTF-8 bytes, not tokens. Timings reset on server restart.'));
  diagnosticSection.append(diagnostic);
  if (recovery) {
    const info = element('dl');
    definition(info,'Failed attempts',recovery.failures);
    definition(info,'Category',recovery.category);
    definition(info,'Reason',recovery.reason);
    if (recovery.next_at && !recovery.next_at.startsWith('0001-')) definition(info,'Next automatic retry',timeLabel(recovery.next_at,true));
    diagnosticSection.append(info);
  }
  const failures = (snapshot.failures || []).filter(f=>f.agent_id === aid);
  const history = details(`Failure history (${failures.length})`,`failures-${aid}`,openKeys);
  if (!failures.length) history.append(element('p','','No recorded failures.'));
  failures.forEach((failure,index) => {
    const record = element('div','failure-record');
    record.append(element('p','',`${timeLabel(failure.created_at,true)} · attempt ${failure.attempt} · ${failure.category}`),element('p','',failure.reason));
    if (failure.excerpt) {
      const excerpt = details('Rejected output · untrusted; may contain private data',`excerpt-${aid}-${index}`,openKeys);
      excerpt.append(element('pre','',failure.excerpt)); record.append(excerpt);
    }
    history.append(record);
  });
  diagnosticSection.append(history); panel.append(diagnosticSection);
  const memory = element('div','memory-note','Private working memory');
  memory.append(element('p','','Agent-generated summaries can lose or distort evidence. Their contents are not shared with other agents automatically.'));
  panel.append(memory);
  if (focusKey) panel.querySelector(`[data-control="${focusKey}"]`)?.focus({preventScroll:true});
  panel.parentElement.scrollTop = scroll;
}

function renderMessages(force = false) {
  if (!snapshot) return;
  const messages = snapshot.messages || [];
  const visible = messages.filter(m=>GarthUI.visibleMessage(m,messageFilter));
  // Messages are immutable: activity-only updates need not rebuild the whole thread.
  const signature = `${messageFilter}:${visible.length}:${visible.at(-1)?.id}:${JSON.stringify(snapshot.delivery)}:${currentAgents.map(a=>a.status).join(',')}`;
  if (!force && signature === messageSignature) return;
  messageSignature = signature;
  const container = $('messages'), follow = container.scrollHeight - container.scrollTop - container.clientHeight < 48;
  const previousScroll = container.scrollTop;
  const openThoughts = new Set([...container.querySelectorAll('details[open]')].map(n=>n.dataset.key));
  const names = new Map(currentAgents.map(a=>[a.id,a.name]));
  const indices = new Map(currentAgents.map((a,i)=>[a.id,i]));
  container.replaceChildren();
  $('message-count').textContent = `${visible.length} ${visible.length === 1 ? 'message' : 'messages'}`;
  if (!visible.length) container.append(element('p','empty-state',messageFilter === 'direct' ? 'No direct messages yet. Choose an agent in the composer to write privately.' : 'No messages here yet. Start the conversation below.'));
  let previousDay = '';
  for (const message of visible) {
    const date = new Date(message.created_at), day = date.toLocaleDateString();
    if (day !== previousDay) { container.append(element('p','day-divider',day)); previousDay = day; }
    const human = !message.agent_id, sender = human ? 'You' : names.get(message.agent_id) || 'Unknown agent';
    const item = element('article',`chat-message${message.recipient_id ? ' private' : ''}`);
    const content = element('div','message-content'), header = element('div','message-header');
    header.append(element('strong','',message.recipient_id ? `${sender} → ${names.get(message.recipient_id) || message.recipient_id}` : sender));
    if (message.recipient_id) header.append(element('span','message-label','Private · operator visible'));
    else if (human) header.append(element('span','message-label','Everyone'));
    if (message.kind === 'help') header.append(element('span','message-label','Help request'));
    const timestamp = element('time','',timeLabel(message.created_at)); timestamp.dateTime = message.created_at; timestamp.title = timeLabel(message.created_at,true); header.append(timestamp);
    content.append(header,element('p','message-body',message.body));
    if (message.thinking) {
      const thought = details('Model thinking',`thought-${message.id}`,openThoughts); thought.className = 'thought';
      thought.append(element('p','',message.thinking)); content.append(thought);
    }
    if (human) {
      const groups = {processed:[],pending:[],'needs attention':[],unknown:[]};
      for (const agent of currentAgents.filter(a=>!message.recipient_id || a.id === message.recipient_id)) groups[GarthUI.deliveryState(message,agent,snapshot.delivery?.[agent.id])].push(agent.name);
      const progress = details(Object.entries(groups).filter(([,names])=>names.length).map(([state,names])=>`${names.length} ${state}`).join(' · ') || 'No recipients',`delivery-${message.id}`,openThoughts);
      progress.className = 'delivery'; progress.title = 'Processed means included in a completed turn, not necessarily understood or answered.';
      for (const [state,names] of Object.entries(groups)) if (names.length) progress.append(element('p','',`${state}: ${names.join(', ')}`));
      content.append(progress);
    }
    item.append(avatar(sender,indices.get(message.agent_id) || 0,human),content); container.append(item);
  }
  const last = visible.at(-1)?.id || 0;
  if (follow || force) { container.scrollTop = container.scrollHeight; $('jump-latest').hidden = true; }
  else { container.scrollTop = previousScroll; if (last > seenLastMessage) $('jump-latest').hidden = false; }
  seenLastMessage = last;
}
function updateRecipientWarning() {
  const recipient = $('recipient').value;
  const failed = currentAgents.filter(a=>a.status === 'error' && (!recipient || a.id === recipient));
  $('recipient-warning').textContent = failed.length ? `Needs attention: ${failed.map(a=>a.name).join(', ')}. Messages remain pending until recovery.` : '';
  $('privacy-note').textContent = recipient ? 'Private · visible to the operator' : 'Public to the group';
}
async function openSession(id) {
  const navigation = ++navigationSequence;
  await api(`/api/sessions/${encodeURIComponent(id)}`);
  if (navigation !== navigationSequence) return;
  leaveSession(); current = id; selectedAgent = ''; messageSignature = ''; seenLastMessage = 0;
  const draft = drafts.get(id);
  $('send').elements.body.value = draft?.body || '';
  $('recipient').replaceChildren(new Option('Everyone',''));
  if (draft?.recipient) { $('recipient').add(new Option(draft.recipient,draft.recipient)); $('recipient').value = draft.recipient; }
  $('rename-result').textContent = ''; $('session-name').dataset.session = '';
  $('session').querySelector('.session-settings').open = false;
  // Clear details from the last session so expansion state cannot cross sessions.
  $('agent-detail').replaceChildren(); $('messages').replaceChildren();
  await refresh();
  if (current !== id) return;
  library.hidden = true; $('setup').hidden = true; $('session').hidden = false;
  $('messages').scrollTop = $('messages').scrollHeight;
  if (stream) stream.close();
  stream = new EventSource(`/api/sessions/${encodeURIComponent(id)}/events`);
  stream.addEventListener('update',()=>{ if (current === id) refresh().catch(report); });
  stream.addEventListener('deleted',()=>{
    if (current === id) { const pending = showLibrary(); drafts.delete(id); pending.catch(report); }
  });
  stream.onopen = ()=>{ if (current === id) refresh().catch(report); };
}

$('nav-sessions').onclick = () => showLibrary().catch(report);
$('brand').onclick = event => { event.preventDefault(); showLibrary().catch(report); };
$('cancel-setup').onclick = () => showLibrary().catch(report);
$('sessions-previous').onclick = () => { libraryPage = Math.max(0,libraryPage-1); refreshLibrary().catch(report); };
$('sessions-next').onclick = () => { ++libraryPage; refreshLibrary().catch(report); };
$('sessions-refresh').onclick = () => { libraryPage = 0; refreshLibrary().catch(report); };
$('session-search').onsubmit = event => { event.preventDefault(); filterLibrary(); };
$('session-query').oninput = () => filterLibrary(true);
$('session-status').onchange = () => filterLibrary();
$('clear-session-filters').onclick = () => {
  $('session-query').value = ''; $('session-status').value = ''; filterLibrary();
  $('session-query').focus();
};
$('new-session').onclick = () => { clearTimeout(librarySearchTimer); leaveSession(); library.hidden = true; $('setup').hidden = false; document.title = 'New session · Garth'; };
$('recipient').onchange = updateRecipientWarning;
$('close-inspector').onclick = () => {
  $('session').classList.remove('inspector-open');
  [...$('agents').querySelectorAll('button')].find(b=>b.dataset.agent === selectedAgent)?.focus();
};
document.addEventListener('keydown',event=>{
  if (event.key === 'Escape') {
    disarmDeletion();
    if ($('session').classList.contains('inspector-open')) $('close-inspector').click();
  }
});
document.addEventListener('click',event=>{ if (!event.target.closest?.('.delete-session')) disarmDeletion(); });
document.addEventListener('focusin',event=>{ if (!event.target.closest?.('.delete-session')) disarmDeletion(); });
document.querySelectorAll('[data-filter]').forEach(button=>{
  button.onclick = () => {
    messageFilter = button.dataset.filter;
    document.querySelectorAll('[data-filter]').forEach(b=>b.setAttribute('aria-pressed',String(b === button)));
    renderMessages(true);
  };
});
$('jump-latest').onclick = () => { $('messages').scrollTop = $('messages').scrollHeight; $('jump-latest').hidden = true; };
$('messages').onscroll = () => { if ($('messages').scrollHeight - $('messages').scrollTop - $('messages').clientHeight < 48) $('jump-latest').hidden = true; };
$('rename-session').onsubmit = async event => {
  event.preventDefault(); const id = current;
  try {
    const result = await api(`/api/sessions/${encodeURIComponent(id)}/name`,post({name:$('session-name').value}));
    if (current === id) { $('session-name').value = result.name; $('rename-result').textContent = 'Name saved'; await refresh(); }
  } catch(error) { report(error); }
};
$('create').onsubmit = async event => {
  event.preventDefault(); const form = event.target, button = form.querySelector('button'); button.disabled = true; clearError();
  try {
    const f = new FormData(form);
    const agents = f.get('agents').split('\n').filter(x=>x.trim()).map(line=>{ const [name,role,model] = line.split(',').map(x=>x.trim()); return {name,role,model}; });
    const result = await api('/api/sessions',post({name:f.get('name'),goal:f.get('goal'),concurrency:Number(f.get('concurrency')),thinking:f.has('thinking'),inference_timeout_ms:Number(f.get('inference_timeout_ms')),retry_delays_ms:f.get('retry_delays_ms').trim() ? f.get('retry_delays_ms').split(',').map(x=>Number(x.trim())) : [],agents,target_ip:f.get('target_ip').trim(),rate_interval_ms:Number(f.get('rate_interval_ms'))}));
    await openSession(result.session.id);
  } catch(error) { report(error); }
  finally { button.disabled = false; }
};
$('open').onsubmit = async event => { event.preventDefault(); try { await openSession(new FormData(event.target).get('id').trim()); } catch(error) { report(error); } };
$('send').onsubmit = async event => {
  event.preventDefault(); clearError();
  const id = current, form = event.target, f = new FormData(form), body = f.get('body');
  const button = form.querySelector('button'); button.disabled = true;
  try {
    await api(`/api/sessions/${encodeURIComponent(id)}/messages`,post({body,recipient_id:f.get('recipient_id')}));
    if (drafts.get(id)?.body === body) drafts.delete(id);
    if (current === id) { if (form.elements.body.value === body) form.elements.body.value = ''; await refresh(); }
  } catch(error) { report(error); }
  finally { button.disabled = false; }
};
$('pause').onclick = async () => {
  const id = current; $('pause').disabled = true;
  try { await api(`/api/sessions/${encodeURIComponent(id)}/${status === 'paused' ? 'resume' : 'pause'}`,post({})); if (current === id) await refresh(); }
  catch(error) { report(error); }
  finally { $('pause').disabled = false; }
};
$('storage-recover').onclick = async () => {
  const id = current; $('storage-recover').disabled = true;
  try { await api(`/api/sessions/${encodeURIComponent(id)}/storage/recover`,post({})); if (current === id) { clearError(); await refresh(); } }
  catch(error) { report(error); }
  finally { $('storage-recover').disabled = false; }
};
refreshLibrary().catch(report);
