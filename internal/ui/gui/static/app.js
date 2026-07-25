/* ollama-mgr GUI client - ASCII-only strings to avoid encoding issues in WebView2 */
let models = [];
let families = [];
let popularItems = [];
let selected = null;
let selectedSet = {};
let selectAnchor = null;
let checkMap = {};
let jobs = [];
let sortKey = 'name';
let sortDir = 1;
let pollTimer = null;
let viewMode = 'family'; // family | tag | popular
let expandedFamily = {};
let tagOrder = [];
let familyTagOrder = [];
let popTop = 10;
let popPage = 0;
let popPageSize = 10;
let popTotal = 0;

const tbody = document.getElementById('tbody');
const familyBody = document.getElementById('familyBody');
const popularBody = document.getElementById('popularBody');
const statusEl = document.getElementById('status');
const selBar = document.getElementById('selectionBar');
const selCount = document.getElementById('selCount');
const familyView = document.getElementById('familyView');
const tagView = document.getElementById('tagView');
const popularView = document.getElementById('popularView');
const familyLegend = document.getElementById('familyLegend');
const familyEmpty = document.getElementById('familyEmpty');
const btnRun = document.getElementById('btnRun');

function setStatus(s) { statusEl.textContent = s; }

function esc(s) {
  return String(s == null ? '' : s).replace(/[&<>"']/g, function(c) {
    return ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'})[c];
  });
}

async function api(path, opts) {
  const r = await fetch(path, opts);
  const j = await r.json();
  if (!r.ok || j.error) throw new Error(j.error || r.statusText);
  return j;
}

function flagChipHTML(og) {
  og = og || {};
  var code = (og.code || '').toLowerCase();
  var title = (og.name || 'Unknown') + (og.org ? ' | ' + og.org : '') + ' (curated origin; not from Ollama API)';
  if (!code || og.unknown) {
    return '<div class="flag-chip unknown" title="' + esc(title) + '"><span class="code">?</span></div>';
  }
  var src = 'https://flagcdn.com/w40/' + encodeURIComponent(code) + '.png';
  return '<div class="flag-chip" title="' + esc(title) + '">' +
    '<img src="' + src + '" alt="' + esc(code.toUpperCase()) + '" width="28" height="21" loading="lazy"/>' +
    '<span class="code">' + esc(code.toUpperCase()) + '</span></div>';
}

function selectedNames() {
  return Object.keys(selectedSet).filter(function(k) { return selectedSet[k]; });
}

function clearSelection() {
  selectedSet = {};
  selected = null;
  selectAnchor = null;
  updateSelectionBar();
  render();
}

function selectOnly(name) {
  selectedSet = {};
  if (name) {
    selectedSet[name] = true;
    selected = name;
    selectAnchor = name;
  } else {
    selected = null;
    selectAnchor = null;
  }
  updateSelectionBar();
}

function toggleSelect(name) {
  if (selectedSet[name]) {
    delete selectedSet[name];
    if (selected === name) {
      var left = selectedNames();
      selected = left.length ? left[left.length - 1] : null;
    }
  } else {
    selectedSet[name] = true;
    selected = name;
  }
  selectAnchor = name;
  updateSelectionBar();
}

function rangeSelect(name, order) {
  if (!selectAnchor || !order || !order.length) {
    selectOnly(name);
    return;
  }
  var a = order.indexOf(selectAnchor);
  var b = order.indexOf(name);
  if (a < 0 || b < 0) {
    selectOnly(name);
    return;
  }
  if (a > b) { var t = a; a = b; b = t; }
  selectedSet = {};
  for (var i = a; i <= b; i++) selectedSet[order[i]] = true;
  selected = name;
  updateSelectionBar();
}

function handleRowSelect(ev, name, order) {
  if (!name) return;
  if (ev.shiftKey) rangeSelect(name, order || tagOrder);
  else if (ev.ctrlKey || ev.metaKey) toggleSelect(name);
  else selectOnly(name);
  render();
  if (selected) setStatus('Selected ' + selectedNames().join(', '));
}

function updateSelectionBar() {
  var names = selectedNames();
  selCount.textContent = names.length === 0 ? 'Nothing selected' :
    (names.length === 1 ? 'Selected: ' + names[0] : names.length + ' selected');
  if (names.length > 0) selBar.classList.add('visible');
  else selBar.classList.remove('visible');
  if (btnRun) btnRun.disabled = names.filter(isInstalledTag).length !== 1 && !(selected && isInstalledTag(selected) && names.length <= 1);
}

