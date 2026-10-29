# 删除只用 Finder，废弃 rename 方案

ADR-0003 决定用 `rename` 把文件搬进 `~/.Trash`，理由是 Finder 删除依赖 Apple Events 授权、在受限环境下会失败。这个决定在实测后被推翻。

## 为什么改

`rename` 能把文件放进废纸篓目录，但 **macOS 只对 Finder 移入的项目记录原路径**。因此 rename 出来的废纸篓项：

- 没有「放回原处」，无法一键还原
- 文件名可能是哈希串（微信的视频/图片缓存），用户认不出是什么
- 仍然占着磁盘空间

这三条加起来，`rename` 等于**自己造了个废纸篓**——既然目标就是系统废纸篓，就没有理由不用 Finder。原先「rename 比直接删除安全」的论证站不住脚：如果放不回原处，rename 和直接删除对用户没有实质差别。

## 决定

删除动作只走 Finder（`osascript -e 'tell application "Finder" to delete ...'`）。

- `Info.plist` 必须声明 `NSAppleEventsUsageDescription`。缺这一条时系统**不会弹授权框，而是直接拒绝**——这是早期 Finder 模式失败的真正原因。
- Finder 失败时明确报错并提示去「系统设置 → 隐私与安全性 → 自动化」开权限，**不回退**到 rename。静默降级会得到一个用户以为能放回原处、实际不能的废纸篓，比直接失败更糟。
- Finder 批量删除是原子性的：任一文件失败则整体失败，不产生部分清单。这比「删一半」更可预期。

## Consequences

- 首次清理会弹出「想要控制 Finder」授权框，拒绝后无法删除（错误信息会说明原因）。
- JSON 清单仍然保留：Finder 会为同名文件改名，清单记录实际落点，供本工具的 `undo` 使用；它也是系统废纸篓之外的第二重保障。
- 若将来 macOS 收紧 Apple Events 权限，需要重新评估。
