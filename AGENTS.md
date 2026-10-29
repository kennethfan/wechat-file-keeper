# wechat-file-keeper

macOS 微信文件清理工具。技术栈：Go 1.26 + Wails v3（WebView 前端）。

## 架构要点

- **UI**：Wails v3 + WebView，前端在 `frontend/dist/`（原生 HTML/CSS/JS，无构建步骤）
- **前后端通信**：自建 HTTP JSON API（`internal/apiserver`，`/api/*`），
  **不用** Wails 的绑定机制——它依赖的 `/wails/runtime.js` 在本机 WebKit 上静默加载失败
- **业务逻辑**：`internal/` 下的包与 UI 无关，可独立测试
- **删除方式**：只用 Finder（`osascript`），因此废纸篓里有「放回原处」。
  不要改回 rename——那样废纸篓里既没「放回原处」也难辨认文件
- **性能约束**：体检接口不返回全量条目（1.4 万条约 7.6MB），
  改用 `/api/entries` 分页；缩略图请求有并发上限与缓存

## 开发命令

```bash
make check    # gofmt + vet + test
make app      # 打包 dist/微信文件管家.app
make gui      # 直接跑二进制
```

改前端后**必须重新编译二进制**（前端被 embed），只重启进程不生效。
改 `Info.plist` 后必须重新 `make app`（权限说明只在 .app 里生效）。

## Agent skills

### Issue tracker

Issues and specs live as GitHub issues in `kennethfan/wechat-file-keeper`, driven by the `gh` CLI. See `docs/agents/issue-tracker.md`.

### Triage labels

Default five-role vocabulary (`needs-triage`, `needs-info`, `ready-for-agent`, `ready-for-human`, `wontfix`). See `docs/agents/triage-labels.md`.

### Domain docs

Single-context: one `CONTEXT.md` at the repo root plus `docs/adr/`. See `docs/agents/domain.md`.