function isInstalledTag(name) {
  return (models || []).some(function(m) { return m.name === name; });
}

function requireRunnableModel() {
  var list = selectedNames().filter(isInstalledTag);
  if (list.length === 1) return list[0];
  if (selected && isInstalledTag(selected) && list.length === 0) return selected;
  if (list.length > 1) {
    alert('Multiple models selected. Click one row (without Ctrl) for Run model.');
    return null;
  }
  alert('Select an installed model first:\n\n- Tag view: click a row\n- Family view: solid size pill or Show tags -> click a tag');
  return null;
}

function targetsForAction() {
  var names = selectedNames().filter(isInstalledTag);
  if (names.length) return names;
  if (selected && isInstalledTag(selected)) return [selected];
  return [];
}

function statusClass(st) {
  if (!st || st === '-' || st === 'OK') return 'badge-ok';
  if (String(st).indexOf('DELETE PENDING') === 0 || String(st).indexOf('DOWNLOAD') === 0) return 'badge-dl';
  if (String(st).indexOf('UPDATE') === 0 || String(st).indexOf('NOTIONAL') === 0) return 'badge-warn';
  if (String(st).indexOf('ERROR') === 0 || String(st).indexOf('FAIL') === 0) return 'badge-bad';
  return '';
}

function statusHTML(st, percent) {
  var cls = statusClass(st);
  var showBar = percent != null && percent >= 0 && (String(st).indexOf('DOWNLOAD') === 0 || String(st).indexOf('%') >= 0);
  if (showBar) {
    var w = Math.max(0, Math.min(100, percent));
    return '<span class="' + cls + '"><span class="prog"><i style="width:' + w + '%"></i></span>' + esc(st) + '</span>';
  }
  return '<span class="' + cls + '">' + esc(st) + '</span>';
}

function setViewMode(mode) {
  viewMode = mode;
  document.getElementById('btnViewFamily').classList.toggle('on', mode === 'family');
  document.getElementById('btnViewTag').classList.toggle('on', mode === 'tag');
  document.getElementById('btnViewPopular').classList.toggle('on', mode === 'popular');
  familyView.classList.toggle('hidden', mode !== 'family');
  tagView.classList.toggle('hidden', mode !== 'tag');
  popularView.classList.toggle('hidden', mode !== 'popular');
  familyLegend.classList.toggle('hidden', mode === 'popular');
  document.getElementById('popularBar').classList.toggle('visible', mode === 'popular');
  if (mode === 'popular') loadPopular();
  else render();
}

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
  if (active && !pollTimer) pollTimer = setInterval(pollJobs, 500);
  if (!active && pollTimer) { clearInterval(pollTimer); pollTimer = null; }
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
      setStatus((j.done ? 'Job done: ' : 'Job: ') + (j.phase || '') + ' - ' + (j.message || '') + pct);
    }
    if (prevActive > 0 && nowActive === 0) await refresh(true);
    ensureJobPoll();
  } catch (e) { /* keep polling */ }
}

function parseParams(s) {
  if (!s) return -1;
  var m = String(s).trim().match(/^([\d.]+)\s*([bBmM]?)/);
  if (!m) return -1;
  var n = parseFloat(m[1]);
  if (isNaN(n)) return -1;
  if ((m[2] || 'b').toLowerCase() === 'm') n = n / 1000;
  return n;
}

function cmpStr(a, b) {
  a = (a == null || a === '-' || a === '') ? '' : String(a).toLowerCase();
  b = (b == null || b === '-' || b === '') ? '' : String(b).toLowerCase();
  if (a < b) return -1;
  if (a > b) return 1;
  return 0;
}

function sortModels() {
  models.sort(function(a, b) {
    var r = 0;
    switch (sortKey) {
      case 'size': r = (a.size_bytes || 0) - (b.size_bytes || 0); break;
      case 'params': r = parseParams(a.params) - parseParams(b.params); break;
      case 'released':
        var ar = (!a.released || a.released === '-') ? (sortDir > 0 ? '9999' : '') : a.released;
        var br = (!b.released || b.released === '-') ? (sortDir > 0 ? '9999' : '') : b.released;
        r = cmpStr(ar, br); break;
      default: r = cmpStr(a.name, b.name);
    }
    if (r === 0) r = cmpStr(a.name, b.name);
    return r * sortDir;
  });
}

