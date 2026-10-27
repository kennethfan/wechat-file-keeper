# 微信文件管家

macOS 微信本地文件的管理工具：只读扫描微信留下的聊天文件、视频与图片缓存，按空间占用呈现，用户显式确认后才把选中项移入废纸篓。

## 为什么需要它

微信自己的「存储空间管理」只给一个总数，不能按大小排序、不能按月看，也不告诉你哪些是重复的。在 Finder 里手动翻容器目录更麻烦——而且 Finder 会把硬链接的同一份文件算成多个文件，让人误以为有几倍空间可以回收。

本机实测（微信 4.1.15，账号库 5.5 GB）：

| 范围 | 实占 | 说明 |
|---|---|---|
| `msg/video` | 2.34 GB | 122 个 mp4，单个最大 374 MB |
| `msg/attach` | 2.03 GB | 12407 个 `.dat` 图片缓存 |
| `msg/file` | 0.51 GB | 180 个文件，保留原始文件名 |
| 硬链接虚增 | 331 MB | 同一 inode 挂了多个名字，删除**不释放空间** |
| 真重复 | 7.0 MB | 不同 inode 但内容一致 |

## 设计边界

- **不碰加密数据库**：谁发的文件、什么时候发的，都存在 `db_storage` 的加密 SQLite 里。本工具不读取、不解密，因此不提供会话归属（见 [ADR-0002](docs/adr/0002-no-conversation-attribution.md)）
- **空间以 inode 为准**：实占按 inode 归并，名义大小仅作对照，避免把硬链接误报成可回收空间（见 [ADR-0001](docs/adr/0001-inode-based-space-accounting.md)）
- **只移入废纸篓**：唯一的删除动作是 rename 到 `~/.Trash`，绝不 unlink；每次操作写 JSON 清单，支持撤销（见 [ADR-0003](docs/adr/0003-trash-via-rename.md)）
- **不建索引**：每次全量扫描，结果永远与当下磁盘一致（见 [ADR-0004](docs/adr/0004-no-persistent-index.md)）

## 构建

```bash
make build      # 编译到 bin/wechat-file-keeper
make scan       # 编译并扫描本机账号库
make check      # gofmt + vet + test
```

受限沙箱下 Makefile 会把 `GOPATH` / `GOMODCACHE` / `GOCACHE` 指向工作区内，避免写 `~/.gvm` 被拒。

## 使用

```bash
wechat-file-keeper scan                 # 扫描并显示空间统计
wechat-file-keeper scan --lib <路径>     # 指定账号库
wechat-file-keeper scan --root <路径>    # 指定微信数据根目录
```

输出三个数字，缺一不可：

```
实占大小  4.77 GB
名义大小  5.09 GB
硬链接虚增 331.4 MB  （同一 inode 挂了多个名字，删除不释放空间）
```

### 子命令一览

```bash
wechat-file-keeper scan                   # 空间统计
wechat-file-keeper list --category 视频 --min-size 100MB   # 按分类与大小筛选
wechat-file-keeper dup                    # 真重复分组（只读，不提供删除）
wechat-file-keeper cache                  # 图片缓存年份分布
wechat-file-keeper clean --orphans        # 预览清理孤儿缩略图
wechat-file-keeper clean --orphans --yes  # 确认执行
wechat-file-keeper undo                   # 回滚最近一次清理
wechat-file-keeper gui                    # 图形界面
```

`clean` 默认只打印计划，必须加 `--yes` 才动文件。图片缓存需额外加
`--include-cache`：它是本地唯一副本，微信服务器有过期策略，老缓存删了
可能再也下载不回来。

### 图形界面

`wechat-file-keeper gui`，或打包后双击 `.app`。左侧是分类树（带实占大小），
右侧是可多选的条目表，底部显示「已选 N 项，合计 X MB」。微信运行时清理
按钮会置灰并说明原因。

## 打包成 .app

```bash
make app      # 产出 dist/微信文件管家.app，ad-hoc 签名
make icon     # 仅重新生成图标（需 Pillow）
```

默认走 ad-hoc 签名，只够本机运行。要让别人双击打开需要 Apple 开发者证书：

```bash
make app SIGN_IDENTITY="Developer ID Application: Your Name (TEAMID)"
```

图标按 Apple HIG 绘制：1024 画布上圆角方块主体 824×824 居中，四边各留
100px 透明边距。源图由 `packaging/make_icon_source.py` 生成，再经
`packaging/make_icon.py` 打包成 `.icns`。

## 开发文档

- [CONTEXT.md](CONTEXT.md) — 领域词汇表
- [docs/adr/](docs/adr/) — 架构决策记录
- [AGENTS.md](AGENTS.md) — 工程技能配置（issue tracker、triage 标签、领域文档布局）

## 许可

MIT
