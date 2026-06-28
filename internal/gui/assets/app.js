/* ── i18n ────────────────────────────────────────────────────────── */
const TRANSLATIONS = {
  en: {
    'lang.toggle': '中文',
    'status.checking': 'Checking…',
    'status.running': 'Running',
    'status.stopped': 'Stopped',
    'status.connected': 'Connected',
    'tab.overview': 'Overview',
    'tab.history': 'History',
    'tab.settings': 'Settings',
    'metric.total': 'Total Requests',
    'metric.success': 'Success',
    'metric.failed': 'Failed',
    'metric.streamed': 'Streamed',
    'section.modelDist': 'Model Distribution',
    'empty.noData': 'No data yet',
    'filter.allModels': 'All Models',
    'th.time': 'Time',
    'th.model': 'Model',
    'th.scenario': 'Scenario',
    'th.inputTokens': 'Input Tokens',
    'th.outputTokens': 'Output Tokens',
    'th.duration': 'Duration',
    'th.status': 'Status',
    'empty.noHistory': 'No history yet',
    'setting.proxy': 'Proxy Service',
    'setting.proxyDesc': 'Start or stop the proxy HTTP service',
    'setting.autostart': 'Start on Boot',
    'setting.autostartDesc': 'Auto-start routatic-proxy at login (launchd)',
    'setting.notify': 'Desktop Notifications',
    'setting.notifyDesc': 'Notify on failures or model switches',
    'setting.language': 'Language',
    'setting.languageDesc': 'Switch interface language',
    'section.proxyConfig': 'Proxy Configuration',
    'placeholder.envOrEmpty': 'Use env var or leave empty',
    'placeholder.notSet': 'Not configured',
    'label.globalKey': 'Global API Key (optional)',
    'label.host': 'Listen Address (Host)',
    'label.port': 'Listen Port (Port)',
    'btn.save': 'Save & Apply Config',
    'status.saving': 'Saving…',
    'status.saveOk': 'Config saved successfully!',
    'status.saveFail': 'Save failed: ',
    'status.networkError': 'Network error, save failed',
    'status.count': ' entries',
    'status.filtered': ' (filtered)',
    'badge.success': 'Success',
    'badge.fail': 'Fail',
    'port.info': 'Listening port: —',
    'save.unloaded': 'Config not loaded, cannot save',
  },
  zh: {
    'lang.toggle': 'English',
    'status.checking': '检查中…',
    'status.running': '运行中',
    'status.stopped': '已停止',
    'status.connected': '已连接',
    'tab.overview': '概览',
    'tab.history': '历史请求',
    'tab.settings': '设置',
    'metric.total': '总请求数',
    'metric.success': '成功',
    'metric.failed': '失败',
    'metric.streamed': '流式请求',
    'section.modelDist': '模型调用分布',
    'empty.noData': '暂无数据',
    'filter.allModels': '全部模型',
    'th.time': '时间',
    'th.model': '模型',
    'th.scenario': '场景',
    'th.inputTokens': '输入 Token',
    'th.outputTokens': '输出 Token',
    'th.duration': '耗时',
    'th.status': '状态',
    'empty.noHistory': '暂无历史请求',
    'setting.proxy': '代理服务',
    'setting.proxyDesc': '启动或停止代理 HTTP 服务',
    'setting.autostart': '开机自启',
    'setting.autostartDesc': '登录时自动启动 routatic-proxy（launchd）',
    'setting.notify': '桌面通知',
    'setting.notifyDesc': '请求失败或切换模型时发送系统通知',
    'setting.language': '语言',
    'setting.languageDesc': '切换界面语言',
    'section.proxyConfig': '服务代理配置',
    'placeholder.envOrEmpty': '使用环境变量或留空',
    'placeholder.notSet': '未配置',
    'label.globalKey': 'Global API Key (可选)',
    'label.host': '监听地址 (Host)',
    'label.port': '监听端口 (Port)',
    'btn.save': '保存并应用配置',
    'status.saving': '保存中…',
    'status.saveOk': '配置保存并应用成功！',
    'status.saveFail': '保存失败: ',
    'status.networkError': '网络错误，保存失败',
    'status.count': ' 条',
    'status.filtered': '（已筛选）',
    'badge.success': '成功',
    'badge.fail': '失败',
    'port.info': '监听端口：—',
    'save.unloaded': '未加载当前配置，无法保存',
  }
};

