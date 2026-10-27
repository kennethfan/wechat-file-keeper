#!/usr/bin/env python3
"""生成 macOS 应用图标源图 packaging/icon-source.png。

用法：
    python3 packaging/make_icon_source.py

按 Apple HIG 绘制：1024 画布上圆角方块主体 824x824 居中，
四边各留 100px 透明边距（占比 824/1024 ≈ 80.5%）。

图形语义：一只文件盒（keeper 的「保管」意象），盒内叠放几份文件，
右下角一个绿色对勾表示「已安全处置」。颜色取微信绿，与工具用途呼应。
"""

from __future__ import annotations

import sys
from pathlib import Path

try:
    from PIL import Image, ImageDraw
except ImportError:
    sys.exit("缺少 Pillow，请先安装：pip install Pillow")

CANVAS = 1024
MARGIN = 100
BODY = CANVAS - MARGIN * 2  # 824
RADIUS = 185

REPO_ROOT = Path(__file__).resolve().parent.parent
OUTPUT = REPO_ROOT / "packaging" / "icon-source.png"

# 微信绿渐变（上浅下深），让图标在浅色与深色桌面上都有辨识度。
TOP_COLOR = (43, 196, 108)
BOTTOM_COLOR = (18, 150, 82)
PAPER = (255, 255, 255)
PAPER_EDGE = (222, 236, 228)
CHECK = (255, 255, 255)
BOX = (11, 122, 66)


def rounded_mask(size: int, radius: int) -> Image.Image:
    """生成圆角矩形遮罩。"""
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        (0, 0, size - 1, size - 1), radius=radius, fill=255
    )
    return mask


def vertical_gradient(size: int, top: tuple[int, int, int], bottom: tuple[int, int, int]) -> Image.Image:
    """生成竖直渐变。"""
    grad = Image.new("RGB", (1, size))
    for y in range(size):
        ratio = y / max(size - 1, 1)
        grad.putpixel(
            (0, y),
            tuple(int(top[i] + (bottom[i] - top[i]) * ratio) for i in range(3)),
        )
    return grad.resize((size, size), Image.NEAREST)


def draw_files(draw: ImageDraw.ImageDraw, x: int, y: int) -> None:
    """画叠放的三份文件。

    从后往前画：左右两份先画且略小，中间那份最后画在最上层，
    这样三份都能露出边角，读起来是「一叠文件」而不是一张歪纸。
    """
    w, h = 300, 360
    # (水平偏移, 垂直偏移, 是否画文字线)
    layers = ((-70, 30, False), (70, 30, False), (0, 0, True))
    for offset, dy, with_lines in layers:
        left = x + offset
        top = y + dy
        draw.rounded_rectangle(
            (left, top, left + w, top + h),
            radius=26,
            fill=PAPER,
            outline=PAPER_EDGE,
            width=6,
        )
        if with_lines:
            for line in range(4):
                ly = top + 90 + line * 52
                draw.rounded_rectangle(
                    (left + 44, ly, left + w - 44, ly + 16),
                    radius=8,
                    fill=(176, 205, 189),
                )


def draw_check(draw: ImageDraw.ImageDraw, cx: int, cy: int) -> None:
    """画带底色圆的对勾。"""
    r = 118
    draw.ellipse((cx - r, cy - r, cx + r, cy + r), fill=BOX, outline=PAPER, width=14)
    # 两段线组成对勾。
    draw.line(
        [(cx - 52, cy + 4), (cx - 14, cy + 44)],
        fill=CHECK,
        width=34,
        joint="curve",
    )
    draw.line(
        [(cx - 14, cy + 44), (cx + 56, cy - 42)],
        fill=CHECK,
        width=34,
        joint="curve",
    )


def main() -> int:
    canvas = Image.new("RGBA", (CANVAS, CANVAS), (0, 0, 0, 0))

    # 主体：824x824 圆角方块，居中放置。
    body = vertical_gradient(BODY, TOP_COLOR, BOTTOM_COLOR).convert("RGBA")
    body.putalpha(rounded_mask(BODY, RADIUS))
    canvas.paste(body, (MARGIN, MARGIN), body)

    draw = ImageDraw.Draw(canvas)
    # 文件叠放在方块中上部，对勾压在右下角。
    draw_files(draw, MARGIN + 190, MARGIN + 150)
    draw_check(draw, MARGIN + BODY - 190, MARGIN + BODY - 190)

    canvas.save(OUTPUT)
    print(f"已生成 {OUTPUT}（{CANVAS}x{CANVAS}，主体 {BODY}x{BODY}，边距 {MARGIN}px）")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
