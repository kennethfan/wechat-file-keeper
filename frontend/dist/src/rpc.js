// 后端调用层：普通 HTTP JSON 接口。
//
// 不用 Wails 的绑定机制——它依赖 /wails/runtime.js 这个 ES module，
// 在本机 WebKit（macOS 12）上加载失败，导致所有后端调用静默不生效。
// 这里改用 Go 端自建的 /api/* 接口，前端只用 fetch，没有额外依赖。

/** 把诊断信息写到页面顶部，方便定位问题。 */
export function diag(msg) {
  const el = document.getElementById('boot-error');
  if (!el) return;
  el.hidden = false;
  el.textContent = (el.textContent ? el.textContent + ' | ' : '') + msg;
}

/** 调用后端接口。 */
async function post(action, body) {
  const resp = await fetch('/api/' + action, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body || {}),
  });

  const text = await resp.text();
  let data;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    throw new Error('响应不是 JSON: ' + text.slice(0, 120));
  }

  if (!resp.ok) {
    throw new Error((data && data.error) || ('HTTP ' + resp.status));
  }
  if (data && data.error) {
    throw new Error(data.error);
  }
  return data;
}

export function scan(libPath) {
  return post('scan', { libPath: libPath || '' });
}

export function clean(paths, includeCache) {
  return post('clean', { paths, includeCache: !!includeCache });
}

export function undo() {
  return post('undo', {});
}

export function checkRunning(paths) {
  return post('checkRunning', { paths });
}

export function preview(path) {
  return post('preview', { path });
}

export function openFile(path) {
  return post('open', { path });
}

export function reveal(path) {
  return post('reveal', { path });
}

export function libraries() {
  return post('libraries', {});
}

/** 分页查询条目。 */
export function entries(query) {
  return post('entries', query);
}

/** 月份汇总。 */
export function months() {
  return post('months', {});
}

/** 查询/设置删除方式。 */
export function trashMethod(method) {
  return post('trashMethod', { method: method || '' });
}