let currentLang = localStorage.getItem('routatic-proxy-lang') || 'en';

function t(key) {
  return (TRANSLATIONS[currentLang] && TRANSLATIONS[currentLang][key]) || key;
}

function applyTranslations() {
  // Update all data-i18n elements
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    el.textContent = t(key);
  });
  // Update placeholder attributes for inputs
  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    const key = el.getAttribute('data-i18n-placeholder');
    el.placeholder = t(key);
  });
  // Update the language toggle text
  const langBtn = document.getElementById('btn-lang-toggle');
  if (langBtn) {
    langBtn.innerHTML = '<span data-i18n="lang.toggle">' + t('lang.toggle') + '</span>';
  }
}

function toggleLanguage() {
  currentLang = currentLang === 'en' ? 'zh' : 'en';
  localStorage.setItem('routatic-proxy-lang', currentLang);
  document.documentElement.lang = currentLang;
  applyTranslations();
  // Re-render dynamic content
  renderModelList(lastModelCounts);
  renderHistory();
}

// Apply translations on load
document.addEventListener('DOMContentLoaded', () => {
  document.documentElement.lang = currentLang;
  applyTranslations();
});

/* global state */
let allHistory = [];
let currentFilter = '';
let lastModelCounts = {};

/* ── Tab switching ─────────────────────────────────────────────── */
document.querySelectorAll('.tab').forEach(tab => {
  tab.addEventListener('click', () => {
    document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
    document.querySelectorAll('.tab-content').forEach(c => c.classList.remove('active'));
    tab.classList.add('active');
    document.getElementById('tab-' + tab.dataset.tab).classList.add('active');
  });
});

/* ── Polling ───────────────────────────────────────────────────── */
function startPolling() {
  refreshAll();
  setInterval(refreshAll, 3000);
}

async function refreshAll() {
  await Promise.all([refreshMetrics(), refreshHistory(), refreshConfig()]);
}

// Debounced refresh for manual triggers (keyboard shortcuts)
let refreshDebounceTimer = null;
function debouncedRefresh() {
  if (refreshDebounceTimer) clearTimeout(refreshDebounceTimer);
  refreshDebounceTimer = setTimeout(() => {
    refreshAll();
    refreshDebounceTimer = null;
  }, 300);
}

/* ── /api/metrics ──────────────────────────────────────────────── */
async function refreshMetrics() {
  try {
    const r = await fetch('/api/metrics');
    if (!r.ok) return;
    const d = await r.json();

    // status badge
    const running = d.proxy_running;
    const connected = d.connected_to_existing;
    const dot  = document.getElementById('status-dot');
    const text = document.getElementById('status-text');
    dot.className = 'status-dot ' + (running ? 'running' : 'stopped');
    if (running && connected) {
      text.textContent = t('status.connected');
    } else if (running) {
      text.textContent = t('status.running');
    } else {
      text.textContent = t('status.stopped');
    }

    // metric cards
    document.getElementById('m-total').textContent   = fmt(d.requests_received);
    document.getElementById('m-success').textContent = fmt(d.requests_success);
    document.getElementById('m-failed').textContent  = fmt(d.requests_failed);
    document.getElementById('m-streamed').textContent = fmt(d.requests_streamed);

    // port info
    const portEl = document.getElementById('port-info');
    if (d.port) {
      portEl.textContent = (currentLang === 'zh' ? '监听端口：' : 'Listening port: ') + d.port;
    }

    // model list
    lastModelCounts = d.model_counts || {};
    renderModelList(lastModelCounts);

    // proxy toggle sync
    const proxyToggle = document.getElementById('toggle-proxy');
    if (proxyToggle && !proxyToggle._changing) proxyToggle.checked = running;
  } catch(e) { /* server may not be ready yet */ }
}