function updateSortHeaders() {
  document.querySelectorAll('th.sortable').forEach(function(th) {
    var key = th.getAttribute('data-sort');
    var ind = th.querySelector('.ind');
    if (!ind) return;
    if (key === sortKey) {
      th.classList.add('active');
      ind.textContent = sortDir > 0 ? '^' : 'v';
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

function displayRows() {
  var rows = models.map(function(m) {
    var j = jobForFrom(m.name);
    var st = (checkMap[m.name] && checkMap[m.name].status) || m.status || '-';
    var pending = false;
    if (j && j.pending_delete) {
      pending = true;
      st = 'DELETE PENDING -> ' + (j.to || '?');
      if (j.phase === 'deleting') st = 'REMOVING (swap pull ok)...';
      if (j.phase === 'error') st = 'SWAP FAILED (kept) - ' + (j.error || j.message || '');
    }
    return {
      kind: 'model', name: m.name, size: m.size, size_bytes: m.size_bytes,
      params: m.params, quant: m.quant, released: m.released, modified: m.modified,
      library: m.library, status: st, pending_delete: pending, percent: null,
      synthetic: false, origin: m.origin, flag: m.flag
    };
  });
  jobs.forEach(function(j) {
    if (!j.show_download || !j.to) return;
    if (j.done && j.phase === 'done') return;
    var st = 'DOWNLOADING ' + (j.to || '');
    if (j.percent >= 0) st += ' ' + Math.round(j.percent) + '%';
    if (j.phase === 'verifying') st = 'VERIFYING ' + j.to;
    if (j.phase === 'deleting') st = 'INSTALLED - removing old...';
    if (j.phase === 'error') st = 'DOWNLOAD/SWAP ERROR - ' + (j.error || j.message || '');
    if (j.message && j.phase === 'pulling') st = j.message;
    rows.push({
      kind: 'job', name: j.to + ' (pulling)', size: '-', size_bytes: -1,
      params: '', quant: '', released: '-', modified: '-', library: '',
      status: st, pending_delete: false, percent: j.percent, synthetic: true, job: j
    });
  });
  return rows;
}

function render() {
  updateSelectionBar();
  if (viewMode === 'family') { renderFamily(); return; }
  if (viewMode === 'popular') { renderPopular(); return; }
  updateSortHeaders();
  tbody.innerHTML = '';
  var rows = displayRows();
  var real = rows.filter(function(r) { return !r.synthetic; });
  var synth = rows.filter(function(r) { return r.synthetic; });
  real.sort(function(a, b) {
    var r = 0;
    switch (sortKey) {
      case 'size': r = (a.size_bytes || 0) - (b.size_bytes || 0); break;
      case 'params': r = parseParams(a.params) - parseParams(b.params); break;
      case 'released':
        var ar = (!a.released || a.released === '-') ? (sortDir > 0 ? '9999' : '') : a.released;
        var br = (!b.released || b.released === '-') ? (sortDir > 0 ? '9999' : '') : b.released;
        r = cmpStr(ar, br); break;
      default: r = cmpStr(a.name, b.name);
    }
    if (r === 0) r = cmpStr(a.name, b.name);
    return r * sortDir;
  });
  tagOrder = real.map(function(m) { return m.name; });
  synth.concat(real).forEach(function(m) {
    var tr = document.createElement('tr');
    var isSel = !m.synthetic && !!selectedSet[m.name];
    tr.className = 'data' + (isSel ? ' selected' : '') + (m.synthetic ? ' job-row' : '') + (m.pending_delete ? ' pending-delete' : '');
    tr.onclick = function(e) {
      if (m.synthetic) return;
      if (e.target && (e.target.tagName === 'A' || (e.target.closest && e.target.closest('a')))) return;
      handleRowSelect(e, m.name, tagOrder);
    };
    function tdText(t) {
      var td = document.createElement('td');
      td.textContent = t == null ? '' : t;
      return td;
    }
    var tdStatus = document.createElement('td');
    tdStatus.innerHTML = statusHTML(m.status, m.percent);
    var tdLib = document.createElement('td');
    if (m.library) {
      var a = document.createElement('a');
      a.href = m.library; a.target = '_blank'; a.textContent = m.library;
      a.onclick = function(e) { e.stopPropagation(); };
      tdLib.appendChild(a);
    } else if (m.synthetic && m.job && m.job.to) {
      tdLib.textContent = 'pull -> ' + m.job.to;
    }
    var tdFlag = document.createElement('td');
    tdFlag.className = 'flag-cell';
    tdFlag.innerHTML = flagChipHTML(m.origin || {});
    tr.appendChild(tdFlag);
    tr.appendChild(tdText(m.name));
    tr.appendChild(tdText(m.size));
    tr.appendChild(tdText(m.params || ''));
    tr.appendChild(tdText(m.quant || ''));
    tr.appendChild(tdText(m.released || '-'));
    tr.appendChild(tdText(m.modified || '-'));
    tr.appendChild(tdStatus);
    tr.appendChild(tdLib);
    tbody.appendChild(tr);
  });
}

function renderFamily() {
  familyBody.innerHTML = '';
  familyTagOrder = [];
  families.forEach(function(f) {
    (f.installed || []).forEach(function(t) { familyTagOrder.push(t.name); });
  });
  if (familyEmpty) familyEmpty.classList.toggle('hidden', families.length > 0);
  families.forEach(function(f) {
    var tr = document.createElement('tr');
    var famTags = (f.installed || []).map(function(t) { return t.name; });
    var famSelected = famTags.some(function(n) { return selectedSet[n]; });
    tr.className = 'data' + (famSelected ? ' selected' : '');

    var tdFlag = document.createElement('td');
    tdFlag.className = 'flag-cell';
    tdFlag.innerHTML = flagChipHTML(f.origin || {});

    var tdName = document.createElement('td');
    var title = document.createElement('div');
    title.className = 'family-base';
    var link = document.createElement('a');
    link.href = f.library_url || ('https://ollama.com/library/' + f.base);
    link.target = '_blank';
    link.textContent = f.base;
    link.onclick = function(e) { e.stopPropagation(); };
    title.appendChild(link);
    if (f.fetched && !f.on_disk) {
      var badge = document.createElement('span');
      badge.className = 'pill fetched-badge';
      badge.textContent = 'fetched';
      badge.style.marginLeft = '8px';
      title.appendChild(badge);
      var rm = document.createElement('button');
      rm.textContent = 'x';
      rm.title = 'Remove from board';
      rm.style.marginLeft = '6px';
      rm.style.padding = '0 8px';
      rm.onclick = function(e) { e.stopPropagation(); removeFetched(f.base); };
      title.appendChild(rm);
    }
    var meta = document.createElement('div');
    meta.className = 'family-meta';
    meta.textContent = f.on_disk ? ((f.tag_count || 0) + ' installed tag(s)') : 'not on disk - click a size pill to pull';
    tdName.appendChild(title);
    tdName.appendChild(meta);
    if (expandedFamily[f.base]) {
      var exp = document.createElement('div');
      exp.className = 'expand-tags';
      (f.installed || []).forEach(function(t) {
        var row = document.createElement('div');
        row.textContent = t.name + ' | ' + t.size_human + ' | ' + (t.quant || '');
        if (selectedSet[t.name]) row.style.color = '#93c5fd';
        row.onclick = function(e) { e.stopPropagation(); handleRowSelect(e, t.name, familyTagOrder); };
        exp.appendChild(row);
      });
      tdName.appendChild(exp);
    }

    var tdFeat = document.createElement('td');
    var feats = document.createElement('div');
    feats.className = 'pills';
    (f.features || []).forEach(function(fp) {
      if (fp.name === 'completion' && (f.features || []).length > 1) return;
      var p = document.createElement('span');
      p.className = 'pill feat' + (fp.local ? '' : ' lib-only');
      p.textContent = fp.name;
      p.title = (fp.local ? 'local' : '') + (fp.local && fp.lib ? ' + ' : '') + (fp.lib ? 'library' : '');
      feats.appendChild(p);
    });
    if (!feats.childNodes.length) {
      var none = document.createElement('span');
      none.className = 'muted';
      none.textContent = '-';
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
        p.title = 'Installed: ' + (sp.local_tags || []).join(', ') + ' - click to select for Run model';
        p.onclick = function(e) {
          e.stopPropagation();
          if (sp.local_tags && sp.local_tags.length) {
            handleRowSelect(e, sp.local_tags[0], familyTagOrder);
            if (sp.local_tags.length > 1) expandedFamily[f.base] = true;
            setStatus('Selected ' + selected + ' - Run model opens a chat console');
          }
        };
      } else {
        p.title = 'Not downloaded - click to pull ' + sp.pull_tag;
        p.onclick = function(e) { e.stopPropagation(); pullSize(sp.pull_tag); };
      }
      sizes.appendChild(p);
    });
    if (!sizes.childNodes.length) {
      var dash = document.createElement('span');
      dash.className = 'muted';
      dash.textContent = '-';
      sizes.appendChild(dash);
    }
    tdSizes.appendChild(sizes);

    var tdDisk = document.createElement('td');
    tdDisk.textContent = f.disk_human || '-';
    var tdTags = document.createElement('td');
    var btn = document.createElement('button');
    btn.textContent = expandedFamily[f.base] ? 'Hide tags' : 'Show tags';
    btn.onclick = function(e) {
      e.stopPropagation();
      expandedFamily[f.base] = !expandedFamily[f.base];
      renderFamily();
    };
    tdTags.appendChild(btn);

    tr.onclick = function(e) {
      if (e.target && (e.target.tagName === 'BUTTON' || e.target.tagName === 'A' || (e.target.closest && e.target.closest('button,a,.pill')))) return;
      expandedFamily[f.base] = !expandedFamily[f.base];
      if (famTags.length === 1) { handleRowSelect(e, famTags[0], familyTagOrder); return; }
      renderFamily();
    };

    tr.appendChild(tdFlag);
    tr.appendChild(tdName);
    tr.appendChild(tdFeat);
    tr.appendChild(tdSizes);
    tr.appendChild(tdDisk);
    tr.appendChild(tdTags);
    familyBody.appendChild(tr);
  });
  updateSelectionBar();
}

function renderPopular() {
  popularBody.innerHTML = '';
  var pages = Math.max(1, Math.ceil(popTotal / popPageSize));
  document.getElementById('popPageLabel').textContent =
    'page ' + (popPage + 1) + ' / ' + pages + ' (top ' + popTop + ', ' + popTotal + ' shown)';
  document.getElementById('btnPopPrev').disabled = popPage <= 0;
  document.getElementById('btnPopNext').disabled = (popPage + 1) * popPageSize >= popTotal;

  popularItems.forEach(function(it) {
    var tr = document.createElement('tr');
    tr.className = 'data';
    function td(t) { var d = document.createElement('td'); d.textContent = t == null ? '' : t; return d; }
    var tdRank = document.createElement('td');
    tdRank.className = 'rank';
    tdRank.textContent = it.rank;
    var tdFlag = document.createElement('td');
    tdFlag.className = 'flag-cell';
    tdFlag.innerHTML = flagChipHTML(it.origin || {});
    var tdName = document.createElement('td');
    var a = document.createElement('a');
    a.href = it.url || ('https://ollama.com/library/' + it.name);
    a.target = '_blank';
    a.textContent = it.name;
    tdName.appendChild(a);
    var tdPulls = td(it.pulls || '-');
    var tdFeat = document.createElement('td');
    var feats = document.createElement('div');
    feats.className = 'pills';
    (it.features || []).forEach(function(name) {
      if (name === 'completion') return;
      var p = document.createElement('span');
      p.className = 'pill feat lib-only';
      p.textContent = name;
      p.title = 'Library feature (why you might want this model)';
      feats.appendChild(p);
    });
    if (!feats.childNodes.length) {
      var n = document.createElement('span');
      n.className = 'muted';
      n.textContent = '-';
      feats.appendChild(n);
    }
    tdFeat.appendChild(feats);
    var tdSizes = document.createElement('td');
    var sizes = document.createElement('div');
    sizes.className = 'pills';
    (it.sizes || []).forEach(function(sz) {
      var p = document.createElement('span');
      var installed = it.installed_sizes && it.installed_sizes[sz];
      p.className = 'pill ' + (installed ? 'size-in' : 'size-out');
      p.textContent = sz;
      if (!installed) {
        p.title = 'Click to pull ' + it.name + ':' + sz;
        p.onclick = function(e) { e.stopPropagation(); pullSize(it.name + ':' + sz); };
      } else {
        p.title = 'Already on disk';
      }
      sizes.appendChild(p);
    });
    if (!sizes.childNodes.length) {
      var d = document.createElement('span');
      d.className = 'muted';
      d.textContent = '-';
      sizes.appendChild(d);
    }
    tdSizes.appendChild(sizes);
    var tdAct = document.createElement('td');
    var btn = document.createElement('button');
    btn.textContent = 'Fetch';
    btn.title = 'Add to Family board with outline sizes';
    btn.onclick = function(e) { e.stopPropagation(); fetchFamily(it.name); };
    tdAct.appendChild(btn);
    tr.appendChild(tdRank);
    tr.appendChild(tdFlag);
    tr.appendChild(tdName);
    tr.appendChild(tdPulls);
    tr.appendChild(tdFeat);
    tr.appendChild(tdSizes);
    tr.appendChild(tdAct);
    popularBody.appendChild(tr);
  });
}

async function loadPopular() {
  setStatus('Loading popular models (top ' + popTop + ')...');
  try {
    var data = await api('/api/popular?top=' + popTop + '&page=' + popPage + '&page_size=' + popPageSize);
    popularItems = data.items || [];
    popTotal = data.total || 0;
    popPage = data.page || 0;
    renderPopular();
    setStatus('Popular: top ' + popTop + ' by ollama.com library order (features = why you might want them)');
  } catch (e) {
    setStatus('Popular failed: ' + e.message);
  }
}

async function pullSize(pullTag) {
  if (!pullTag) return;
  if (!confirm('Pull ' + pullTag + '?\n\nThis may be a large download.')) return;
  setStatus('Starting pull ' + pullTag + '...');
  try {
    var res = await api('/api/upgrade', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({from: '', to: pullTag, mode: 'pull'})
    });
    if (res.job) jobs = [res.job].concat(jobs.filter(function(j) { return j.id !== res.job.id; }));
    ensureJobPoll();
    await pollJobs();
  } catch (e) {
    alert(e.message);
    setStatus(e.message);
  }
}

async function refresh(keepJobStatus) {
  if (!keepJobStatus) setStatus('Loading models...');
  try {
    const st = await api('/api/status');
    if (!st.up) {
      models = [];
      families = [];
      render();
      setStatus((st.endpoint || '') + ' | DOWN - ' + (st.message || 'Ollama not reachable') +
        ' | Start server or fix OLLAMA_HOST (0.0.0.0 is rewritten to 127.0.0.1)');
      return;
    }
    const data = await api('/api/list');
    const famData = await api('/api/families');
    const jobData = await api('/api/jobs');
    models = data.models || [];
    families = famData.families || [];
    jobs = jobData.jobs || [];
    Object.keys(selectedSet).forEach(function(k) {
      if (!models.some(function(m) { return m.name === k; })) delete selectedSet[k];
    });
    if (selected && !isInstalledTag(selected)) selected = selectedNames()[0] || null;
    sortModels();
    if (viewMode === 'popular') await loadPopular();
    else render();
    ensureJobPoll();
    if (!keepJobStatus || !activeJobs().length) {
      setStatus(st.endpoint + ' | ' + st.message + ' | ' +
        (famData.count || 0) + ' families / ' + (data.count || 0) + ' tags | ' + (data.total || ''));
    }
  } catch (e) {
    models = [];
    families = [];
    render();
    setStatus('Error loading models: ' + e.message);
  }
}

async function checkNames(names) {
  var label = names && names.length ? ('selected (' + names.length + ')') : 'all';
  setStatus('Checking updates for ' + label + '...');
  try {
    var opts = undefined;
    if (names && names.length) {
      opts = { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({names: names}) };
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
    setStatus('Check complete - ' + data.attention + ' need attention');
  } catch (e) {
    setStatus('Check failed: ' + e.message);
  }
}

var addBar = document.getElementById('addBar');
var addQuery = document.getElementById('addQuery');
var addHits = document.getElementById('addHits');
var addSearchTimer = null;
var lastExact = '';

function showAddBar(show) {
  if (show) {
    addBar.classList.add('visible');
    setViewMode('family');
    addQuery.value = '';
    addHits.innerHTML = '';
    lastExact = '';
    addQuery.focus();
  } else {
    addBar.classList.remove('visible');
    addHits.innerHTML = '';
  }
}

async function runLibrarySearch() {
  var q = addQuery.value.trim();
  if (!q) { addHits.innerHTML = ''; lastExact = ''; return; }
  try {
    var data = await api('/api/library/search?q=' + encodeURIComponent(q));
    lastExact = data.exact || '';
    addHits.innerHTML = '';
    (data.results || []).slice(0, 12).forEach(function(h) {
      var b = document.createElement('button');
      b.type = 'button';
      b.className = 'hit' + (data.exact && h.name.toLowerCase() === data.exact.toLowerCase() ? ' exact' : '');
      b.textContent = h.name;
      b.onclick = function() { fetchFamily(h.name); };
      addHits.appendChild(b);
    });
    if (!(data.results || []).length) addHits.innerHTML = '<span class="muted">No library hits</span>';
  } catch (e) {
    addHits.innerHTML = '<span class="muted">' + esc(e.message) + '</span>';
  }
}

async function fetchFamily(name) {
  if (!name) name = lastExact || addQuery.value.trim();
  if (!name) { alert('Enter a library model name'); return; }
  setStatus('Fetching family ' + name + '...');
  try {
    var res = await api('/api/families/fetch', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({name: name})
    });
    showAddBar(false);
    setViewMode('family');
    await refresh();
    expandedFamily[res.added || name] = true;
    renderFamily();
    setStatus('Fetched ' + (res.added || name) + (res.already_on_disk ? ' (already on disk)' : ' - sizes outlined; click to pull'));
  } catch (e) {
    alert(e.message);
    setStatus(e.message);
  }
}

async function removeFetched(name) {
  try {
    await api('/api/families/fetch?name=' + encodeURIComponent(name), {method: 'DELETE'});
    await refresh();
  } catch (e) { alert(e.message); }
}

document.getElementById('btnViewFamily').onclick = function() { setViewMode('family'); };
document.getElementById('btnViewTag').onclick = function() { setViewMode('tag'); };
document.getElementById('btnViewPopular').onclick = function() { setViewMode('popular'); };
document.getElementById('btnClearSel').onclick = clearSelection;
document.getElementById('btnRefresh').onclick = refresh;
document.getElementById('btnCheck').onclick = function() {
  var names = selectedNames();
  checkNames(names.length ? names : null);
};
document.getElementById('btnBatchCheck').onclick = function() {
  var names = selectedNames();
  if (names.length) checkNames(names);
};
document.getElementById('btnOpen').onclick = async function() {
  var names = targetsForAction();
  if (!names.length) { alert('Select a model first.'); return; }
  await api('/api/open', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({names: names}) });
};
document.getElementById('btnBatchOpen').onclick = async function() {
  var names = selectedNames();
  if (!names.length) return;
  await api('/api/open', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({names: names}) });
};
document.getElementById('btnRun').onclick = async function() {
  var n = requireRunnableModel(); if (!n) return;
  setStatus('Opening console: ollama run ' + n + '...');
  try {
    await api('/api/run', {method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({name: n})});
    setStatus('Opened new console for ' + n);
  } catch (e) { alert(e.message); setStatus(e.message); }
};
async function deleteNames(names) {
  if (!names.length) return;
  if (!confirm('Permanently delete ' + names.length + ' model(s)?\n\n' + names.join('\n'))) return;
  setStatus('Deleting...');
  try {
    var res = await api('/api/delete', { method: 'POST', headers: {'Content-Type': 'application/json'}, body: JSON.stringify({names: names}) });
    names.forEach(function(n) { delete selectedSet[n]; if (selected === n) selected = null; });
    if (res.failed && res.failed.length) alert('Some deletes failed:\n' + res.failed.join('\n'));
    await refresh();
  } catch (e) { alert(e.message); }
}
document.getElementById('btnDelete').onclick = function() {
  var names = targetsForAction();
  if (!names.length) { alert('Select model(s) first.'); return; }
  deleteNames(names);
};
document.getElementById('btnBatchDelete').onclick = function() { deleteNames(selectedNames()); };
document.getElementById('btnServe').onclick = async function() {
  setStatus('Checking / starting Ollama server...');
  try {
    var j = await api('/api/serve', {method: 'POST'});
    var msg = j.message || '';
    if (msg === 'already up') msg = 'Ollama server is already running';
    else if (msg === 'started in background' || msg === 'serve started') msg = 'Ollama server started in background (no console)';
    setStatus(msg);
    await refresh(true);
  } catch (e) { alert(e.message); }
};
document.getElementById('btnUpgrade').onclick = function() {
  var n = requireRunnableModel(); if (!n) return;
  var chk = checkMap[n];
  var target = n;
  if (chk && chk.candidates && chk.candidates.length) target = chk.candidates[0].full_name;
  document.getElementById('upgradeFrom').textContent = 'From: ' + n + (chk ? ' (' + chk.status + ')' : '');
  document.getElementById('target').value = target;
  document.getElementById('upgradeDlg').showModal();
};
document.getElementById('btnCancelUpgrade').onclick = function() { document.getElementById('upgradeDlg').close(); };
document.getElementById('btnDoUpgrade').onclick = async function() {
  var n = selected;
  var to = document.getElementById('target').value.trim();
  var mode = document.querySelector('input[name=mode]:checked').value;
  if (mode === 'swap') {
    if (!confirm('Swap is staged and safe:\n1) Tag old as DELETE PENDING\n2) Pull/verify ' + to + '\n3) Only then remove old\nContinue?')) return;
  }
  document.getElementById('upgradeDlg').close();
  setStatus('Starting ' + mode + '...');
  try {
    var res = await api('/api/upgrade', {
      method: 'POST', headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({from: n, to: to, mode: mode})
    });
    if (res.job) jobs = [res.job].concat(jobs.filter(function(j) { return j.id !== res.job.id; }));
    setStatus(res.message || 'Upgrade job started');
    ensureJobPoll();
    await pollJobs();
  } catch (e) { alert(e.message); setStatus(e.message); }
};

