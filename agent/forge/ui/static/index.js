let isPaused = false;
let selectedTunnel = null;
let selectedRequestID = null;
let requests = [];
let ws = null;

// Connect WebSocket
function connectWS() {
  const protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
  const wsUrl = `${protocol}//${location.host}/ws`;
  ws = new WebSocket(wsUrl);

  ws.onopen = () => {
    const dot = document.getElementById('ws-dot');
    const text = document.getElementById('ws-text');
    if (dot) dot.className = 'status-dot online';
    if (text) text.innerText = 'Live';
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      if (data.type === 'request' && data.entry) {
        handleNewRequest(data.entry);
      }
    } catch (e) {
      console.error('Error parsing WS event:', e);
    }
  };

  ws.onclose = () => {
    const dot = document.getElementById('ws-dot');
    const text = document.getElementById('ws-text');
    if (dot) dot.className = 'status-dot';
    if (text) text.innerText = 'Disconnected';
    setTimeout(connectWS, 2000);
  };
}

// Load active tunnels
async function loadTunnels() {
  try {
    const res = await fetch('/api/tunnels');
    const data = await res.json();
    const listEl = document.getElementById('tunnel-list');
    if (!listEl) return;
    listEl.innerHTML = '';

    // All Tunnels option
    const allLi = document.createElement('li');
    allLi.className = `tunnel-item ${selectedTunnel === null ? 'active' : ''}`;
    allLi.innerHTML = `<div class="name">All Tunnels</div>`;
    allLi.onclick = () => selectTunnel(null);
    listEl.appendChild(allLi);

    for (const [subdomain, entry] of Object.entries(data)) {
      const local = entry.local || entry.Local || '';
      const capture = entry.capture ?? entry.Capture ?? false;
      const li = document.createElement('li');
      li.className = `tunnel-item ${selectedTunnel === subdomain ? 'active' : ''}`;
      li.innerHTML = `
        <div class="name">
          <span>${subdomain}</span>
          ${capture ? '<span class="badge-capture">Capture</span>' : ''}
        </div>
        <div class="local">${local}</div>
      `;
      li.onclick = () => selectTunnel(subdomain);
      listEl.appendChild(li);
    }
  } catch (err) {
    console.error('Error loading tunnels:', err);
  }
}

// Load request history
async function loadRequests() {
  try {
    let url = '/api/requests?limit=100';
    if (selectedTunnel) {
      url += `&subdomain=${encodeURIComponent(selectedTunnel)}`;
    }
    const res = await fetch(url);
    const data = await res.json();
    requests = data || [];
    renderRequests();
  } catch (err) {
    console.error('Error loading requests:', err);
  }
}

function selectTunnel(subdomain) {
  selectedTunnel = subdomain;
  loadTunnels();
  loadRequests();
}

function handleNewRequest(entry) {
  if (isPaused) return;
  if (selectedTunnel && entry.subdomain !== selectedTunnel) return;

  requests.unshift(entry);
  if (requests.length > 200) requests.pop();
  renderRequests();
}

function renderRequests() {
  const searchInput = document.getElementById('search-input');
  const search = searchInput ? searchInput.value.toLowerCase() : '';
  const listEl = document.getElementById('request-list');
  if (!listEl) return;
  listEl.innerHTML = '';

  const filtered = requests.filter(r => !search || r.url.toLowerCase().includes(search) || r.method.toLowerCase().includes(search));

  filtered.forEach(req => {
    const row = document.createElement('li');
    row.className = `request-row ${req.id === selectedRequestID ? 'selected' : ''}`;
    
    let statusClass = 'status-2xx';
    if (req.response_status >= 500) statusClass = 'status-5xx';
    else if (req.response_status >= 400) statusClass = 'status-4xx';
    else if (req.response_status >= 300) statusClass = 'status-3xx';

    const timeStr = new Date(req.timestamp).toLocaleTimeString();
    const isReplayed = !!(req.replayed || req.is_replayed || req.isreplied);
    const replayedBadge = isReplayed ? '<span class="badge-replayed">Replayed</span>' : '';

    row.innerHTML = `
      <span class="status-badge ${statusClass}">${req.response_status || '---'}</span>
      <span class="method-badge method-${req.method}">${req.method}</span>
      <span class="path-col">${req.url}</span>
      ${replayedBadge}
      <div class="meta-col">
        <span>${req.duration_ms}ms</span>
        <span>${timeStr}</span>
      </div>
    `;

    row.onclick = () => selectRequest(req);
    listEl.appendChild(row);
  });
}