function renderModelList(counts) {
  lastModelCounts = counts;
  const list = document.getElementById('model-list');
  const entries = Object.entries(counts).sort((a, b) => b[1] - a[1]);
  if (entries.length === 0) {
    list.innerHTML = '<div class="empty-state">' + t('empty.noData') + '</div>';
    return;
  }
  const max = entries[0][1];
  list.innerHTML = entries.slice(0, 10).map(([model, count]) => `
    <div class="model-row">
      <div class="model-name" title="${escapeHtml(model)}">${escapeHtml(model)}</div>
      <div class="model-bar-wrap">
        <div class="model-bar" style="width:${Math.round(count/max*100)}%"></div>
      </div>
      <div class="model-count">${count}</div>
    </div>
  `).join('');
}

/* ── /api/history ──────────────────────────────────────────────── */
async function refreshHistory() {
  try {
    const r = await fetch('/api/history');
    if (!r.ok) return;
    allHistory = await r.json() || [];
    renderHistory();
    updateModelFilter();
  } catch(e) {}
}

function renderHistory() {
  const tbody = document.getElementById('history-tbody');

  // Apply filter
  let filtered = currentFilter
    ? allHistory.filter(h => h.model === currentFilter)
    : allHistory;

  // Apply search
  if (searchQuery) {
    filtered = filtered.filter(h => {
      return (h.model || '').toLowerCase().includes(searchQuery) ||
             (h.scenario || '').toLowerCase().includes(searchQuery) ||
             (h.provider || '').toLowerCase().includes(searchQuery);
    });
  }

  // Apply sort
  filtered = sortHistory(filtered);

  document.getElementById('history-count').textContent =
    filtered.length + t('status.count') + (currentFilter ? t('status.filtered') : '');

  if (filtered.length === 0) {
    tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + t('empty.noHistory') + '</td></tr>';
    return;
  }

  tbody.innerHTML = filtered.map(h => {
    // Use composite key to ensure uniqueness when multiple requests occur in the same second
    const rowId = `${h.start_time}_${h.model || 'unknown'}_${h.duration_ms || 0}`;
    return `
    <tr data-id="${escapeHtml(rowId)}" style="cursor: pointer;">
      <td>${fmtTime(h.start_time)}</td>
      <td><span title="${escapeHtml(h.provider || '')}">${escapeHtml(h.model) || '—'}</span></td>
      <td><span class="badge badge-scene">${escapeHtml(h.scenario) || '—'}</span></td>
      <td>${h.input_tokens != null ? h.input_tokens.toLocaleString() : '—'}</td>
      <td>${h.output_tokens != null ? h.output_tokens.toLocaleString() : '—'}</td>
      <td>${fmtDuration(h.duration_ms)}</td>
      <td><span class="badge ${h.success ? 'badge-success' : 'badge-error'}">${h.success ? t('badge.success') : t('badge.fail')}</span></td>
    </tr>
  `}).join('');

  // Add click handlers for detail modal
  tbody.querySelectorAll('tr[data-id]').forEach(row => {
    row.addEventListener('click', function() {
      const rowId = this.dataset.id;
      // Parse the composite key to find the matching record
      const record = filtered.find(h => {
        const expectedId = `${h.start_time}_${h.model || 'unknown'}_${h.duration_ms || 0}`;
        return expectedId === rowId;
      });
      if (record) showHistoryDetail(record);
    });
  });
}

function updateModelFilter() {
  const sel = document.getElementById('model-filter');
  const current = sel.value;
  const models = [...new Set(allHistory.map(h => h.model).filter(Boolean))].sort();
  sel.innerHTML = '<option value="">' + t('filter.allModels') + '</option>' +
    models.map(m => `<option value="${escapeHtml(m)}" ${m===current?'selected':''}>${escapeHtml(m)}</option>`).join('');
  sel.value = current;
}

document.getElementById('model-filter').addEventListener('change', function() {
  currentFilter = this.value;
  renderHistory();
});