document.getElementById('btnAddFamily').onclick = function() { showAddBar(true); };
document.getElementById('btnAddCancel').onclick = function() { showAddBar(false); };
document.getElementById('btnAddDo').onclick = function() {
  var name = lastExact || addQuery.value.trim();
  if (!lastExact && name) {
    runLibrarySearch().then(function() {
      if (lastExact) fetchFamily(lastExact);
      else alert('No exact library match. Pick a hit from the list.');
    });
    return;
  }
  fetchFamily(name);
};
addQuery.addEventListener('input', function() {
  clearTimeout(addSearchTimer);
  addSearchTimer = setTimeout(runLibrarySearch, 280);
});
addQuery.addEventListener('keydown', function(e) {
  if (e.key === 'Enter') { e.preventDefault(); document.getElementById('btnAddDo').click(); }
  if (e.key === 'Escape') showAddBar(false);
});

document.querySelectorAll('.pop-top').forEach(function(btn) {
  btn.onclick = function() {
    document.querySelectorAll('.pop-top').forEach(function(b) { b.classList.remove('on'); });
    btn.classList.add('on');
    popTop = parseInt(btn.getAttribute('data-top'), 10) || 10;
    popPage = 0;
    loadPopular();
  };
});
document.getElementById('btnPopPrev').onclick = function() {
  if (popPage > 0) { popPage--; loadPopular(); }
};
document.getElementById('btnPopNext').onclick = function() {
  if ((popPage + 1) * popPageSize < popTotal) { popPage++; loadPopular(); }
};

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') { clearSelection(); setStatus('Selection cleared'); }
  if ((e.ctrlKey || e.metaKey) && e.key === 'a' && viewMode === 'tag') {
    e.preventDefault();
    selectedSet = {};
    (tagOrder.length ? tagOrder : models.map(function(m) { return m.name; })).forEach(function(n) {
      selectedSet[n] = true;
    });
    selected = tagOrder[0] || (models[0] && models[0].name) || null;
    selectAnchor = selected;
    updateSelectionBar();
    render();
  }
});

// Optional deep-link for screenshots/docs: ?view=family|tag|popular
(function initViewFromQuery() {
  try {
    var v = new URLSearchParams(location.search).get('view');
    if (v === 'tag' || v === 'popular' || v === 'family') {
      setTimeout(function() { setViewMode(v); }, 0);
    }
  } catch (e) { /* ignore */ }
})();

refresh();