function selectRequest(req) {
  selectedRequestID = req.id;
  renderRequests();

  const emptyState = document.getElementById('empty-state');
  const content = document.getElementById('inspector-content');
  if (emptyState) emptyState.style.display = 'none';
  if (content) content.style.display = 'flex';

  const metaEl = document.getElementById('request-meta');
  if (metaEl) {
    const isReplayed = !!(req.replayed || req.is_replayed || req.isreplied);
    const replayedBadge = isReplayed ? '<span class="badge-replayed">Replayed</span>' : '';
    metaEl.innerHTML = `<span>${req.method} ${req.url} (${req.duration_ms}ms)</span>${replayedBadge}`;
  }

  // Request tab
  renderHeaders('req-headers-table', req.request_headers);
  renderBody('req-body-view', req.request_body);

  // Response tab
  renderHeaders('resp-headers-table', req.response_headers);
  renderBody('resp-body-view', req.response_body);
}

function renderHeaders(tableId, headers) {
  const table = document.getElementById(tableId);
  if (!table) return;
  table.innerHTML = '';
  if (!headers || Object.keys(headers).length === 0) {
    table.innerHTML = '<tr><td colspan="2" style="color: var(--text-muted);">No headers</td></tr>';
    return;
  }
  for (const [k, v] of Object.entries(headers)) {
    const tr = document.createElement('tr');
    tr.innerHTML = `<th>${k}</th><td>${Array.isArray(v) ? v.join(', ') : v}</td>`;
    table.appendChild(tr);
  }
}

function renderBody(viewId, bodyData) {
  const view = document.getElementById(viewId);
  if (!view) return;
  if (!bodyData) {
    view.innerText = '(empty body)';
    return;
  }

  const text = decodeBase64(bodyData);
  try {
    const parsed = JSON.parse(text);
    view.innerText = JSON.stringify(parsed, null, 2);
  } catch {
    view.innerText = text;
  }
}

function switchTab(tab) {
  document.querySelectorAll('.tab-btn').forEach(btn => btn.classList.remove('active'));
  const activeBtn = document.getElementById(`tab-btn-${tab}`);
  if (activeBtn) activeBtn.classList.add('active');
  const reqTab = document.getElementById('tab-request');
  const respTab = document.getElementById('tab-response');
  if (reqTab) reqTab.style.display = tab === 'request' ? 'block' : 'none';
  if (respTab) respTab.style.display = tab === 'response' ? 'block' : 'none';
}

function togglePause() {
  isPaused = !isPaused;
  const btn = document.getElementById('pause-btn');
  if (btn) {
    btn.classList.toggle('active', isPaused);
    btn.innerText = isPaused ? 'Resume' : 'Pause';
  }
}

function decodeBase64(b64) {
  if (!b64 || typeof b64 !== 'string') return '';
  try {
    const binString = atob(b64);
    const bytes = Uint8Array.from(binString, c => c.codePointAt(0));
    return new TextDecoder().decode(bytes);
  } catch (e) {
    return b64;
  }
}

async function replayCurrentRequest() {
  const req = requests.find(r => r.id === selectedRequestID);
  if (!req) return;
  const btn = document.getElementById('replay-btn');
  if (btn) {
    btn.disabled = true;
    btn.innerText = 'Replaying...';
  }
  try {
    const res = await fetch('/api/requests/replay', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        subdomain: req.subdomain,
        request_id: req.id,
        capture: true
      })
    });
    if (!res.ok) {
      const errText = await res.text();
      alert(`Replay failed: ${errText}`);
    } else {
      const replayed = await res.json();
      await loadRequests();
      if (replayed && replayed.id) {
        selectRequest(replayed);
      }
    }
  } catch (err) {
    alert(`Replay error: ${err.message}`);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.innerText = 'Replay';
    }
  }
}

async function clearLogs() {
  requests = [];
  renderRequests();
  const emptyState = document.getElementById('empty-state');
  const content = document.getElementById('inspector-content');
  if (emptyState) emptyState.style.display = 'flex';
  if (content) content.style.display = 'none';
  selectedRequestID = null;

  const url = selectedTunnel ? `/api/requests?subdomain=${encodeURIComponent(selectedTunnel)}` : '/api/requests';
  await fetch(url, { method: 'DELETE' });
}

// Expose functions globally for inline HTML event handlers
window.connectWS = connectWS;
window.loadTunnels = loadTunnels;
window.loadRequests = loadRequests;
window.selectTunnel = selectTunnel;
window.handleNewRequest = handleNewRequest;
window.renderRequests = renderRequests;
window.selectRequest = selectRequest;
window.switchTab = switchTab;
window.togglePause = togglePause;
window.clearLogs = clearLogs;
window.replayCurrentRequest = replayCurrentRequest;

// Init
connectWS();
loadTunnels();
loadRequests();