/* ── /api/config ───────────────────────────────────────────────── */
async function refreshConfig() {
  try {
    const r = await fetch('/api/config');
    if (!r.ok) return;
    const d = await r.json();
    const autostartToggle = document.getElementById('toggle-autostart');
    const notifyToggle    = document.getElementById('toggle-notify');
    if (autostartToggle && !autostartToggle._changing) autostartToggle.checked = !!d.autostart;
    if (notifyToggle    && !notifyToggle._changing)    notifyToggle.checked    = !!d.notify;
  } catch(e) {}
}

/* ── Toggle actions ────────────────────────────────────────────── */
async function toggleProxy(el) {
  el._changing = true;
  try {
    const action = el.checked ? 'start' : 'stop';
    const r = await fetch('/api/proxy/' + action, { method: 'POST' });
    if (!r.ok) { el.checked = !el.checked; }
  } catch(e) { el.checked = !el.checked; }
  setTimeout(() => { el._changing = false; }, 1000);
}

async function toggleAutostart(el) {
  el._changing = true;
  try {
    const r = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ autostart: el.checked })
    });
    if (!r.ok) { el.checked = !el.checked; }
  } catch(e) { el.checked = !el.checked; }
  setTimeout(() => { el._changing = false; }, 1000);
}

async function toggleNotify(el) {
  el._changing = true;
  try {
    const r = await fetch('/api/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ notify: el.checked })
    });
    if (!r.ok) { el.checked = !el.checked; }
  } catch(e) { el.checked = !el.checked; }
  setTimeout(() => { el._changing = false; }, 1000);
}

/* ── Helpers ───────────────────────────────────────────────────── */
function fmt(n) { return n != null ? Number(n).toLocaleString() : '—'; }

function escapeHtml(str) {
  if (!str && str !== 0) return '';
  return String(str).replace(/[&<>"']/g, function(c) {
    return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#039;'})[c];
  });
}

function fmtTime(iso) {
  if (!iso) return '—';
  const d = new Date(iso);
  const hh = d.getHours().toString().padStart(2,'0');
  const mm = d.getMinutes().toString().padStart(2,'0');
  const ss = d.getSeconds().toString().padStart(2,'0');
  return hh + ':' + mm + ':' + ss;
}

function fmtDuration(ms) {
  if (!ms && ms !== 0) return '—';
  if (ms < 1000) return ms + ' ms';
  return (ms / 1000).toFixed(1) + ' s';
}

/* ── Proxy Config Form ─────────────────────────────────────────── */
let currentProxyConfig = null;

// Map of config field paths to element IDs for loading and saving.
// Each entry: [jsonPath, elementId, type, transform]
const CONFIG_FIELDS = [
  // Server
  ['host', 'cfg-host', 'string'],
  ['port', 'cfg-port', 'int'],
  ['api_key', 'cfg-global-key', 'string'],
  ['hot_reload', 'cfg-hot-reload', 'bool'],

  // OpenCode Go
  ['opencode_go.base_url', 'cfg-go-base-url', 'string'],
  ['opencode_go.anthropic_base_url', 'cfg-go-anthropic-url', 'string'],
  ['opencode_go.api_key', 'cfg-go-api-key', 'string'],
  ['opencode_go.timeout_ms', 'cfg-go-timeout', 'int'],
  ['opencode_go.stream_timeout_ms', 'cfg-go-stream-timeout', 'int'],

  // OpenCode Zen
  ['opencode_zen.base_url', 'cfg-zen-base-url', 'string'],
  ['opencode_zen.anthropic_base_url', 'cfg-zen-anthropic-url', 'string'],
  ['opencode_zen.responses_base_url', 'cfg-zen-responses-url', 'string'],
  ['opencode_zen.gemini_base_url', 'cfg-zen-gemini-url', 'string'],
  ['opencode_zen.api_key', 'cfg-zen-api-key', 'string'],
  ['opencode_zen.timeout_ms', 'cfg-zen-timeout', 'int'],
  ['opencode_zen.stream_timeout_ms', 'cfg-zen-stream-timeout', 'int'],

  // AWS Bedrock
  ['aws_bedrock.base_url', 'cfg-bedrock-base-url', 'string'],
  ['aws_bedrock.anthropic_base_url', 'cfg-bedrock-anthropic-url', 'string'],
  ['aws_bedrock.api_key', 'cfg-bedrock-api-key', 'string'],
  ['aws_bedrock.project_id', 'cfg-bedrock-project-id', 'string'],
  ['aws_bedrock.timeout_ms', 'cfg-bedrock-timeout', 'int'],
  ['aws_bedrock.stream_timeout_ms', 'cfg-bedrock-stream-timeout', 'int'],

  // Logging
  ['logging.level', 'cfg-log-level', 'string'],
];

