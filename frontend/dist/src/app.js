// 微清 WeClean 前端逻辑。
//
// 所有文件系统操作都经 Wails 生成的绑定调用 Go 服务，
// 前端只负责渲染与交互。

import * as api from './rpc.js';


// 全局错误捕获：模块加载或运行时异常必须可见，否则界面只是「点了没反应」。
window.addEventListener('error', (ev) => {
  console.error('[wfk] 未捕获错误:', ev.message, ev.filename, ev.lineno);
  const t = document.getElementById('toast');
  if (t) {
    t.textContent = '加载失败：' + ev.message;
    t.hidden = false;
    t.classList.add('show');
  }
});
window.addEventListener('unhandledrejection', (ev) => {
  console.error('[wfk] 未处理的 Promise 拒绝:', ev.reason);
});


// ---------- 状态 ----------

const state = {
  scan: null,           // 体检结果
  view: 'overview',     // 当前视图
  typeFilter: '全部',    // 类型筛选
  sizeFilter: 0,        // 大小门槛（字节）
  grouped: true,        // 是否按月份分组
  selected: new Set(),  // 已勾选的绝对路径
  selectedMeta: new Map(), // path -> { size, month, category, name }
  expanded: new Set(),  // 用户主动展开的月份
  expandedTouched: false, // 用户是否操作过展开状态
  libPath: '',          // 当前账号库
};

/** 单次从后端拉取的条目数上限。卡片过多会拖慢渲染。 */
const MAX_CARDS = 60;

