#!/usr/bin/env python3
"""从 packaging/icon-source.png 生成 AppIcon.icns。

用法：
    python3 packaging/make_icon.py

依赖：Pillow（pip install Pillow）、macOS 自带的 iconutil。

说明：macOS 图标规范要求圆角矩形之外为透明。icon-source.png
应已满足该条件（圆角外 alpha=0）；本脚本只负责按 10 个标准尺寸
缩放并打包，不修改圆角。
"""

from __future__ import annotations

import shutil
import subprocess
import sys
import tempfile
from pathlib import Path

try:
    from PIL import Image
except ImportError:
    sys.exit("缺少 Pillow，请先安装：pip install Pillow")

# macOS iconset 规范要求的 10 个文件
ICONSET_SPECS: list[tuple[str, int]] = [
    ("icon_16x16.png", 16),
    ("icon_16x16@2x.png", 32),
    ("icon_32x32.png", 32),
    ("icon_32x32@2x.png", 64),
    ("icon_128x128.png", 128),
    ("icon_128x128@2x.png", 256),
    ("icon_256x256.png", 256),
    ("icon_256x256@2x.png", 512),
    ("icon_512x512.png", 512),
    ("icon_512x512@2x.png", 1024),
]

REPO_ROOT = Path(__file__).resolve().parent.parent
SOURCE = REPO_ROOT / "packaging" / "icon-source.png"
OUTPUT = REPO_ROOT / "packaging" / "AppIcon.icns"


def main() -> int:
    if not SOURCE.exists():
        print(f"找不到源图: {SOURCE}", file=sys.stderr)
        return 1
    if shutil.which("iconutil") is None:
        print("找不到 iconutil（需在 macOS 上运行）", file=sys.stderr)
        return 1

    src = Image.open(SOURCE).convert("RGBA")
    if src.size != (1024, 1024):
        print(f"警告：源图为 {src.size}，建议 1024×1024", file=sys.stderr)

    with tempfile.TemporaryDirectory() as tmp:
        iconset = Path(tmp) / "AppIcon.iconset"
        iconset.mkdir()

        for name, size in ICONSET_SPECS:
            resized = src.resize((size, size), Image.LANCZOS)
            resized.save(iconset / name)
            print(f"  {name:24s} {size}×{size}")

        subprocess.run(
            ["iconutil", "-c", "icns", str(iconset), "-o", str(OUTPUT)],
            check=True,
        )

    print(f"已生成 {OUTPUT}  ({OUTPUT.stat().st_size / 1024 / 1024:.1f} MB)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