// Deep-set a value in an object by dot-separated path.
function deepSet(obj, path, value) {
  const parts = path.split('.');
  let cur = obj;
  for (let i = 0; i < parts.length - 1; i++) {
    if (!cur[parts[i]] || typeof cur[parts[i]] !== 'object') cur[parts[i]] = {};
    cur = cur[parts[i]];
  }
  cur[parts[parts.length - 1]] = value;
}

// Deep-get a value from an object by dot-separated path.
function deepGet(obj, path) {
  return path.split('.').reduce((o, k) => (o != null ? o[k] : undefined), obj);
}

// Read a field from the form and produce its typed value (or undefined if unchanged).
function readFieldValue(field) {
  const el = document.getElementById(field[1]);
  if (!el) return undefined;
  const raw = el.value !== undefined ? el.value : '';
  if (field[2] === 'bool') {
    const v = el.checked;
    // Compare with current config to detect actual changes
    const current = deepGet(currentProxyConfig, field[0]);
    return v === !!current ? undefined : v;
  }
  if (field[2] === 'int') {
    const v = raw.trim() === '' ? undefined : parseInt(raw, 10);
    const current = deepGet(currentProxyConfig, field[0]);
    return v === current ? undefined : v;
  }
  // string
  const v = raw;
  const current = deepGet(currentProxyConfig, field[0]);
  return v === (current || '') ? undefined : v;
}

async function loadProxyConfig() {
  try {
    const r = await fetch('/api/proxy/config');
    if (!r.ok) return;
    currentProxyConfig = await r.json();
    if (!currentProxyConfig) return;

    for (const [path, id, type] of CONFIG_FIELDS) {
      const el = document.getElementById(id);
      if (!el) continue;
      const val = deepGet(currentProxyConfig, path);
      if (type === 'bool') {
        el.checked = !!val;
      } else if (type === 'int') {
        el.value = val != null ? val : '';
      } else {
        el.value = val || '';
      }
    }
  } catch (e) {
    console.error('Failed to load proxy config:', e);
  }
}

async function saveProxyConfig() {
  if (!currentProxyConfig) {
    showSaveStatus('Config not loaded, cannot save', 'error');
    return;
  }

  const saveBtn = document.getElementById('btn-save-cfg');
  saveBtn.disabled = true;
  saveBtn.textContent = 'Saving...';

  // Build a patch object with only changed fields.
  const patch = {};
  for (const field of CONFIG_FIELDS) {
    const v = readFieldValue(field);
    if (v !== undefined) {
      deepSet(patch, field[0], v);
    }
  }

  // If nothing changed, no-op.
  if (Object.keys(patch).length === 0) {
    showSaveStatus('No changes to save', 'success');
    saveBtn.disabled = false;
    saveBtn.textContent = 'Save & Apply Config';
    return;
  }

  try {
    const r = await fetch('/api/proxy/config', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(patch)
    });

    if (r.ok) {
      showSaveStatus('Config saved successfully!', 'success');
      // Reload the full config from the server to stay in sync.
      await loadProxyConfig();
    } else {
      const txt = await r.text();
      showSaveStatus('Save failed: ' + txt, 'error');
    }
  } catch (e) {
    showSaveStatus('Network error, save failed', 'error');
  } finally {
    saveBtn.disabled = false;
    saveBtn.textContent = 'Save & Apply Config';
  }
}

function showSaveStatus(msg, type) {
  const status = document.getElementById('save-status');
  status.textContent = msg;
  status.className = 'save-status ' + type;
  setTimeout(() => {
    status.textContent = '';
    status.className = 'save-status';
  }, 4000);
}