// ---------- 工具 ----------

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s).replace(/[&<>"']/g, (c) =>
  ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

/** 调用后端接口。 */
async function call(method, ...args) {
  switch (method) {
    case 'Scan': return await api.scan(args[0]);
    case 'Clean': return await api.clean(args[0].paths, args[0].includeCache);
    case 'Undo': return await api.undo();
    case 'CheckRunning': return await api.checkRunning(args[0]);
    case 'PreviewImage': return await api.preview(args[0]);
    case 'OpenFile': return await api.openFile(args[0]);
    case 'RevealFile': return await api.reveal(args[0]);
    case 'Libraries': return await api.libraries();
    case 'Entries': return await api.entries(args[0]);
    case 'Months': return await api.months();
    case 'TrashMethod': return await api.trashMethod(args[0]);
    default: throw new Error(`未知接口 ${method}`);
  }
}

let toastTimer = null;
function toast(msg) {
  const el = $('toast');
  el.textContent = msg;
  el.hidden = false;
  requestAnimationFrame(() => el.classList.add('show'));
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => {
    el.classList.remove('show');
    setTimeout(() => { el.hidden = true; }, 240);
  }, 2600);
}

/** 分类对应的图标与渐变底色。 */
const CAT_META = {
  '视频': { icon: '🎬', grad: 'linear-gradient(135deg,#e8f5ee,#d6ece1)' },
  '文档': { icon: '📄', grad: 'linear-gradient(135deg,#eef1f7,#dfe5ef)' },
  '压缩包': { icon: '📦', grad: 'linear-gradient(135deg,#fdf0e6,#f7e2cf)' },
  '其他': { icon: '📎', grad: 'linear-gradient(135deg,#f4f4f6,#e8e8ec)' },
  '图片缓存': { icon: '🖼', grad: 'linear-gradient(135deg,#f0eefb,#e2ddf6)' },
};

// ---------- 视图切换 ----------

function switchView(view) {
  state.view = view;
  document.querySelectorAll('.view').forEach((v) => v.classList.remove('show'));
  $('view-' + view)?.classList.add('show');
  document.querySelectorAll('.nav-item').forEach((n) => {
    n.classList.toggle('active', n.dataset.view === view);
  });

  const titles = {
    overview: ['数据体检', '只读分析 · 不改动任何文件'],
    months: ['按月份清理', '按月份筛选，勾选后移入废纸篓'],
    orphans: ['孤儿与缓存', '零风险清理项'],
    safety: ['安全承诺', '这个工具做什么、不做什么'],
  };
  const [t, s] = titles[view] || ['', ''];
  $('pageTitle').textContent = t;
  $('pageSub').textContent = s;
  $('btnRescan').hidden = view !== 'months' && view !== 'orphans';

  if (view === 'months') renderMonths();
  if (view === 'orphans') renderOrphans();
  updateActionBar();
}

// ---------- 体检 ----------

async function doScan(silent = false) {
  // 两个入口（概览页大按钮、月份页重扫按钮）共用一套逻辑，
  // 都要置忙并恢复，否则会出现「按钮一直转圈」的错觉。
  const btn = $('btnScan');
  const rescan = $('btnRescan');
  btn.disabled = true;
  btn.textContent = '正在体检…';
  rescan.disabled = true;
  rescan.textContent = '正在体检…';
  $('scanHint').textContent = '正在扫描消息区，请稍候';

  try {
    const result = await call('Scan', state.libPath || '');
    state.scan = result;
    state.libPath = result.library.path;
    state.selected.clear();
    state.selectedMeta.clear();
    renderOverview();
    updateSidebar();
    updateBanner(result.weChatRunning);
    // 停留在当前视图：首次是概览页看结论，重新体检时保持原位置。
    switchView(state.view);
    if (!silent) {
      toast(`体检完成：${result.totalText}，用时 ${result.durationMs}ms`);
    }
  } catch (err) {
    toast('体检失败：' + (err?.message || err));
    $('scanHint').textContent = '体检失败，请重试';
  } finally {
    btn.disabled = false;
    btn.textContent = '✨ 开始体检';
    rescan.disabled = false;
    rescan.textContent = '重新体检';
    $('scanHint').textContent = '只读分析 · 不改动任何文件';
  }
}

function renderOverview() {
  const s = state.scan;
  if (!s) return;

  $('ringValue').textContent = s.totalText.replace(/ ?[A-Z]+$/, '');
  $('ringUnit').textContent = (s.totalText.match(/[A-Z]+$/) || [''])[0] || '实占';
  $('ringFg').style.strokeDashoffset = '0';

  $('heroTitle').textContent = `微信占用 ${s.totalText}`;
  $('heroDesc').innerHTML =
    `${s.entryCount.toLocaleString()} 个文件 · 扫描用时 ${s.durationMs}ms` +
    (s.hardlinkOvercount > 0
      ? `<br><span class="muted">硬链接虚增 ${fmt(s.hardlinkOvercount)}（同一文件多个名字，删除不释放空间）</span>`
      : '');

  // 分类卡片
  const max = Math.max(...s.categories.map((c) => c.bytes), 1);
  $('catCards').innerHTML = s.categories.map((c) => {
    const meta = CAT_META[c.name] || { icon: '📎' };
    const pct = Math.round((c.bytes / max) * 100);
    return `<div class="card">
      <div class="card-head">
        <span class="card-ico">${meta.icon}</span>
        <span class="card-name">${esc(c.name)}</span>
      </div>
      <div class="card-num">${esc(c.bytesText)}</div>
      <div class="card-sub">${c.count.toLocaleString()} 个文件</div>
      <div class="card-bar"><i style="width:${pct}%"></i></div>
    </div>`;
  }).join('');

  // 入口瓦片
  $('entryTiles').hidden = false;
  $('tileMonths').textContent = `${s.months.length} 个月份 · ${s.totalText}`;
  $('tileOrphans').textContent = s.orphans.count > 0
    ? `${s.orphans.count} 个 · ${s.orphans.bytesText}`
    : '没有孤儿文件';
}

function updateSidebar() {
  const s = state.scan;
  if (!s) return;
  $('wxName').textContent = s.library.wxid;
  $('wxPath').textContent = s.totalText + ' · ' + s.entryCount.toLocaleString() + ' 个文件';
  $('navCntM').textContent = s.months.length;
  $('navCntO').textContent = s.orphans.count;
}

function updateBanner(running) {
  const banner = $('banner');
  banner.hidden = !running;
  if (running) {
    $('bannerText').textContent =
      '检测到微信正在运行：清理后微信的「存储空间管理」统计可能对不上，文件本身不受影响';
  }
}


// ---------- 缩略图加载 ----------
//
// 卡片可能有几百个，每个缩略图都是一次后端调用 + base64 传输。
// 因此严格限制并发，并缓存结果，避免请求堆积把界面拖死。

const thumbCache = new Map();      // path -> dataURL | null
const thumbQueue = [];             // 待加载任务
const thumbPending = new Set();    // 正在加载的 path
const pendingCards = new Map();    // path -> 等待同一张图的卡片
const MAX_CONCURRENT = 4;
let thumbActive = 0;

/** 切换一张卡片的选中状态，并记录元信息。 */
function toggleSelect(card) {
  const path = card.dataset.path;
  if (state.selected.has(path)) {
    state.selected.delete(path);
    state.selectedMeta.delete(path);
    card.classList.remove('sel');
  } else {
    state.selected.add(path);
    state.selectedMeta.set(path, {
      size: Number(card.dataset.size || 0),
      month: card.dataset.month || '',
      category: card.dataset.category || '',
      name: card.dataset.name || '',
    });
    card.classList.add('sel');
  }
  // 同步所在月份桶的「清理」按钮：单张勾选也要让它变可点。
  syncMonthCleanButtons();
  updateActionBar();
}

/** 按各月份桶内是否有选中项，刷新它们的「清理」按钮状态。 */
function syncMonthCleanButtons() {
  document.querySelectorAll('[data-clean]').forEach((btn) => {
    const month = btn.dataset.clean;
    const body = document.querySelector(`[data-body="${CSS.escape(month)}"]`);
    if (!body) {
      btn.disabled = true;
      return;
    }
    const cards = [...body.querySelectorAll('.fc[data-path]')];
    btn.disabled = !cards.some((c) => state.selected.has(c.dataset.path));
  });
}

/** 生成文件网格 HTML。 */
function fileGrid(entries) {
  if (!entries || entries.length === 0) {
    return '<div class="empty">没有符合条件的文件</div>';
  }

  const shown = entries.slice(0, MAX_CARDS);
  const more = entries.length - shown.length;
  const moreHint = more > 0
    ? `<div class="empty" style="padding:16px">还有 ${more} 个未显示，用筛选缩小范围</div>`
    : '';

  const cards = shown.map((e) => {
    const meta = CAT_META[e.category] || { icon: '📎', grad: '' };
    const sel = state.selected.has(e.path);
    const orphan = e.isOrphan ? '<span class="badge-orphan">孤儿</span>' : '';
    return `<div class="fc${sel ? ' sel' : ''}${e.isOrphan ? ' orphan' : ''}"
        data-path="${esc(e.path)}"
        data-previewable="${e.previewable ? '1' : '0'}"
        data-name="${esc(e.name)}"
        data-size="${e.size}"
        data-month="${esc(e.month)}"
        data-category="${esc(e.category)}">
      <div class="thumb" style="background:${meta.grad}">
        ${orphan}
        <span data-icon="${esc(e.path)}">${meta.icon}</span>
        <span class="fc-check"></span>
      </div>
      <div class="fc-body">
        <div class="fc-name" title="${esc(e.relPath)}">${esc(e.name)}</div>
        <div class="fc-meta">
          <span>${esc(e.sizeText)}</span>
          <span>${e.daysAgo > 0 ? e.daysAgo + ' 天前' : '今天'}</span>
        </div>
      </div>
    </div>`;
  }).join('');

  return `<div class="file-grid">${cards}</div>${moreHint}`;
}

/** 绑定卡片点击与缩略图加载。 */
function bindFileCards(root) {
  const scope = root || document;
  scope.querySelectorAll('.fc[data-path]').forEach((card) => {
    card.addEventListener('click', () => toggleSelect(card));

    if (card.dataset.previewable === '1') {
      enqueueThumb(card, { path: card.dataset.path, name: card.dataset.name || '' });
    }
  });
}

/** 把卡片加入缩略图队列。 */
function enqueueThumb(card, entry) {
  const path = entry.path;

  if (thumbCache.has(path)) {
    applyThumb(card, thumbCache.get(path));
    return;
  }
  if (thumbPending.has(path)) {
    pendingCards.set(path, [...(pendingCards.get(path) || []), card]);
    return;
  }
  thumbQueue.push({ card, entry });
  pumpQueue();
}

/** 从队列取任务执行，受 MAX_CONCURRENT 限制。 */
function pumpQueue() {
  while (thumbActive < MAX_CONCURRENT && thumbQueue.length > 0) {
    const job = thumbQueue.shift();
    thumbActive++;
    loadOne(job).finally(() => {
      thumbActive--;
      pumpQueue();
    });
  }
}

/** 加载一张缩略图。 */
async function loadOne({ card, entry }) {
  const path = entry.path;
  if (thumbCache.has(path)) {
    applyThumb(card, thumbCache.get(path));
    return;
  }
  thumbPending.add(path);

  let dataUrl = null;
  try {
    dataUrl = await call('PreviewImage', path);
  } catch {
    dataUrl = null;
  }

  thumbCache.set(path, dataUrl || null);
  thumbPending.delete(path);

  applyThumb(card, dataUrl);
  const waiting = pendingCards.get(path);
  if (waiting) {
    waiting.forEach((c) => applyThumb(c, dataUrl));
    pendingCards.delete(path);
  }
}

/** 把缩略图应用到卡片上；无图时保留类型图标。 */
function applyThumb(card, dataUrl) {
  if (!dataUrl || !card || !card.isConnected) return;
  const thumb = card.querySelector('.thumb');
  if (!thumb) return;
  const icon = thumb.querySelector('[data-icon]');
  if (!icon) return;
  const img = document.createElement('img');
  img.src = dataUrl;
  icon.replaceWith(img);
}

// ---------- 按月份清理 ----------

async function renderMonths() {
  const s = state.scan;
  if (!s) return;

  const container = $('monthList');
  container.innerHTML = '<div class="empty">正在加载…</div>';

  if (!state.grouped) {
    // 平铺模式：一页一页拉
    await renderFlat();
    return;
  }

  // 分组模式：先用月份汇总画桶头，展开时才拉该月条目
  let months;
  try {
    months = await call('Months');
  } catch (err) {
    container.innerHTML = `<div class="empty">加载失败：${esc(err.message || err)}</div>`;
    return;
  }

  if (!months || months.length === 0) {
    container.innerHTML = '';
    $('monthEmpty').hidden = false;
    return;
  }
  $('monthEmpty').hidden = true;

  container.innerHTML = months.map((m, idx) => {
    // 首次进入展开最新三个月
    const open = state.expanded.has(m.month) || (!state.expandedTouched && idx < 3);
    return `<div class="grp-card${open ? ' open' : ''}" data-month="${esc(m.month)}">
      <div class="grp-head">
        <span class="grp-ico">📅</span>
        <div class="grp-title"><b>${esc(m.month)}</b><span class="tag tag-info">月份桶</span></div>
        <div class="grp-nums"><b>${esc(m.bytesText)}</b><span>${m.count} 个</span></div>
        <button class="btn btn-ghost btn-sm" data-pick="${esc(m.month)}">全选</button>
        <button class="btn btn-danger btn-sm" data-clean="${esc(m.month)}" disabled>清理</button>
        <span class="caret">▶</span>
      </div>
      <div class="grp-body" data-body="${esc(m.month)}"></div>
    </div>`;
  }).join('');

  // 组头点击展开/折叠
  container.querySelectorAll('.grp-card[data-month] .grp-head').forEach((h) => {
    h.addEventListener('click', () => {
      const card = h.parentElement;
      const m = card.dataset.month;
      const opening = !card.classList.contains('open');
      card.classList.toggle('open', opening);
      if (opening) {
        state.expandedTouched = true;
        state.expanded.add(m);
        loadMonthBody(m);
      } else {
        state.expandedTouched = true;
        state.expanded.delete(m);
      }
    });
  });

  // 组头内的「全选」与「清理」按钮
  container.querySelectorAll('[data-pick]').forEach((btn) => {
    btn.addEventListener('click', (ev) => {
      ev.stopPropagation();
      pickMonth(btn.dataset.pick);
    });
  });
  container.querySelectorAll('[data-clean]').forEach((btn) => {
    btn.addEventListener('click', (ev) => {
      ev.stopPropagation();
      doClean();
    });
  });

  // 已展开的桶按需加载内容
  container.querySelectorAll('.grp-card.open').forEach((card) => {
    loadMonthBody(card.dataset.month);
  });
}

/** 加载某个月份桶的条目。 */
async function loadMonthBody(month) {
  const body = document.querySelector(`[data-body="${CSS.escape(month)}"]`);
  if (!body || body.dataset.loaded === '1') return;
  body.dataset.loaded = '1';
  body.innerHTML = '<div class="empty">正在加载…</div>';

  try {
    const page = await call('Entries', {
      month: month === '未知月份' ? '' : month,
      minSize: state.sizeFilter,
      category: state.typeFilter === '全部' ? '' : state.typeFilter,
      limit: MAX_CARDS,
      offset: 0,
      sortBy: 'size',
      descending: true,
    });
    body.innerHTML = fileGrid(page.entries) +
      (page.hasMore
        ? `<div class="empty" style="padding:12px">该月还有 ${page.total - page.entries.length} 个，用筛选缩小范围</div>`
        : '');
    bindFileCards(body);
  } catch (err) {
    body.innerHTML = `<div class="empty">加载失败：${esc(err.message || err)}</div>`;
  }
}

/** 平铺模式：跨月份的分页列表。 */
async function renderFlat() {
  const container = $('monthList');
  try {
    const page = await call('Entries', {
      category: state.typeFilter === '全部' ? '' : state.typeFilter,
      minSize: state.sizeFilter,
      limit: 60,
      offset: 0,
      sortBy: 'size',
      descending: true,
    });

    $('monthEmpty').hidden = page.total > 0;
    container.innerHTML = `<div class="grp-card open">
      <div class="grp-head">
        <span class="grp-ico">📋</span>
        <div class="grp-title"><b>全部文件</b></div>
        <div class="grp-nums"><b>${page.total} 个</b></div>
      </div>
      <div class="grp-body">${fileGrid(page.entries)}</div>
    </div>`;
    bindFileCards(container);
  } catch (err) {
    container.innerHTML = `<div class="empty">加载失败：${esc(err.message || err)}</div>`;
  }
}

// ---------- 孤儿与缓存 ----------

async function renderOrphans() {
  const s = state.scan;
  if (!s) return;

  $('orphanSize').textContent = s.orphans.bytesText;
  $('orphanCount').textContent = s.orphans.count + ' 个';

  const grid = $('orphanGrid');
  grid.innerHTML = '<div class="empty">正在加载…</div>';

  try {
    const page = await call('Entries', {
      orphansOnly: true,
      limit: MAX_CARDS,
      offset: 0,
      sortBy: 'size',
      descending: true,
    });

    $('orphanEmpty').hidden = page.total > 0;
    state.orphanTotal = page.total;
    state.orphanOffset = page.entries.length;

    grid.innerHTML = page.total > 0 ? fileGrid(page.entries) : '';
    bindFileCards(grid);
    renderOrphanMore(page.total - page.entries.length);
  } catch (err) {
    grid.innerHTML = `<div class="empty">加载失败：${esc(err.message || err)}</div>`;
  }
}

/** 渲染「加载更多」按钮，点击时向后端请求下一页。 */
function renderOrphanMore(remaining) {
  const old = document.getElementById('orphanMore');
  if (old) old.remove();
  if (remaining <= 0) return;

  const btn = document.createElement('button');
  btn.id = 'orphanMore';
  btn.className = 'btn btn-ghost';
  btn.style.margin = '12px auto 0';
  btn.style.display = 'block';
  btn.textContent = `加载更多（还有 ${remaining} 个）`;

  btn.addEventListener('click', async () => {
    btn.disabled = true;
    btn.textContent = '正在加载…';
    try {
      const page = await call('Entries', {
        orphansOnly: true,
        limit: MAX_CARDS,
        offset: state.orphanOffset || 0,
        sortBy: 'size',
        descending: true,
      });
      state.orphanOffset = (state.orphanOffset || 0) + page.entries.length;

      const grid = $('orphanGrid');
      // 追加而不是重建，避免已加载的缩略图白加载
      const tmp = document.createElement('div');
      tmp.innerHTML = fileGrid(page.entries);
      while (tmp.firstElementChild) grid.appendChild(tmp.firstElementChild);
      bindFileCards(grid);

      renderOrphanMore(page.total - state.orphanOffset);
    } catch (err) {
      btn.disabled = false;
      btn.textContent = '加载失败，点击重试';
    }
  });

  $('orphanGrid').after(btn);
}

// ---------- 底部汇总栏 ----------

function updateActionBar() {
  const bar = $('actionBar');
  const orphanBtn = $('btnCleanOrphans');

  if (state.selected.size === 0) {
    bar.hidden = true;
    if (orphanBtn) orphanBtn.disabled = true;
    return;
  }

  const metas = [...state.selectedMeta.values()];
  const bytes = metas.reduce((a, m) => a + m.size, 0);
  const months = new Set(metas.map((m) => m.month).filter(Boolean));

  bar.hidden = false;
  // 诊断：把 action-bar 的实际几何信息打出来
  $('sumSize').textContent = fmt(bytes);
  $('sumDetail').textContent = `已选 ${state.selected.size} 个文件` +
    (months.size > 0 ? ` · 涉及 ${months.size} 个月份` : '');
  $('btnClean').disabled = false;
  if (orphanBtn) orphanBtn.disabled = false;
}

/** 撤销上一次清理。 */
async function doUndo() {
  const btn = $('btnUndo');
  btn.disabled = true;
  try {
    const res = await call('Undo');
    if (res.restored > 0) {
      toast(`已恢复 ${res.restored} 项（${res.bytesText}）`);
      await doScan(true);
    } else {
      toast('没有可恢复的文件（可能已手动清空废纸篓）');
    }
    if (res.skipped > 0) {
      toast(`已恢复 ${res.restored} 项，${res.skipped} 项跳过（文件已不在废纸篓）`);
    }
  } catch (err) {
    toast('撤销失败：' + (err?.message || err));
  } finally {
    btn.disabled = false;
  }
}

function fmt(n) {
  const kb = 1024, mb = kb * 1024, gb = mb * 1024;
  if (n < kb) return n + ' B';
  if (n < mb) return (n / kb).toFixed(1) + ' KB';
  if (n < gb) return (n / mb).toFixed(1) + ' MB';
  return (n / gb).toFixed(2) + ' GB';
}

// ---------- 清理 ----------

async function doClean() {
  const paths = [...state.selected];
  if (paths.length === 0) return;

  // 先查风险，有风险时把提示放进确认框
  let warning = '';
  try {
    const w = await call('CheckRunning', paths);
    if (w?.running && w.message) warning = w.message;
  } catch { /* 查不到风险就按无风险处理 */ }

  const metas = [...state.selectedMeta.values()];
  const bytes = metas.reduce((a, m) => a + m.size, 0);
  const hasCache = metas.some((m) => m.category === '图片缓存');

  let sub = `共 ${state.selected.size} 项 · 合计 ${fmt(bytes)}`;
  if (warning) sub += `\n\n${warning}`;
  if (hasCache) sub += '\n\n注意：图片缓存是本地唯一副本，较早的缓存删除后可能无法重新下载。';

  $('mTitle').textContent = warning ? '确认清理（微信正在运行）' : '确认清理这些微信文件？';
  $('mSub').textContent = sub;
  $('overlay').hidden = false;

  const confirm = async () => {
    $('overlay').hidden = true;
    $('btnClean').disabled = true;

    try {
      const result = await call('Clean', {
        paths,
        includeCache: hasCache,
        method: 'finder',
      });
      state.selected.clear();

      // 清理后重新体检让数字与实际一致，但要保留清理结果的提示，
      // 否则「已移入废纸篓 N 项」会被「体检完成」立刻盖掉。
      await doScan(true);
      // 清理后启用撤销入口，让用户知道还能反悔
      if (result.moved > 0) {
        $('btnUndo').disabled = false;
        toast(`已移入废纸篓 ${result.moved} 项（${result.bytesText}）· 可点左下角「撤销上次清理」恢复`);
      }
    } catch (err) {
      toast('清理失败：' + (err?.message || err));
      $('btnClean').disabled = false;
    } finally {
      $('mCancel').onclick = null;
      $('mConfirm').onclick = null;
    }
  };

  $('mCancel').onclick = () => { $('overlay').hidden = true; };
  $('mConfirm').onclick = confirm;
}

// ---------- 事件绑定 ----------

function bindEvents() {
  // 导航
  document.querySelectorAll('.nav-item').forEach((n) => {
    n.addEventListener('click', () => switchView(n.dataset.view));
  });
  document.querySelectorAll('[data-goto]').forEach((t) => {
    t.addEventListener('click', () => switchView(t.dataset.goto));
  });

  // 体检
  $('btnScan').addEventListener('click', doScan);
  $('btnRescan').addEventListener('click', doScan);
  $('btnBannerOk').addEventListener('click', () => { $('banner').hidden = true; });

  // 类型筛选
  $('typeFilter').addEventListener('click', (ev) => {
    const btn = ev.target.closest('.seg-btn');
    if (!btn) return;
    $('typeFilter').querySelectorAll('.seg-btn').forEach((b) => b.classList.remove('active'));
    btn.classList.add('active');
    state.typeFilter = btn.dataset.type;
    state.selected.clear();  // 换筛选清空选择，避免清掉看不见的文件
    renderMonths();
    updateActionBar();
  });

  // 大小筛选
  $('sizeFilter').addEventListener('click', (ev) => {
    const btn = ev.target.closest('.seg-btn');
    if (!btn) return;
    $('sizeFilter').querySelectorAll('.seg-btn').forEach((b) => b.classList.remove('active'));
    btn.classList.add('active');
    state.sizeFilter = Number(btn.dataset.size);
    state.selected.clear();
    renderMonths();
    updateActionBar();
  });

  // 分组开关
  const toggleGroup = () => {
    state.grouped = !state.grouped;
    const sw = $('groupSwitch');
    sw.classList.toggle('on', state.grouped);
    sw.setAttribute('aria-checked', String(state.grouped));
    $('groupLabel').textContent = state.grouped ? '按月份分组' : '平铺列表';
    renderMonths();
  };
  $('groupSwitch').addEventListener('click', toggleGroup);
  $('groupSwitch').addEventListener('keydown', (e) => {
    if (e.key === ' ' || e.key === 'Enter') { e.preventDefault(); toggleGroup(); }
  });

  // 孤儿全选
  // 孤儿视图内直接提供清理入口，不用滚到底部找汇总栏
  $('btnCleanOrphans').addEventListener('click', doClean);

  // 撤销上次清理
  $('btnUndo').addEventListener('click', doUndo);


  $('btnPickOrphans').addEventListener('click', () => {
    // 全选当前已渲染的孤儿卡片（后端分页，前端只知道自己渲染过的）
    const cards = [...document.querySelectorAll('#orphanGrid .fc[data-path]')];
    const allSelected = cards.length > 0 && cards.every((c) => state.selected.has(c.dataset.path));
    cards.forEach((c) => {
      const path = c.dataset.path;
      if (allSelected) {
        state.selected.delete(path);
        state.selectedMeta.delete(path);
        c.classList.remove('sel');
      } else {
        state.selected.add(path);
        state.selectedMeta.set(path, {
          size: Number(c.dataset.size || 0),
          month: c.dataset.month || '',
          category: c.dataset.category || '',
          name: c.dataset.name || '',
        });
        c.classList.add('sel');
      }
    });
    renderOrphans();
    updateActionBar();
  });

  // 底部栏
  $('btnClear').addEventListener('click', () => {
    state.selected.clear();
    renderMonths();
    renderOrphans();
    updateActionBar();
  });
  $('btnClean').addEventListener('click', doClean);

  // 弹窗遮罩与 Esc
  $('overlay').addEventListener('click', (e) => {
    if (e.target === $('overlay')) $('overlay').hidden = true;
  });
  document.addEventListener('keydown', (e) => {
    if (e.key === 'Escape' && !$('overlay').hidden) $('overlay').hidden = true;
  });
}

// ---------- 启动 ----------

window.addEventListener('DOMContentLoaded', async () => {
  bindEvents();
  // 自动体检一次，省去用户点按钮
  try {
    await doScan();
  } catch (err) {
    toast('体检失败：' + (err?.message || err));
  }
});

/** 全选某个已加载月份桶里的卡片。 */
function pickMonth(month) {
  const body = document.querySelector(`[data-body="${CSS.escape(month)}"]`);
  if (!body) return;
  const cards = [...body.querySelectorAll('.fc[data-path]')];
  if (cards.length === 0) {
    toast('请先展开该月份');
    return;
  }

  const allSelected = cards.every((c) => state.selected.has(c.dataset.path));
  cards.forEach((c) => {
    const path = c.dataset.path;
    if (allSelected) {
      state.selected.delete(path);
      state.selectedMeta.delete(path);
      c.classList.remove('sel');
    } else {
      state.selected.add(path);
      state.selectedMeta.set(path, {
        size: Number(c.dataset.size || 0),
        month: c.dataset.month || '',
        category: c.dataset.category || '',
        name: c.dataset.name || '',
      });
      c.classList.add('sel');
    }
  });

  syncMonthCleanButtons();
  updateActionBar();
  toast(allSelected ? `已取消 ${month} 的选择` : `已选中 ${month} 的 ${cards.length} 个文件`);
}
