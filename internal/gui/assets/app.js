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
    'tab.fallback': 'Fallback',
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
    'setting.catalog': 'Catalog',
    'setting.catalogNotSynced': 'Catalog not synced',
    'setting.catalogAge': 'Last synced: {age}',
    'section.proxyConfig': 'Proxy Configuration',
    'placeholder.envOrEmpty': 'Use env var or leave empty',
    'placeholder.notSet': 'Not configured',
    'label.globalKey': 'Global API Key (optional)',
    'label.host': 'Listen Address (Host)',
    'label.port': 'Listen Port (Port)',
    'btn.save': 'Save & Apply Config',
    'btn.refreshCatalog': 'Refresh catalog',
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
    'fallback.scenario': 'Scenario',
    'fallback.default': 'Default',
    'fallback.streaming': 'Streaming',
    'fallback.longContext': 'Long Context',
    'fallback.chainOrder': 'Fallback Chain Order',
    'fallback.addModel': '+ Add Model',
    'fallback.preview': 'Preview',
    'fallback.save': 'Save',
    'fallback.empty': 'No models configured',
    'fallback.previewTitle': 'Fallback Chain Preview',
    'fallback.selectModel': 'Select a model',
    'fallback.saving': 'Saving fallback chain...',
    'fallback.saved': 'Fallback chain saved successfully!',
    'fallback.saveFailed': 'Failed to save fallback chain',
    'fallback.noChanges': 'No changes to save',
    'perf.lastHour': 'Last Hour',
    'perf.last24h': 'Last 24 Hours',
    'perf.last7d': 'Last 7 Days',
    'perf.allTime': 'All Time',
    'perf.th.model': 'Model',
    'perf.th.count': 'Count',
    'perf.th.successRate': 'Success %',
    'perf.th.avg': 'Avg (ms)',
    'perf.th.p50': 'P50',
    'perf.th.p90': 'P90',
    'perf.th.p99': 'P99',
    'perf.empty': 'No performance data',
    'setting.backup': 'Backup Configuration',
    'setting.backupDesc': 'Export current config as JSON file',
    'setting.restore': 'Restore Configuration',
    'setting.restoreDesc': 'Import config from JSON file',
    'btn.export': 'Export',
    'btn.import': 'Import',
    'label.anonymize': 'Anonymize',
    'status.exporting': 'Exporting...',
    'status.exportOk': 'Config exported successfully!',
    'status.exportFail': 'Export failed: ',
    'status.importing': 'Importing...',
    'status.importOk': 'Config imported successfully!',
    'status.importFail': 'Import failed: ',
    'status.importInvalid': 'Invalid config file',
    'modal.importPreview': 'Import Preview',
    'modal.importConfirm': 'Apply this configuration?',
    'btn.apply': 'Apply',
    'btn.cancel': 'Cancel',
    'setting.testModel': 'Test Model',
    'setting.testModelDesc': 'Send a quick test request to verify model connectivity',
    'btn.testModel': 'Test Model',
    'test.title': 'Quick Model Test',
    'test.selectModel': 'Select a model...',
    'test.send': 'Send',
    'test.promptPlaceholder': 'Enter your prompt...',
    'test.latency': 'Latency:',
    'test.tokens': 'Tokens:',
    'test.copy': 'Copy',
    'test.copied': 'Copied!',
    'test.sending': 'Sending...',
    'test.noModel': 'Please select a model',
    'test.noPrompt': 'Please enter a prompt',
    'test.error': 'Error: ',
    'test.networkError': 'Network error',
    'tab.performance': 'Performance',
  },
  zh: {
    'lang.toggle': 'English',
    'status.checking': '检查中…',
    'status.running': '运行中',
    'status.stopped': '已停止',
    'status.connected': '已连接',
    'tab.overview': '概览',
    'tab.history': '历史请求',
    'tab.fallback': '降级策略',
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
    'setting.catalog': '模型目录',
    'setting.catalogNotSynced': '模型目录未同步',
    'setting.catalogAge': '上次同步：{age}',
    'section.proxyConfig': '服务代理配置',
    'placeholder.envOrEmpty': '使用环境变量或留空',
    'placeholder.notSet': '未配置',
    'label.globalKey': 'Global API Key (可选)',
    'label.host': '监听地址 (Host)',
    'label.port': '监听端口 (Port)',
    'btn.save': '保存并应用配置',
    'btn.refreshCatalog': '刷新模型目录',
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
    'setting.testModel': '测试模型',
    'setting.testModelDesc': '发送快速测试请求以验证模型连接',
    'btn.testModel': '测试模型',
    'test.title': '快速模型测试',
    'test.selectModel': '选择模型...',
    'test.send': '发送',
    'test.promptPlaceholder': '输入测试提示词...',
    'test.latency': '延迟：',
    'test.tokens': 'Token：',
    'test.copy': '复制',
    'test.copied': '已复制！',
    'test.sending': '发送中...',
    'test.noModel': '请选择模型',
    'test.noPrompt': '请输入提示词',
    'test.error': '错误：',
    'test.networkError': '网络错误',
    'fallback.scenario': '使用场景',
    'fallback.default': '默认',
    'fallback.streaming': '流式请求',
    'fallback.longContext': '长上下文',
    'fallback.chainOrder': '降级链顺序',
    'fallback.addModel': '+ 添加模型',
    'fallback.preview': '预览',
    'fallback.save': '保存',
    'fallback.empty': '未配置模型',
    'fallback.previewTitle': '降级链预览',
    'fallback.selectModel': '选择模型',
    'fallback.saving': '保存中...',
    'fallback.saved': '降级链保存成功！',
    'fallback.saveFailed': '保存失败',
    'fallback.noChanges': '无更改',
    'perf.lastHour': '最近 1 小时',
    'perf.last24h': '最近 24 小时',
    'perf.last7d': '最近 7 天',
    'perf.allTime': '全部时间',
    'perf.th.model': '模型',
    'perf.th.count': '请求数',
    'perf.th.successRate': '成功率',
    'perf.th.avg': '平均延迟',
    'perf.th.p50': 'P50',
    'perf.th.p90': 'P90',
    'perf.th.p99': 'P99',
    'perf.empty': '暂无性能数据',
    'setting.backup': '备份配置',
    'setting.backupDesc': '导出当前配置为 JSON 文件',
    'setting.restore': '恢复配置',
    'setting.restoreDesc': '从 JSON 文件导入配置',
    'btn.export': '导出',
    'btn.import': '导入',
    'label.anonymize': '脱敏',
    'status.exporting': '导出中...',
    'status.exportOk': '配置导出成功！',
    'status.exportFail': '导出失败：',
    'status.importing': '导入中...',
    'status.importOk': '配置导入成功！',
    'status.importFail': '导入失败：',
    'status.importInvalid': '无效的配置文件',
    'modal.importPreview': '导入预览',
    'modal.importConfirm': '应用此配置？',
    'btn.apply': '应用',
    'btn.cancel': '取消',
    'tab.logs': '日志',
    'tab.performance': '性能',
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
  PerfModule.render();
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

/* ── Performance Module ───────────────────────────────────────────── */
const PerfModule = {
  data: [],
  sortField: 'count',
  sortDir: 'desc',
  timeRange: 'all',

  init() {
    const timeRangeSelect = document.getElementById('perf-time-range');
    if (timeRangeSelect) {
      timeRangeSelect.addEventListener('change', (e) => {
        this.timeRange = e.target.value;
        this.refresh();
      });
    }

    document.querySelectorAll('.perf-table .sortable').forEach(th => {
      th.addEventListener('click', () => {
        const field = th.dataset.sort;
        if (this.sortField === field) {
          this.sortDir = this.sortDir === 'asc' ? 'desc' : 'asc';
        } else {
          this.sortField = field;
          this.sortDir = 'desc';
        }
        document.querySelectorAll('.perf-table .sortable').forEach(s => {
          s.classList.remove('asc', 'desc');
          s.setAttribute('aria-sort', 'none');
        });
        th.classList.add(this.sortDir);
        th.setAttribute('aria-sort', this.sortDir === 'asc' ? 'ascending' : 'descending');
        this.render();
      });
    });
  },

  async refresh() {
    try {
      const r = await fetch('/api/perf/models?range=' + encodeURIComponent(this.timeRange));
      if (!r.ok) return;
      this.data = await r.json() || [];
      this.render();
    } catch (e) {
      console.error('PerfModule refresh failed:', e);
    }
  },

  render() {
    const tbody = document.getElementById('perf-tbody');
    if (!tbody) return;

    if (this.data.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="empty-state">' + t('empty.noData') + '</td></tr>';
      return;
    }

    const sorted = [...this.data].sort((a, b) => {
      let aVal = a[this.sortField];
      let bVal = b[this.sortField];
      if (aVal == null) aVal = 0;
      if (bVal == null) bVal = 0;
      if (typeof aVal === 'string') aVal = aVal.toLowerCase();
      if (typeof bVal === 'string') bVal = bVal.toLowerCase();
      if (aVal < bVal) return this.sortDir === 'asc' ? -1 : 1;
      if (aVal > bVal) return this.sortDir === 'asc' ? 1 : -1;
      return 0;
    });

    tbody.innerHTML = sorted.map(row => {
      const successRate = row.count > 0 ? (row.success / row.count * 100).toFixed(1) : 0;
      const successClass = successRate >= 99 ? 'success-rate' : (successRate >= 95 ? '' : 'error-rate');
      return `
        <tr>
          <td class="perf-model">${escapeHtml(row.model)}</td>
          <td>${fmt(row.count)}</td>
          <td class="${successClass}">${successRate}%</td>
          <td class="${this.getLatencyClass(row.avg_ms)}">${fmt(row.avg_ms)}</td>
          <td class="${this.getLatencyClass(row.p50_ms)}">${fmt(row.p50_ms)}</td>
          <td class="${this.getLatencyClass(row.p90_ms)}">${fmt(row.p90_ms)}</td>
          <td class="${this.getLatencyClass(row.p99_ms)}">${fmt(row.p99_ms)}</td>
        </tr>
      `;
    }).join('');
  },

  getLatencyClass(ms) {
    if (ms == null) return '';
    if (ms < 1000) return 'latency-cell latency-fast';
    if (ms < 2000) return 'latency-cell latency-medium';
    return 'latency-cell latency-slow';
  }
};

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
let perfPollTimer = null;
let perfPollCounter = 0;

function startPolling() {
  refreshAll();
  PerfModule.init();
  PerfModule.refresh();
  setInterval(refreshAll, 3000);
  perfPollTimer = setInterval(() => {
    perfPollCounter++;
    if (perfPollCounter >= 2) {
      PerfModule.refresh();
      perfPollCounter = 0;
    }
  }, 3000);
}

async function refreshAll() {
  await Promise.all([refreshMetrics(), refreshHistory(), refreshConfig(), refreshCatalogAge()]);
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

/* ── /api/catalog/lock & /api/catalog/sync ─────────────────────── */
async function refreshCatalogAge() {
  try {
    const r = await fetch('/api/catalog/lock');
    if (!r.ok) return;
    const d = await r.json();
    const el = document.getElementById('catalog-age');
    if (!el) return;
    if (!d.synced) {
      el.textContent = t('setting.catalogNotSynced');
      return;
    }
    el.textContent = t('setting.catalogAge').replace('{age}', fmtAge(d.age_seconds));
  } catch(e) {}
}

async function refreshCatalog() {
  const btn = document.getElementById('btn-refresh-catalog');
  if (btn) {
    btn.disabled = true;
    btn.textContent = currentLang === 'zh' ? '同步中…' : 'Syncing…';
  }
  try {
    const r = await fetch('/api/catalog/sync', { method: 'POST' });
    if (r.ok) {
      await refreshCatalogAge();
    } else {
      const txt = await r.text();
      console.error('Catalog refresh failed:', txt);
    }
  } catch(e) {
    console.error('Catalog refresh network error:', e);
  } finally {
    if (btn) {
      btn.disabled = false;
      btn.textContent = t('btn.refreshCatalog');
    }
  }
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

function fmtAge(seconds) {
  if (seconds == null || seconds < 0) return '—';
  if (seconds < 60) return seconds + (currentLang === 'zh' ? ' 秒前' : ' seconds ago');
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60) return minutes + (currentLang === 'zh' ? ' 分钟前' : ' minutes ago');
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return hours + (currentLang === 'zh' ? ' 小时前' : ' hours ago');
  const days = Math.floor(hours / 24);
  return days + (currentLang === 'zh' ? ' 天前' : ' days ago');
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
    case 'goto-performance':
      document.querySelector('[data-tab="performance"]').click();
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
  // Tab shortcuts: Cmd/Ctrl + 1/2/3/4/5/6
  if ((e.metaKey || e.ctrlKey) && ['1', '2', '3', '4', '5', '6'].includes(e.key)) {
    e.preventDefault();
    const tabs = ['overview', 'history', 'performance', 'fallback', 'settings'];
    document.querySelector(`[data-tab="${tabs[parseInt(e.key) - 1]}"]`)?.click();
  }
  // Escape to close modals (use if-else to ensure only one action)
  if (e.key === 'Escape') {
    if (commandPaletteOpen) {
      closeCommandPalette();
    } else if (TestModule.testModal?.classList.contains('visible')) {
      TestModule.close();
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

/* ── Config Backup/Restore ─────────────────────────────────────── */
async function exportConfig() {
  const anonymize = document.getElementById('export-anonymize').checked;
  const btn = document.getElementById('btn-export-config');
  btn.disabled = true;
  btn.textContent = t('status.exporting');

  try {
    const url = '/api/config/export?anonymize=' + anonymize;
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(await response.text());
    }

    const blob = await response.blob();
    const downloadUrl = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = downloadUrl;
    a.download = 'routatic-proxy-config.json';
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(downloadUrl);

    showSaveStatus(t('status.exportOk'), 'success');
  } catch (e) {
    showSaveStatus(t('status.exportFail') + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = t('btn.export');
    applyTranslations();
  }
}

function importConfig() {
  document.getElementById('import-file').click();
}

async function handleConfigImport(file) {
  if (!file || file.type !== 'application/json') {
    showSaveStatus(t('status.importInvalid'), 'error');
    return;
  }

  const btn = document.getElementById('btn-import-config');
  btn.disabled = true;
  btn.textContent = t('status.importing');

  try {
    const content = await file.text();
    const config = JSON.parse(content);

    const previewHtml = `
      <div class="detail-row">
        <span class="detail-label">${t('modal.importConfirm')}</span>
      </div>
      <pre style="max-height: 300px; overflow: auto; background: var(--surface2); padding: 12px; border-radius: var(--radius-sm); font-size: 11px; white-space: pre-wrap; word-break: break-all;">${escapeHtml(JSON.stringify(config, null, 2))}</pre>
    `;

    modalBody.innerHTML = previewHtml;
    document.getElementById('modal-title').textContent = t('modal.importPreview');

    const footerHtml = `
      <div style="padding: 12px 16px; display: flex; gap: 8px; justify-content: flex-end; border-top: 1px solid var(--border);">
        <button class="btn btn-small" id="btn-import-cancel">${t('btn.cancel')}</button>
        <button class="btn btn-small btn-primary" id="btn-import-apply">${t('btn.apply')}</button>
      </div>
    `;

    const existingFooter = modal.querySelector('.modal-footer');
    if (existingFooter) existingFooter.remove();

    modal.querySelector('.modal-content').insertAdjacentHTML('beforeend', footerHtml);

    modal.classList.add('visible');

    document.getElementById('btn-import-cancel').onclick = () => {
      modal.classList.remove('visible');
      const footer = modal.querySelector('.modal-footer');
      if (footer) footer.remove();
    };

    document.getElementById('btn-import-apply').onclick = async () => {
      try {
        const response = await fetch('/api/config/import', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ config: config, apply: true })
        });

        if (!response.ok) {
          throw new Error(await response.text());
        }

        modal.classList.remove('visible');
        const footer = modal.querySelector('.modal-footer');
        if (footer) footer.remove();

        showSaveStatus(t('status.importOk'), 'success');
        await loadProxyConfig();
      } catch (e) {
        showSaveStatus(t('status.importFail') + e.message, 'error');
      }
    };
  } catch (e) {
    showSaveStatus(t('status.importFail') + e.message, 'error');
  } finally {
    btn.disabled = false;
    btn.textContent = t('btn.import');
    applyTranslations();
    document.getElementById('import-file').value = '';
  }
}

document.addEventListener('DOMContentLoaded', () => {
  document.getElementById('btn-export-config')?.addEventListener('click', exportConfig);
  document.getElementById('btn-import-config')?.addEventListener('click', importConfig);
  document.getElementById('import-file')?.addEventListener('change', function(e) {
    if (e.target.files && e.target.files[0]) {
      handleConfigImport(e.target.files[0]);
    }
  });
});

/* ── Fallback Chain Editor ─────────────────────────────────────── */
const FallbackModule = {
  chains: {
    default: [],
    streaming: [],
    'long-context': []
  },
  currentScenario: 'default',
  originalChains: null,
  availableModels: [],

  init() {
    this.loadConfig();
  },

  async loadConfig() {
    try {
      const r = await fetch('/api/proxy/config');
      if (!r.ok) return;
      const config = await r.json();

      this.availableModels = config.models || [];

      this.chains = {
        default: this.parseFallbackChain(config, 'default'),
        streaming: this.parseFallbackChain(config, 'streaming'),
        'long-context': this.parseFallbackChain(config, 'long_context')
      };

      this.originalChains = JSON.parse(JSON.stringify(this.chains));
      this.renderChain();
    } catch (e) {
      console.error('Failed to load fallback config:', e);
    }
  },

  parseFallbackChain(config, scenario) {
    const key = scenario === 'long-context' ? 'long_context' : scenario;
    if (config.router_config && config.router_config.scenario_fallbacks && config.router_config.scenario_fallbacks[key]) {
      return [...config.router_config.scenario_fallbacks[key]];
    }
    return [];
  },

  renderChain() {
    const list = document.getElementById('fallback-chain');
    const chain = this.chains[this.currentScenario];

    if (!chain || chain.length === 0) {
      list.innerHTML = '<li class="empty-state">' + t('fallback.empty') + '</li>';
      list.classList.remove('has-items');
      return;
    }

    list.classList.add('has-items');
    list.innerHTML = chain.map((modelId, index) => {
      const model = this.availableModels.find(m => m.id === modelId);
      const displayName = model ? (model.display_name || model.id) : modelId;
      const provider = model ? model.provider : '';
      return `
        <li class="fallback-item" draggable="true" data-index="${index}" role="option">
          <span class="handle">⋮⋮</span>
          <span class="model-name">${escapeHtml(displayName)}</span>
          ${provider ? '<span class="model-meta">' + escapeHtml(provider) + '</span>' : ''}
          <button class="remove-btn" onclick="FallbackModule.removeModel(${index})" title="Remove model" aria-label="Remove ${escapeHtml(displayName)}">×</button>
        </li>
      `;
    }).join('');

    this.setupDragDrop();
  },

  setupDragDrop() {
    const items = document.querySelectorAll('.fallback-item');

    items.forEach(item => {
      item.addEventListener('dragstart', (e) => this.onDragStart(e));
      item.addEventListener('dragover', (e) => this.onDragOver(e));
      item.addEventListener('dragleave', (e) => this.onDragLeave(e));
      item.addEventListener('drop', (e) => this.onDrop(e));
      item.addEventListener('dragend', (e) => this.onDragEnd(e));
    });
  },

  onDragStart(e) {
    e.target.classList.add('dragging');
    e.dataTransfer.effectAllowed = 'move';
    e.dataTransfer.setData('text/plain', e.target.dataset.index);
  },

  onDragOver(e) {
    e.preventDefault();
    e.dataTransfer.dropEffect = 'move';
    const dragging = document.querySelector('.fallback-item.dragging');
    if (dragging !== e.currentTarget) {
      e.currentTarget.classList.add('drag-over');
    }
  },

  onDragLeave(e) {
    e.currentTarget.classList.remove('drag-over');
  },

  onDrop(e) {
    e.preventDefault();
    const fromIndex = parseInt(e.dataTransfer.getData('text/plain'), 10);
    const toIndex = parseInt(e.currentTarget.dataset.index, 10);

    e.currentTarget.classList.remove('drag-over');

    if (fromIndex !== toIndex) {
      const chain = this.chains[this.currentScenario];
      const [removed] = chain.splice(fromIndex, 1);
      chain.splice(toIndex, 0, removed);
      this.renderChain();
    }
  },

  onDragEnd(e) {
    e.target.classList.remove('dragging');
    document.querySelectorAll('.fallback-item').forEach(item => {
      item.classList.remove('drag-over');
    });
  },

  onScenarioChange() {
    const select = document.getElementById('fallback-scenario');
    this.currentScenario = select.value;
    this.renderChain();
    document.getElementById('fallback-preview').style.display = 'none';
  },

  addModel() {
    const modelOptions = this.availableModels
      .filter(m => !this.chains[this.currentScenario].includes(m.id))
      .map(m => `<option value="${escapeHtml(m.id)}">${escapeHtml(m.display_name || m.id)} (${escapeHtml(m.provider)})</option>`)
      .join('');

    if (!modelOptions) {
      alert(currentLang === 'zh' ? '没有可用模型' : 'No available models');
      return;
    }

    const selectHtml = `<select id="new-model-select" class="filter-select">${modelOptions}</select>`;
    const confirmed = confirm(
      (currentLang === 'zh' ? '选择模型添加到降级链:\n\n' : 'Select a model to add:\n\n') +
      this.availableModels.filter(m => !this.chains[this.currentScenario].includes(m.id))
        .map(m => `${m.display_name || m.id} (${m.provider})`).join('\n')
    );

    if (confirmed) {
      const modelId = prompt(
        currentLang === 'zh' ? '输入模型ID:' : 'Enter model ID:',
        this.availableModels.filter(m => !this.chains[this.currentScenario].includes(m.id))[0]?.id || ''
      );

      if (modelId && !this.chains[this.currentScenario].includes(modelId)) {
        const model = this.availableModels.find(m => m.id === modelId);
        if (model) {
          this.chains[this.currentScenario].push(modelId);
          this.renderChain();
        } else {
          alert(currentLang === 'zh' ? '无效的模型ID' : 'Invalid model ID');
        }
      }
    }
  },

  removeModel(index) {
    this.chains[this.currentScenario].splice(index, 1);
    this.renderChain();
  },

  preview() {
    const previewEl = document.getElementById('fallback-preview');
    const contentEl = document.getElementById('fallback-preview-content');
    const chain = this.chains[this.currentScenario];

    if (!chain || chain.length === 0) {
      contentEl.innerHTML = '<div class="empty-state">' + t('fallback.empty') + '</div>';
    } else {
      contentEl.innerHTML = '<div class="fallback-preview-chain">' +
        chain.map((modelId, i) => {
          const model = this.availableModels.find(m => m.id === modelId);
          const displayName = model ? (model.display_name || model.id) : modelId;
          return `
            <span class="fallback-preview-model ${i === 0 ? 'primary' : ''}">${escapeHtml(displayName)}</span>
            ${i < chain.length - 1 ? '<span class="fallback-preview-arrow">→</span>' : ''}
          `;
        }).join('') +
        '</div>';
    }

    previewEl.style.display = 'block';
  },

  async save() {
    const hasChanges = this.originalChains && (
      JSON.stringify(this.chains) !== JSON.stringify(this.originalChains)
    );

    if (!hasChanges) {
      showSaveStatus(t('fallback.noChanges'), 'success');
      return;
    }

    const saveBtn = document.querySelector('.fallback-actions .btn-primary');
    if (saveBtn) {
      saveBtn.disabled = true;
      saveBtn.textContent = t('fallback.saving');
    }

    try {
      const patch = {
        router_config: {
          scenario_fallbacks: {
            default: this.chains.default,
            streaming: this.chains.streaming,
            long_context: this.chains['long-context']
          }
        }
      };

      const r = await fetch('/api/proxy/config', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(patch)
      });

      if (r.ok) {
        showSaveStatus(t('fallback.saved'), 'success');
        this.originalChains = JSON.parse(JSON.stringify(this.chains));
        await loadProxyConfig();
      } else {
        const txt = await r.text();
        showSaveStatus(t('fallback.saveFailed') + ': ' + txt, 'error');
      }
    } catch (e) {
      showSaveStatus(t('fallback.saveFailed'), 'error');
    } finally {
      if (saveBtn) {
        saveBtn.disabled = false;
        saveBtn.textContent = t('fallback.save');
      }
    }
  }
};

document.addEventListener('DOMContentLoaded', () => {
  FallbackModule.init();
});

/* ── Boot ──────────────────────────────────────────────────────── */
loadProxyConfig();
startPolling();

const TestModule = {
  testModal: null,
  testPrompt: null,
  testResponse: null,
  testModelSelect: null,
  testLatency: null,
  testTokens: null,
  testSendBtn: null,
  testCopyBtn: null,
  testModalClose: null,
  testHistoryHint: null,

  STORAGE_KEY: 'routatic-test-prompt-history',
  MAX_HISTORY: 5,

  init() {
    this.testModal = document.getElementById('test-modal');
    this.testPrompt = document.getElementById('test-prompt');
    this.testResponse = document.getElementById('test-response');
    this.testModelSelect = document.getElementById('test-model');
    this.testLatency = document.getElementById('test-latency');
    this.testTokens = document.getElementById('test-tokens');
    this.testSendBtn = document.getElementById('btn-test-send');
    this.testCopyBtn = document.getElementById('btn-test-copy');
    this.testModalClose = document.getElementById('test-modal-close');
    this.testHistoryHint = document.getElementById('test-history-hint');

    document.getElementById('btn-test-model')?.addEventListener('click', () => this.open());
    this.testModalClose?.addEventListener('click', () => this.close());
    this.testModal?.addEventListener('click', (e) => {
      if (e.target === this.testModal) this.close();
    });
    this.testSendBtn?.addEventListener('click', () => this.sendTest());
    this.testCopyBtn?.addEventListener('click', () => this.copyResponse());
    this.testPrompt?.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
        e.preventDefault();
        this.sendTest();
      }
    });
    this.loadHistory();
  },

  open() {
    this.populateModels();
    this.testModal?.classList.add('visible');
    if (this.testPrompt) {
      this.testPrompt.value = '';
      this.testPrompt.focus();
    }
    this.resetResponse();
  },

  close() {
    this.testModal?.classList.remove('visible');
  },

  async populateModels() {
    if (!this.testModelSelect) return;
    this.testModelSelect.innerHTML = '<option value="">Select a model...</option>';

    try {
      const r = await fetch('/api/metrics');
      if (!r.ok) return;
      const data = await r.json();
      const models = Object.keys(data.model_counts || {});
      models.sort().forEach(m => {
        const opt = document.createElement('option');
        opt.value = m;
        opt.textContent = m;
        this.testModelSelect.appendChild(opt);
      });
    } catch (e) {}
  },

  resetResponse() {
    if (this.testResponse) this.testResponse.innerHTML = '';
    if (this.testLatency) this.testLatency.textContent = '—';
    if (this.testTokens) this.testTokens.textContent = '—';
  },

  async sendTest() {
    if (!this.testPrompt || !this.testModelSelect || !this.testResponse) return;

    const model = this.testModelSelect.value;
    const prompt = this.testPrompt.value.trim();
    if (!model) {
      this.resetResponse();
      if (this.testResponse) this.testResponse.innerHTML = `<div class="error">${t('test.noModel')}</div>`;
      return;
    }
    if (!prompt) {
      this.resetResponse();
      if (this.testResponse) this.testResponse.innerHTML = `<div class="error">${t('test.noPrompt')}</div>`;
      return;
    }

    this.saveToHistory(prompt);
    this.testSendBtn.disabled = true;
    this.testSendBtn.textContent = t('test.sending');
    this.resetResponse();

    const start = performance.now();
    try {
      const r = await fetch('/v1/messages', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          model: model,
          max_tokens: 1024,
          messages: [{ role: 'user', content: prompt }]
        })
      });

      const latency = Math.round(performance.now() - start);
      if (this.testLatency) this.testLatency.textContent = latency + ' ms';

      if (!r.ok) {
        this.testResponse.innerHTML = '';
        const pre = document.createElement('pre');
        pre.textContent = t('test.error') + r.status + ': ' + (await r.text());
        this.testResponse.appendChild(pre);
        return;
      }
      const text = await r.text();
      let content = text;
      try {
        const j = JSON.parse(text);
        if (j.content && Array.isArray(j.content)) {
          content = j.content.map(c => c.text || '').join('\n');
        } else if (j.error) {
          content = 'Error: ' + (j.error.message || JSON.stringify(j.error));
        }
      } catch (_) {}

      const pre = document.createElement('pre');
      pre.textContent = content;
      this.testResponse.innerHTML = '';
      this.testResponse.appendChild(pre);

      const usage = this.extractUsage(text);
      if (usage && this.testTokens) {
        this.testTokens.textContent = `${usage.input || 0} in / ${usage.output || 0} out`;
      }
    } catch (e) {
      const pre = document.createElement('pre');
      pre.textContent = t('test.error') + e.message;
      this.testResponse.innerHTML = '';
      this.testResponse.appendChild(pre);
    } finally {
      this.testSendBtn.disabled = false;
      this.testSendBtn.textContent = t('test.send');
    }
  },

  extractUsage(text) {
    try {
      const j = JSON.parse(text);
      if (j.usage) return { input: j.usage.input_tokens, output: j.usage.output_tokens };
    } catch (_) {}
    const m = text.match(/"input_tokens":\s*(\d+).*?"output_tokens":\s*(\d+)/s);
    if (m) return { input: parseInt(m[1]), output: parseInt(m[2]) };
    return null;
  },

  loadHistory() {
    try {
      const history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
      if (history.length > 0 && this.testHistoryHint) {
        this.testHistoryHint.innerHTML = history.slice(0, this.MAX_HISTORY)
          .map(p => `<span title="${escapeHtml(p)}">${escapeHtml(p.substring(0, 20))}${p.length > 20 ? '...' : ''}</span>`)
          .join('');
        this.testHistoryHint.querySelectorAll('span').forEach((el, i) => {
          el.addEventListener('click', () => {
            const history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
            if (history[i]) {
              this.testPrompt.value = history[i];
              this.testPrompt.focus();
            }
          });
        });
      }
    } catch (e) {}
  },

  saveToHistory(prompt) {
    try {
      let history = JSON.parse(localStorage.getItem(this.STORAGE_KEY) || '[]');
      history = [prompt, ...history.filter(p => p !== prompt)].slice(0, this.MAX_HISTORY);
      localStorage.setItem(this.STORAGE_KEY, JSON.stringify(history));
      this.loadHistory();
    } catch (e) {}
  },

  async copyResponse() {
    const pre = this.testResponse.querySelector('pre');
    if (!pre || !pre.textContent) return;

    try {
      await navigator.clipboard.writeText(pre.textContent);
      const originalText = this.testCopyBtn.innerHTML;
      this.testCopyBtn.innerHTML = `<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" style="vertical-align: middle; margin-right: 4px;"><polyline points="20 6 9 17 4 12"></polyline></svg>${t('test.copied')}`;
      this.testCopyBtn.classList.add('copied');
      setTimeout(() => {
        this.testCopyBtn.innerHTML = originalText;
        this.testCopyBtn.classList.remove('copied');
        this.testCopyBtn.classList.remove('copied');
      }, 2000);
    } catch (e) {}
  }
};

document.addEventListener('DOMContentLoaded', () => TestModule.init());