function togglePasswordVisibility(id) {
  const input = document.getElementById(id);
  if (input.type === 'password') {
    input.type = 'text';
  } else {
    input.type = 'password';
  }
}

/* ── History Search ────────────────────────────────────────────── */
let searchQuery = '';

document.getElementById('history-search')?.addEventListener('input', function(e) {
  searchQuery = e.target.value.toLowerCase().trim();
  renderHistory();
});

/* ── History Sorting ───────────────────────────────────────────── */
let currentSort = { field: 'time', dir: 'desc' };

document.querySelectorAll('.sortable').forEach(th => {
  th.addEventListener('click', function() {
    const field = this.dataset.sort;
    if (currentSort.field === field) {
      currentSort.dir = currentSort.dir === 'asc' ? 'desc' : 'asc';
    } else {
      currentSort.field = field;
      currentSort.dir = 'desc';
    }
    // Update visual indicators and aria-sort
    document.querySelectorAll('.sortable').forEach(s => {
      s.classList.remove('asc', 'desc');
      s.setAttribute('aria-sort', 'none');
    });
    this.classList.add(currentSort.dir);
    this.setAttribute('aria-sort', currentSort.dir === 'asc' ? 'ascending' : 'descending');
    renderHistory();
  });
});

function sortHistory(history) {
  return [...history].sort((a, b) => {
    let aVal = a[currentSort.field];
    let bVal = b[currentSort.field];
    if (aVal == null) aVal = '';
    if (bVal == null) bVal = '';
    if (typeof aVal === 'string') aVal = aVal.toLowerCase();
    if (typeof bVal === 'string') bVal = bVal.toLowerCase();
    if (aVal < bVal) return currentSort.dir === 'asc' ? -1 : 1;
    if (aVal > bVal) return currentSort.dir === 'asc' ? 1 : -1;
    return 0;
  });
}

/* ── History Detail Modal ──────────────────────────────────────── */
const modal = document.getElementById('history-modal');
const modalBody = document.getElementById('modal-body');
const modalClose = document.getElementById('modal-close');

function showHistoryDetail(record) {
  modalBody.innerHTML = `
    <div class="detail-row">
      <span class="detail-label">Time</span>
      <span class="detail-value">${fmtTime(record.start_time)}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Model</span>
      <span class="detail-value">${escapeHtml(record.model || '—')}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Provider</span>
      <span class="detail-value">${escapeHtml(record.provider || '—')}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Scenario</span>
      <span class="detail-value">${escapeHtml(record.scenario || '—')}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Input Tokens</span>
      <span class="detail-value">${record.input_tokens != null ? record.input_tokens.toLocaleString() : '—'}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Output Tokens</span>
      <span class="detail-value">${record.output_tokens != null ? record.output_tokens.toLocaleString() : '—'}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Duration</span>
      <span class="detail-value">${fmtDuration(record.duration_ms)}</span>
    </div>
    <div class="detail-row">
      <span class="detail-label">Status</span>
      <span class="detail-value" style="color: var(--${record.success ? 'success' : 'error'})">${record.success ? 'Success' : 'Failed'}</span>
    </div>
  `;
  modal.classList.add('visible');
}

function closeHistoryModal() {
  modal.classList.remove('visible');
}

modalClose?.addEventListener('click', closeHistoryModal);
modal?.addEventListener('click', function(e) {
  if (e.target === modal) closeHistoryModal();
});

/* ── Command Palette ───────────────────────────────────────────── */
const commandPalette = document.getElementById('command-palette');
const commandInput = document.getElementById('command-input');
let commandPaletteOpen = false;

function openCommandPalette() {
  commandPaletteOpen = true;
  commandPalette.classList.add('visible');
  commandInput.value = '';
  commandInput.focus();
  updateCommandList('');
}

function closeCommandPalette() {
  commandPaletteOpen = false;
  commandPalette.classList.remove('visible');
}

