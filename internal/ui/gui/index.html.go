package gui

// indexHTML is the embedded WebView UI.
const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>ollama-mgr</title>
<style>
  :root {
    --bg: #0f1419;
    --panel: #1a2332;
    --border: #2d3a4d;
    --text: #e7ecf3;
    --muted: #8b9bb4;
    --accent: #3b82f6;
    --ok: #22c55e;
    --warn: #f59e0b;
    --bad: #ef4444;
    --row-hover: #243044;
  }
  * { box-sizing: border-box; }
  body {
    margin: 0; font-family: "Segoe UI", system-ui, sans-serif;
    background: var(--bg); color: var(--text); height: 100vh;
    display: flex; flex-direction: column;
  }
  header {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap;
    padding: 10px 14px; background: var(--panel); border-bottom: 1px solid var(--border);
  }
  h1 { font-size: 16px; margin: 0 12px 0 0; font-weight: 600; letter-spacing: 0.02em; }
  button {
    background: #243044; color: var(--text); border: 1px solid var(--border);
    border-radius: 6px; padding: 6px 12px; cursor: pointer; font-size: 13px;
  }
  button:hover { border-color: var(--accent); }
  button.primary { background: var(--accent); border-color: var(--accent); color: #fff; }
  button.danger { background: #3f1d1d; border-color: #7f1d1d; }
  button:disabled { opacity: 0.5; cursor: not-allowed; }
  #status {
    padding: 6px 14px; font-size: 12px; color: var(--muted);
    border-bottom: 1px solid var(--border); background: #121820;
  }
  #selectionBar {
    padding: 6px 14px; font-size: 12px; color: var(--text);
    border-bottom: 1px solid var(--border); background: #152033;
    display: none; align-items: center; gap: 10px; flex-wrap: wrap;
  }
  #selectionBar.visible { display: flex; }
  main { flex: 1; overflow: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 13px; }
  th {
    text-align: left; position: sticky; top: 0; background: #152033;
    padding: 8px 10px; border-bottom: 1px solid var(--border); color: var(--muted);
    font-weight: 600; user-select: none; white-space: nowrap;
  }
  th.sortable { cursor: pointer; }
  th.sortable:hover { color: var(--text); }
  th.sortable.active { color: var(--accent); }
  th.sortable .ind { font-size: 10px; margin-left: 4px; opacity: 0.85; }
  th.chk, td.chk { width: 36px; text-align: center; }
  td { padding: 7px 10px; border-bottom: 1px solid #1c2533; }
  tr.data { cursor: pointer; }
  tr.data:hover { background: var(--row-hover); }
  tr.data.selected { background: #1e3a5f; }
  tr.job-row { background: #1a2838; }
  tr.job-row:hover { background: #223448; }
  tr.pending-delete td { opacity: 0.85; }
  a { color: #7dd3fc; text-decoration: none; }
  a:hover { text-decoration: underline; }
  .badge-ok { color: var(--ok); }
  .badge-warn { color: var(--warn); }
  .badge-bad { color: var(--bad); }
  .badge-dl { color: #38bdf8; }
  .badge-pend { color: #fb923c; font-weight: 600; }
  .prog {
    display: inline-block; min-width: 120px; height: 8px; background: #243044;
    border-radius: 4px; overflow: hidden; vertical-align: middle; margin-right: 6px;
  }
  .prog > i { display: block; height: 100%; background: var(--accent); width: 0%; }
  .view-toggle { display: inline-flex; border: 1px solid var(--border); border-radius: 6px; overflow: hidden; margin-right: 8px; }
  .view-toggle button { border: none; border-radius: 0; background: transparent; }
  .view-toggle button.on { background: var(--accent); color: #fff; }
  .pills { display: flex; flex-wrap: wrap; gap: 6px; align-items: center; }
  .pill {
    display: inline-flex; align-items: center; gap: 4px;
    border-radius: 999px; padding: 2px 10px; font-size: 12px; line-height: 1.5;
    border: 1px solid var(--border); color: var(--muted); background: transparent;
    cursor: default; user-select: none;
  }
  .pill.feat { border-color: #4338ca; color: #a5b4fc; background: #1e1b4b; }
  .pill.feat.lib-only { opacity: 0.65; border-style: dashed; }
  .pill.size-in {
    border-color: #15803d; color: #bbf7d0; background: #14532d; cursor: pointer; font-weight: 600;
  }
  .pill.size-out {
    border-color: #334155; color: var(--muted); background: #0f172a; cursor: pointer;
    border-style: dashed;
  }
  .pill.size-out:hover { border-color: var(--accent); color: var(--text); }
  .pill.size-in:hover { filter: brightness(1.1); }
  .pill .sub { font-size: 10px; opacity: 0.8; font-weight: 400; }
  .family-base { font-weight: 600; font-size: 14px; }
  .family-meta { color: var(--muted); font-size: 11px; margin-top: 2px; }
  .expand-tags { margin-top: 6px; padding-left: 8px; border-left: 2px solid var(--border); font-size: 12px; color: var(--muted); }
  .expand-tags div { padding: 2px 0; cursor: pointer; }
  .expand-tags div:hover { color: var(--text); }
  #tagView.hidden, #familyView.hidden, #familyLegend.hidden { display: none; }
  .legend { font-size: 11px; color: var(--muted); padding: 4px 14px; border-bottom: 1px solid var(--border); }
  dialog {
    border: 1px solid var(--border); border-radius: 10px; background: var(--panel);
    color: var(--text); padding: 0; min-width: 420px; max-width: 92vw;
  }
  dialog::backdrop { background: rgba(0,0,0,0.55); }
  .dlg-body { padding: 16px 18px; }
  .dlg-actions { display: flex; justify-content: flex-end; gap: 8px; padding: 12px 18px; border-top: 1px solid var(--border); }
  label { display: block; margin: 10px 0 4px; color: var(--muted); font-size: 12px; }
  input[type=text] {
    width: 100%; padding: 8px; border-radius: 6px; border: 1px solid var(--border);
    background: var(--bg); color: var(--text); font-size: 13px;
  }
  input[type=checkbox] { width: 15px; height: 15px; cursor: pointer; accent-color: var(--accent); }
  .radio { margin: 6px 0; display: flex; align-items: center; gap: 8px; font-size: 13px; }
  .muted { color: var(--muted); font-size: 12px; }
</style>
</head>
<body>
<header>
  <h1>ollama-mgr</h1>
  <div class="view-toggle">
    <button type="button" id="btnViewFamily" class="on" title="Group by model family">Family</button>
    <button type="button" id="btnViewTag" title="One row per installed tag">Tag</button>
  </div>
  <button class="primary" id="btnRefresh">Refresh</button>
  <button id="btnCheck">Check updates</button>
  <button id="btnUpgrade">Upgrade…</button>
  <button id="btnOpen">Open listing</button>
  <button id="btnRun">Run</button>
  <button class="danger" id="btnDelete">Delete</button>
  <button id="btnServe">Start serve</button>
</header>
<div id="status">Loading…</div>
<div class="legend" id="familyLegend">
  Size pills: <strong style="color:#bbf7d0">solid</strong> = downloaded ·
  <span style="border:1px dashed #64748b;border-radius:999px;padding:0 6px">outline</span> = available (click to pull) ·
  indigo chips = features (tools, vision, …)
</div>
<div id="selectionBar">
  <span id="selCount">0 selected</span>
  <button id="btnBatchCheck">Check selected</button>
  <button id="btnBatchOpen">Open listings</button>
  <button class="danger" id="btnBatchDelete">Delete selected</button>
  <button id="btnClearSel">Clear</button>
</div>
<main>
  <div id="familyView">
    <table>
      <thead>
        <tr>
          <th class="chk"><input type="checkbox" id="chkAllFamily" title="Select all families"/></th>
          <th>Family</th>
          <th>Features</th>
          <th>Sizes</th>
          <th>Disk</th>
          <th>Tags</th>
        </tr>
      </thead>
      <tbody id="familyBody"></tbody>
    </table>
  </div>
  <div id="tagView" class="hidden">
    <table>
      <thead>
        <tr>
          <th class="chk"><input type="checkbox" id="chkAll" title="Select all"/></th>
          <th class="sortable" data-sort="name" title="Sort by name">Name<span class="ind"></span></th>
          <th class="sortable" data-sort="size" title="Sort by disk size">Size<span class="ind"></span></th>
          <th class="sortable" data-sort="params" title="Sort by parameter count">Params<span class="ind"></span></th>
          <th>Quant</th>
          <th class="sortable" data-sort="released" title="Sort by upstream library Updated date">Released<span class="ind"></span></th>
          <th title="Local last modified / pull time">Downloaded</th>
          <th>Status</th><th>Library</th>
        </tr>
      </thead>
      <tbody id="tbody"></tbody>
    </table>
  </div>
</main>

<dialog id="upgradeDlg">
  <div class="dlg-body">
    <h2 style="margin:0 0 8px;font-size:16px;">Upgrade model</h2>
    <p class="muted" id="upgradeFrom"></p>
    <label for="target">Target model</label>
    <input type="text" id="target" />
    <label>Mode</label>
    <div class="radio"><input type="radio" name="mode" value="skip" id="m0"/><label for="m0" style="margin:0">Don't upgrade (skip)</label></div>
    <div class="radio"><input type="radio" name="mode" value="side-by-side" id="m1" checked/><label for="m1" style="margin:0">Side-by-side (pull new, keep old)</label></div>
    <div class="radio"><input type="radio" name="mode" value="swap" id="m2"/><label for="m2" style="margin:0">Swap (pull new, delete old)</label></div>
    <div class="radio"><input type="radio" name="mode" value="pull" id="m3"/><label for="m3" style="margin:0">Re-pull same tag (digest update)</label></div>
  </div>
  <div class="dlg-actions">
    <button id="btnCancelUpgrade">Cancel</button>
    <button class="primary" id="btnDoUpgrade">Go</button>
  </div>
</dialog>

<script>
let models = [];
let families = [];
let selected = null;
let checked = {};
let checkMap = {};
let jobs = [];
let sortKey = 'name';
let sortDir = 1; // 1 asc, -1 desc
let pollTimer = null;
let viewMode = 'family'; // family | tag
let expandedFamily = {};

const tbody = document.getElementById('tbody');
const familyBody = document.getElementById('familyBody');
const statusEl = document.getElementById('status');
const selBar = document.getElementById('selectionBar');
const selCount = document.getElementById('selCount');
const chkAll = document.getElementById('chkAll');
const chkAllFamily = document.getElementById('chkAllFamily');
const familyView = document.getElementById('familyView');
const tagView = document.getElementById('tagView');
const familyLegend = document.getElementById('familyLegend');

function setStatus(s) { statusEl.textContent = s; }

function activeJobs() {
  return (jobs || []).filter(function(j) { return !j.done; });
}

function jobForFrom(name) {
  for (var i = 0; i < jobs.length; i++) {
    if (jobs[i].pending_delete && jobs[i].from === name) return jobs[i];
  }
  return null;
}

function ensureJobPoll() {
  var active = activeJobs().length > 0;
  if (active && !pollTimer) {
    pollTimer = setInterval(pollJobs, 500);
  }
  if (!active && pollTimer) {
    clearInterval(pollTimer);
    pollTimer = null;
  }
}

async function pollJobs() {
  try {
    var data = await api('/api/jobs');
    var prevActive = activeJobs().length;
    jobs = data.jobs || [];
    var nowActive = activeJobs().length;
    render();
    if (jobs.length) {
      var j = jobs[0];
      var pct = (j.percent >= 0) ? (' ' + Math.round(j.percent) + '%') : '';
      setStatus((j.done ? 'Job done: ' : 'Job: ') + (j.phase || '') + ' — ' + (j.message || '') + pct);
    }
    // When a job finishes, refresh real model list once
    if (prevActive > 0 && nowActive === 0) {
      await refresh(true);
    }
    ensureJobPoll();
  } catch (e) {
    /* keep trying while active */
  }
}

function parseParams(s) {
  if (!s) return -1;
  var m = String(s).trim().match(/^([\d.]+)\s*([bBmM]?)/);
  if (!m) return -1;
  var n = parseFloat(m[1]);
  if (isNaN(n)) return -1;
  var u = (m[2] || 'b').toLowerCase();
  if (u === 'm') n = n / 1000;
  return n;
}

function cmpStr(a, b) {
  a = (a == null || a === '—' || a === '') ? '' : String(a).toLowerCase();
  b = (b == null || b === '—' || b === '') ? '' : String(b).toLowerCase();
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
}

function sortModels() {
  models.sort(function(a, b) {
    var r = 0;
    switch (sortKey) {
      case 'name':
        r = cmpStr(a.name, b.name);
        break;
      case 'size':
        r = (a.size_bytes || 0) - (b.size_bytes || 0);
        break;
      case 'params':
        r = parseParams(a.params) - parseParams(b.params);
        break;
      case 'released':
        // ISO dates sort as strings; empty last when ascending
        var ar = (!a.released || a.released === '—') ? (sortDir > 0 ? '9999' : '') : a.released;
        var br = (!b.released || b.released === '—') ? (sortDir > 0 ? '9999' : '') : b.released;
        r = cmpStr(ar, br);
        break;
      default:
        r = cmpStr(a.name, b.name);
    }
    if (r === 0) r = cmpStr(a.name, b.name);
    return r * sortDir;
  });
}

function updateSortHeaders() {
  document.querySelectorAll('th.sortable').forEach(function(th) {
    var key = th.getAttribute('data-sort');
    var ind = th.querySelector('.ind');
    if (key === sortKey) {
      th.classList.add('active');
      ind.textContent = sortDir > 0 ? '▲' : '▼';
    } else {
      th.classList.remove('active');
      ind.textContent = '';
    }
  });
}

document.querySelectorAll('th.sortable').forEach(function(th) {
  th.addEventListener('click', function() {
    var key = th.getAttribute('data-sort');
    if (sortKey === key) sortDir = -sortDir;
    else { sortKey = key; sortDir = 1; }
    sortModels();
    updateSortHeaders();
    render();
  });
});

async function api(path, opts) {
  const r = await fetch(path, opts);
  const j = await r.json();
  if (!r.ok || j.error) throw new Error(j.error || r.statusText);
  return j;
}

function statusClass(st) {
  if (!st || st === '—' || st === 'OK') return 'badge-ok';
  if (st.indexOf('DELETE PENDING') === 0 || st.indexOf('DOWNLOAD') === 0) return 'badge-dl';
  if (st.indexOf('UPDATE') === 0 || st.indexOf('NOTIONAL') === 0) return 'badge-warn';
  if (st.indexOf('ERROR') === 0 || st.indexOf('DOWN') === 0 || st.indexOf('FAIL') === 0) return 'badge-bad';
  if (st.indexOf('PENDING') >= 0) return 'badge-pend';
  return '';
}

function statusHTML(st, percent) {
  var cls = statusClass(st);
  var showBar = percent != null && percent >= 0 && (
    String(st).indexOf('DOWNLOAD') === 0 ||
    String(st).indexOf('pulling') >= 0 ||
    String(st).indexOf('Pulling') >= 0 ||
    String(st).indexOf('%') >= 0
  );
  if (showBar) {
    var w = Math.max(0, Math.min(100, percent));
    return '<span class="' + cls + '"><span class="prog"><i style="width:' + w + '%"></i></span>' + esc(st) + '</span>';
  }
  return '<span class="' + cls + '">' + esc(st) + '</span>';
}

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, function(c) {
    return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c];
  });
}

function selectedNames() {
  return Object.keys(checked).filter(function(k) { return checked[k]; });
}

function updateSelectionBar() {
  var names = selectedNames();
  selCount.textContent = names.length + ' selected';
  if (names.length > 0) selBar.classList.add('visible');
  else selBar.classList.remove('visible');
  var allOn = models.length > 0 && names.length === models.length;
  chkAll.checked = allOn;
  chkAll.indeterminate = names.length > 0 && !allOn;
}

function displayRows() {
  // Base installed models + synthetic download rows from jobs
  var rows = models.map(function(m) {
    var j = jobForFrom(m.name);
    var st = (checkMap[m.name] && checkMap[m.name].status) || m.status || '—';
    var pending = false;
    var pct = null;
    if (j && j.pending_delete) {
      pending = true;
      st = 'DELETE PENDING → ' + (j.to || '?');
      if (j.phase === 'deleting') st = 'REMOVING (swap complete pull)…';
      if (j.phase === 'error') st = 'SWAP FAILED (kept) — ' + (j.error || j.message || '');
    }
    return {
      kind: 'model',
      name: m.name,
      size: m.size,
      size_bytes: m.size_bytes,
      params: m.params,
      quant: m.quant,
      released: m.released,
      modified: m.modified,
      library: m.library,
      status: st,
      pending_delete: pending,
      percent: pct,
      synthetic: false
    };
  });

  // Add download rows for active (or recent error) jobs
  jobs.forEach(function(j) {
    if (!j.show_download || !j.to) return;
    // If target already in models and job still pulling, still show progress overlay row
    // unless done successfully
    if (j.done && j.phase === 'done') return;
    var already = models.some(function(m) { return m.name === j.to; });
    var st = 'DOWNLOADING ' + (j.to || '');
    if (j.percent >= 0) st += ' ' + Math.round(j.percent) + '%';
    if (j.phase === 'verifying') st = 'VERIFYING ' + j.to;
    if (j.phase === 'deleting') st = 'INSTALLED — removing old…';
    if (j.phase === 'error') st = 'DOWNLOAD/SWAP ERROR — ' + (j.error || j.message || '');
    if (j.message && j.phase === 'pulling') st = j.message;
    rows.push({
      kind: 'job',
      name: j.to + (already ? ' (updating)' : ' (pulling)'),
      size: '—',
      size_bytes: -1,
      params: '',
      quant: '',
      released: '—',
      modified: '—',
      library: '',
      status: st,
      pending_delete: false,
      percent: j.percent,
      synthetic: true,
      job: j
    });
  });
  return rows;
}

function render() {
  if (viewMode === 'family') {
    renderFamily();
    updateSelectionBar();
    return;
  }
  updateSortHeaders();
  tbody.innerHTML = '';
  var rows = displayRows();
  // sort only real models by current sort; keep job rows under related model or at top
  var real = rows.filter(function(r) { return !r.synthetic; });
  var synth = rows.filter(function(r) { return r.synthetic; });
  real.sort(function(a, b) {
    // reuse models sort fields
    var A = a, B = b;
    var r = 0;
    switch (sortKey) {
      case 'size': r = (A.size_bytes || 0) - (B.size_bytes || 0); break;
      case 'params': r = parseParams(A.params) - parseParams(B.params); break;
      case 'released':
        var ar = (!A.released || A.released === '—') ? (sortDir > 0 ? '9999' : '') : A.released;
        var br = (!B.released || B.released === '—') ? (sortDir > 0 ? '9999' : '') : B.released;
        r = cmpStr(ar, br); break;
      default: r = cmpStr(A.name, B.name);
    }
    if (r === 0) r = cmpStr(A.name, B.name);
    return r * sortDir;
  });
  // Put synthetic download rows first so they're obvious
  var ordered = synth.concat(real);

  ordered.forEach(function(m) {
    const tr = document.createElement('tr');
    tr.className = 'data'
      + (selected === m.name || (m.job && selected === m.job.to) ? ' selected' : '')
      + (m.synthetic ? ' job-row' : '')
      + (m.pending_delete ? ' pending-delete' : '');
    tr.onclick = function(e) {
      if (e.target && e.target.type === 'checkbox') return;
      if (m.synthetic) return;
      selected = m.name;
      render();
    };

    const tdChk = document.createElement('td');
    tdChk.className = 'chk';
    if (!m.synthetic) {
      const cb = document.createElement('input');
      cb.type = 'checkbox';
      cb.checked = !!checked[m.name];
      cb.onclick = function(e) {
        e.stopPropagation();
        checked[m.name] = cb.checked;
        if (!cb.checked) delete checked[m.name];
        updateSelectionBar();
      };
      tdChk.appendChild(cb);
    }

    function tdText(t) {
      var td = document.createElement('td');
      td.textContent = t == null ? '' : t;
      return td;
    }
    const tdStatus = document.createElement('td');
    tdStatus.innerHTML = statusHTML(m.status, m.percent);

    const tdLib = document.createElement('td');
    if (m.library) {
      const a = document.createElement('a');
      a.href = m.library; a.target = '_blank'; a.textContent = m.library;
      a.onclick = function(e) { e.stopPropagation(); };
      tdLib.appendChild(a);
    } else if (m.synthetic && m.job && m.job.to) {
      tdLib.textContent = 'pull → ' + m.job.to;
    }

    tr.appendChild(tdChk);
    tr.appendChild(tdText(m.name));
    tr.appendChild(tdText(m.size));
    tr.appendChild(tdText(m.params || ''));
    tr.appendChild(tdText(m.quant || ''));
    tr.appendChild(tdText(m.released || '—'));
    tr.appendChild(tdText(m.modified || '—'));
    tr.appendChild(tdStatus);
    tr.appendChild(tdLib);
    tbody.appendChild(tr);
  });
  updateSelectionBar();
}

function setViewMode(mode) {
  viewMode = mode;
  document.getElementById('btnViewFamily').classList.toggle('on', mode === 'family');
  document.getElementById('btnViewTag').classList.toggle('on', mode === 'tag');
  familyView.classList.toggle('hidden', mode !== 'family');
  tagView.classList.toggle('hidden', mode !== 'tag');
  familyLegend.classList.toggle('hidden', mode !== 'family');
  render();
}

function renderFamily() {
  if (!familyBody) return;
  familyBody.innerHTML = '';
  families.forEach(function(f) {
    var tr = document.createElement('tr');
    tr.className = 'data';

    var tdChk = document.createElement('td');
    tdChk.className = 'chk';
    var cb = document.createElement('input');
    cb.type = 'checkbox';
    var allChecked = (f.installed || []).length > 0 && (f.installed || []).every(function(t) { return checked[t.name]; });
    cb.checked = allChecked;
    cb.onclick = function(e) {
      e.stopPropagation();
      (f.installed || []).forEach(function(t) {
        if (cb.checked) checked[t.name] = true;
        else delete checked[t.name];
      });
      updateSelectionBar();
      renderFamily();
    };
    tdChk.appendChild(cb);

    var tdName = document.createElement('td');
    var title = document.createElement('div');
    title.className = 'family-base';
    var link = document.createElement('a');
    link.href = f.library_url || ('https://ollama.com/library/' + f.base);
    link.target = '_blank';
    link.textContent = f.base;
    link.onclick = function(e) { e.stopPropagation(); };
    title.appendChild(link);
    var meta = document.createElement('div');
    meta.className = 'family-meta';
    meta.textContent = (f.tag_count || 0) + ' installed tag(s)';
    tdName.appendChild(title);
    tdName.appendChild(meta);
    if (expandedFamily[f.base]) {
      var exp = document.createElement('div');
      exp.className = 'expand-tags';
      (f.installed || []).forEach(function(t) {
        var row = document.createElement('div');
        row.textContent = t.name + ' · ' + t.size_human + ' · ' + (t.quant || '');
        row.onclick = function(e) {
          e.stopPropagation();
          selected = t.name;
          setViewMode('tag');
        };
        exp.appendChild(row);
      });
      tdName.appendChild(exp);
    }

    var tdFeat = document.createElement('td');
    var feats = document.createElement('div');
    feats.className = 'pills';
    (f.features || []).forEach(function(fp) {
      if (fp.name === 'completion' && (f.features || []).length > 1) return; // noise
      var p = document.createElement('span');
      p.className = 'pill feat' + (fp.local ? '' : ' lib-only');
      p.textContent = fp.name;
      p.title = (fp.local ? 'local' : '') + (fp.local && fp.lib ? ' + ' : '') + (fp.lib ? 'library' : '');
      feats.appendChild(p);
    });
    if (!feats.childNodes.length) {
      var none = document.createElement('span');
      none.className = 'muted';
      none.textContent = '—';
      feats.appendChild(none);
    }
    tdFeat.appendChild(feats);

    var tdSizes = document.createElement('td');
    var sizes = document.createElement('div');
    sizes.className = 'pills';
    (f.sizes || []).forEach(function(sp) {
      var p = document.createElement('span');
      p.className = 'pill ' + (sp.installed ? 'size-in' : 'size-out');
      p.textContent = sp.size;
      if (sp.installed && sp.disk_human) {
        var sub = document.createElement('span');
        sub.className = 'sub';
        sub.textContent = sp.disk_human + (sp.quant ? ' ' + sp.quant : '');
        p.appendChild(sub);
      }
      if (sp.installed) {
        p.title = 'Installed: ' + (sp.local_tags || []).join(', ');
        p.onclick = function(e) {
          e.stopPropagation();
          if (sp.local_tags && sp.local_tags.length) {
            selected = sp.local_tags[0];
            checked = {};
            sp.local_tags.forEach(function(n) { checked[n] = true; });
            updateSelectionBar();
            if (sp.local_tags.length === 1) setViewMode('tag');
            else {
              expandedFamily[f.base] = true;
              renderFamily();
            }
          }
        };
      } else {
        p.title = 'Not downloaded — click to pull ' + sp.pull_tag;
        p.onclick = function(e) {
          e.stopPropagation();
          pullSize(sp.pull_tag);
        };
      }
      sizes.appendChild(p);
    });
    if (!sizes.childNodes.length) {
      var dash = document.createElement('span');
      dash.className = 'muted';
      dash.textContent = '—';
      sizes.appendChild(dash);
    }
    tdSizes.appendChild(sizes);

    var tdDisk = document.createElement('td');
    tdDisk.textContent = f.disk_human || '—';
    var tdTags = document.createElement('td');
    var btn = document.createElement('button');
    btn.textContent = expandedFamily[f.base] ? 'Hide tags' : 'Show tags';
    btn.onclick = function(e) {
      e.stopPropagation();
      expandedFamily[f.base] = !expandedFamily[f.base];
      renderFamily();
    };
    tdTags.appendChild(btn);

    tr.onclick = function() {
      expandedFamily[f.base] = !expandedFamily[f.base];
      renderFamily();
    };

    tr.appendChild(tdChk);
    tr.appendChild(tdName);
    tr.appendChild(tdFeat);
    tr.appendChild(tdSizes);
    tr.appendChild(tdDisk);
    tr.appendChild(tdTags);
    familyBody.appendChild(tr);
  });

  if (chkAllFamily) {
    var allTags = [];
    families.forEach(function(f) { (f.installed || []).forEach(function(t) { allTags.push(t.name); }); });
    var nOn = allTags.filter(function(n) { return checked[n]; }).length;
    chkAllFamily.checked = allTags.length > 0 && nOn === allTags.length;
    chkAllFamily.indeterminate = nOn > 0 && nOn < allTags.length;
  }
}

async function pullSize(pullTag) {
  if (!pullTag) return;
  if (!confirm('Pull ' + pullTag + '?\n\nThis may be a large download.')) return;
  setStatus('Starting pull ' + pullTag + '…');
  try {
    var res = await api('/api/upgrade', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({from: '', to: pullTag, mode: 'pull'})
    });
    if (res.job) {
      jobs = [res.job].concat(jobs.filter(function(j) { return j.id !== res.job.id; }));
    }
    ensureJobPoll();
    await pollJobs();
  } catch (e) {
    alert(e.message);
    setStatus(e.message);
  }
}

async function refresh(keepJobStatus) {
  if (!keepJobStatus) setStatus('Loading models + family pills…');
  try {
    const st = await api('/api/status');
    const data = await api('/api/list');
    const famData = await api('/api/families');
    const jobData = await api('/api/jobs');
    models = data.models || [];
    families = famData.families || [];
    jobs = jobData.jobs || [];
    // drop checks for removed models
    Object.keys(checked).forEach(function(k) {
      if (!models.some(function(m) { return m.name === k; })) delete checked[k];
    });
    sortModels();
    render();
    ensureJobPoll();
    if (!keepJobStatus || !activeJobs().length) {
      setStatus(st.endpoint + '  |  ' + st.message + '  |  ' +
        (famData.count || 0) + ' families / ' + (data.count || 0) + ' tags  |  ' + (data.total || famData.total || ''));
    }
  } catch (e) {
    setStatus('Error: ' + e.message);
  }
}

async function checkNames(names) {
  var label = names && names.length ? ('selected (' + names.length + ')') : 'all';
  setStatus('Checking updates for ' + label + '…');
  try {
    var opts = undefined;
    if (names && names.length) {
      opts = {
        method: 'POST',
        headers: {'Content-Type': 'application/json'},
        body: JSON.stringify({names: names})
      };
    }
    const data = await api('/api/check', opts);
    if (!names || !names.length) checkMap = {};
    (data.results || []).forEach(function(r) { checkMap[r.model] = r; });
    models = models.map(function(m) {
      var copy = Object.assign({}, m);
      if (checkMap[m.name]) copy.status = checkMap[m.name].status;
      return copy;
    });
    render();
    setStatus('Check complete — ' + data.attention + ' need attention');
  } catch (e) {
    setStatus('Check failed: ' + e.message);
  }
}

function requireSelected() {
  if (!selected) { alert('Select a model first (click a row).'); return null; }
  return selected;
}

function targetsForAction() {
  var names = selectedNames();
  if (names.length) return names;
  if (selected) return [selected];
  return [];
}

chkAll.onchange = function() {
  if (chkAll.checked) {
    models.forEach(function(m) { checked[m.name] = true; });
  } else {
    checked = {};
  }
  render();
};

document.getElementById('btnViewFamily').onclick = function() { setViewMode('family'); };
document.getElementById('btnViewTag').onclick = function() { setViewMode('tag'); };
if (chkAllFamily) {
  chkAllFamily.onchange = function() {
    families.forEach(function(f) {
      (f.installed || []).forEach(function(t) {
        if (chkAllFamily.checked) checked[t.name] = true;
        else delete checked[t.name];
      });
    });
    render();
  };
}
document.getElementById('btnRefresh').onclick = refresh;
document.getElementById('btnCheck').onclick = function() {
  var names = selectedNames();
  checkNames(names.length ? names : null);
};
document.getElementById('btnBatchCheck').onclick = function() {
  var names = selectedNames();
  if (!names.length) return;
  checkNames(names);
};
document.getElementById('btnClearSel').onclick = function() {
  checked = {};
  render();
};
document.getElementById('btnOpen').onclick = async function() {
  var names = targetsForAction();
  if (!names.length) { alert('Select or check a model first.'); return; }
  await api('/api/open', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({names: names})
  });
};
document.getElementById('btnBatchOpen').onclick = async function() {
  var names = selectedNames();
  if (!names.length) return;
  await api('/api/open', {
    method: 'POST',
    headers: {'Content-Type': 'application/json'},
    body: JSON.stringify({names: names})
  });
};
document.getElementById('btnRun').onclick = async function() {
  var n = requireSelected(); if (!n) return;
  setStatus('Starting ollama run ' + n + '…');
  try {
    await api('/api/run', {method:'POST', headers:{'Content-Type':'application/json'}, body: JSON.stringify({name:n})});
    setStatus('Launched ' + n);
  } catch(e) { setStatus(e.message); alert(e.message); }
};
async function deleteNames(names) {
  if (!names.length) return;
  if (!confirm('Permanently delete ' + names.length + ' model(s)?\n\n' + names.join('\n'))) return;
  setStatus('Deleting ' + names.length + ' model(s)…');
  try {
    var res = await api('/api/delete', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({names: names})
    });
    names.forEach(function(n) { delete checked[n]; if (selected === n) selected = null; });
    if (res.failed && res.failed.length) alert('Some deletes failed:\n' + res.failed.join('\n'));
    await refresh();
  } catch(e) { alert(e.message); setStatus(e.message); }
}
document.getElementById('btnDelete').onclick = function() {
  var names = targetsForAction();
  if (!names.length) { alert('Select or check model(s) first.'); return; }
  deleteNames(names);
};
document.getElementById('btnBatchDelete').onclick = function() {
  deleteNames(selectedNames());
};
document.getElementById('btnServe').onclick = async function() {
  try {
    var j = await api('/api/serve', {method:'POST'});
    setStatus(j.message);
    await refresh();
  } catch(e) { alert(e.message); }
};
document.getElementById('btnUpgrade').onclick = function() {
  var n = requireSelected(); if (!n) return;
  var chk = checkMap[n];
  var target = n;
  if (chk && chk.candidates && chk.candidates.length) target = chk.candidates[0].full_name;
  document.getElementById('upgradeFrom').textContent = 'From: ' + n + (chk ? '  (' + chk.status + ')' : '');
  document.getElementById('target').value = target;
  document.getElementById('upgradeDlg').showModal();
};
document.getElementById('btnCancelUpgrade').onclick = function() {
  document.getElementById('upgradeDlg').close();
};
document.getElementById('btnDoUpgrade').onclick = async function() {
  var n = selected;
  var to = document.getElementById('target').value.trim();
  var mode = document.querySelector('input[name=mode]:checked').value;
  if (mode === 'swap') {
    var msg = 'Swap is staged and safe:\n\n'
      + '1) Tag ' + n + ' as DELETE PENDING\n'
      + '2) Pull/verify ' + to + ' (progress row)\n'
      + '3) Only then remove ' + n + '\n\n'
      + 'If pull fails, the old model is kept.\nContinue?';
    if (!confirm(msg)) return;
  }
  document.getElementById('upgradeDlg').close();
  setStatus('Starting ' + mode + '…');
  try {
    var res = await api('/api/upgrade', {
      method:'POST', headers:{'Content-Type':'application/json'},
      body: JSON.stringify({from:n, to:to, mode:mode})
    });
    if (res.job) {
      // optimistic: show job immediately
      jobs = [res.job].concat(jobs.filter(function(j) { return j.id !== res.job.id; }));
      render();
    }
    setStatus(res.message || 'Upgrade job started');
    ensureJobPoll();
    await pollJobs();
  } catch(e) {
    alert(e.message);
    setStatus(e.message);
  }
};

refresh();
</script>
</body>
</html>
`
