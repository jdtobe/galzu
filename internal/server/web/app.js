let current = '', stream, status = '';
let refreshSequence = 0;
const $ = id => document.getElementById(id);
async function api(path, options) {
  const response = await fetch(path, options);
  if (!response.ok) throw new Error(await response.text());
  return response.json();
}
function report(error) { $('error').textContent = error.message; }
function post(body) { return {method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify(body)}; }

async function refresh() {
  const id = current, sequence = ++refreshSequence;
  const data = await api(`/api/sessions/${encodeURIComponent(id)}`);
  if (id !== current || sequence !== refreshSequence) return;
  status = data.session.status;
  $('goal').textContent = data.session.goal;
  $('sid').textContent = `Session ID: ${current}`;
  $('status').textContent = `Session: ${status}`;
  $('pause').textContent = status === 'paused' ? 'Resume herd' : 'Pause herd';
  $('agents').replaceChildren();
  const names = {};
  for (const agent of data.agents || []) {
    names[agent.id] = agent.name;
    const li = document.createElement('li');
    const heading = document.createElement('strong');
    heading.textContent = agent.name;
    const description = document.createElement('div');
    description.className = 'agent-meta';
    const activity = (data.activity || {})[agent.id];
    description.textContent = `${agent.role} · ${agent.status === 'error' ? 'error' : (activity?.state || 'idle')}${agent.error ? ' — ' + agent.error : ''}`;
    if (activity) description.textContent += ` · queue ${activity.queue_ms} ms · inference ${activity.inference_ms} ms · prompt ${activity.prompt_bytes} bytes`;
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
    $('agents').append(li);
  }
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
    container.append(item);
  }
  if (follow) container.scrollTop = container.scrollHeight;
  else container.scrollTop = previousScroll;
}
async function openSession(id) {
  current = id;
  await refresh();
  $('setup').hidden = true;
  $('session').hidden = false;
  $('messages').scrollTop = $('messages').scrollHeight;
  if (stream) stream.close();
  stream = new EventSource(`/api/sessions/${encodeURIComponent(id)}/events`);
  stream.addEventListener('update', () => refresh().catch(report));
  stream.onopen = () => refresh().catch(report);
}
$('create').onsubmit = async e => { e.preventDefault(); try { const f = new FormData(e.target); const agents = f.get('agents').split('\n').filter(x=>x.trim()).map(line=>{ const [name,role,model] = line.split(',').map(x=>x.trim()); return {name,role,model}; }); const result = await api('/api/sessions',post({goal:f.get('goal'),concurrency:Number(f.get('concurrency')),thinking:f.has('thinking'),agents,target_ip:f.get('target_ip').trim(),rate_interval_ms:Number(f.get('rate_interval_ms'))})); await openSession(result.session.id); } catch(err) {report(err);} };
$('open').onsubmit = async e => {e.preventDefault(); try {await openSession(new FormData(e.target).get('id'));} catch(err){report(err);} };
$('send').onsubmit = async e => {e.preventDefault(); try {await api(`/api/sessions/${encodeURIComponent(current)}/messages`,post({body:new FormData(e.target).get('body')})); e.target.reset(); await refresh();} catch(err){report(err);} };
$('pause').onclick = async () => {try {await api(`/api/sessions/${encodeURIComponent(current)}/${status==='paused'?'resume':'pause'}`,post({})); await refresh();} catch(err){report(err);} };