function updateCommandList(query) {
  const items = document.querySelectorAll('.command-item');
  const q = query.toLowerCase();
  let firstVisible = null;
  items.forEach(item => {
    const label = item.querySelector('.command-item-label').textContent.toLowerCase();
    const isVisible = label.includes(q);
    item.classList.toggle('hidden', !isVisible);
    if (isVisible && !firstVisible) firstVisible = item;
  });
  // Update aria-activedescendant to first visible item
  const commandInput = document.getElementById('command-input');
  if (firstVisible) {
    commandInput?.setAttribute('aria-activedescendant', firstVisible.id);
  } else {
    commandInput?.setAttribute('aria-activedescendant', '');
  }
}

commandInput?.addEventListener('input', function(e) {
  updateCommandList(e.target.value);
});

commandInput?.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeCommandPalette();
  } else if (e.key === 'Enter') {
    const selected = document.querySelector('.command-item.selected') || document.querySelector('.command-item:not(.hidden)');
    if (selected) executeCommand(selected.dataset.action);
    closeCommandPalette();
  }
});

document.querySelectorAll('.command-item').forEach(item => {
  item.addEventListener('click', function() {
    executeCommand(this.dataset.action);
    closeCommandPalette();
  });
});

function executeCommand(action) {
  switch (action) {
    case 'start-proxy':
      document.getElementById('toggle-proxy').checked = true;
      toggleProxy(document.getElementById('toggle-proxy'));
      break;
    case 'stop-proxy':
      document.getElementById('toggle-proxy').checked = false;
      toggleProxy(document.getElementById('toggle-proxy'));
      break;
    case 'goto-overview':
      document.querySelector('[data-tab="overview"]').click();
      break;
    case 'goto-history':
      document.querySelector('[data-tab="history"]').click();
      break;
    case 'goto-settings':
      document.querySelector('[data-tab="settings"]').click();
      break;
    case 'refresh':
      debouncedRefresh();
      break;
  }
}

commandPalette?.addEventListener('click', function(e) {
  if (e.target === commandPalette) closeCommandPalette();
});

/* ── Keyboard Shortcuts ───────────────────────────────────────── */
document.addEventListener('keydown', function(e) {
  // Command palette: Cmd/Ctrl + K
  if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
    e.preventDefault();
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else {
      openCommandPalette();
    }
  }
  // Refresh: Cmd/Ctrl + R
  if ((e.metaKey || e.ctrlKey) && e.key === 'r') {
    e.preventDefault();
    debouncedRefresh();
  }
  // Search history: Cmd/Ctrl + F
  if ((e.metaKey || e.ctrlKey) && e.key === 'f') {
    const historyTab = document.getElementById('tab-history');
    if (historyTab.classList.contains('active')) {
      e.preventDefault();
      document.getElementById('history-search')?.focus();
    }
  }
  // Tab shortcuts: Cmd/Ctrl + 1/2/3
  if ((e.metaKey || e.ctrlKey) && ['1', '2', '3'].includes(e.key)) {
    e.preventDefault();
    const tabs = ['overview', 'history', 'settings'];
    document.querySelector(`[data-tab="${tabs[parseInt(e.key) - 1]}"]`)?.click();
  }
  // Escape to close modals (use if-else to ensure only one action)
  if (e.key === 'Escape') {
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else if (modal.classList.contains('visible')) {
      closeHistoryModal();
    }
  }
});

/* ── Accordion Sections ────────────────────────────────────────── */
function initAccordions() {
  document.querySelectorAll('.accordion-header').forEach(header => {
    header.addEventListener('click', function() {
      const section = this.closest('.accordion-section');
      const wasExpanded = section.classList.contains('expanded');

      // Collapse all other sections (optional: remove for multi-expand)
      document.querySelectorAll('.accordion-section').forEach(s => {
        s.classList.remove('expanded');
      });

      // Toggle this section
      if (!wasExpanded) {
        section.classList.add('expanded');
      }
    });
  });
}

// Initialize on load
document.addEventListener('DOMContentLoaded', initAccordions);

/* ── Boot ──────────────────────────────────────────────────────── */
loadProxyConfig();
startPolling();

