"""布局候选区域（安全区、锚点、候选优先级）。

屏幕在留出安全边距后划分 9 个锚点区域；字幕与手语分别带各自的
候选锚点优先级序列，规划器按该顺序寻找无冲突位置。
"""

from __future__ import annotations

from dataclasses import dataclass

from .geometry import Rect, clamp01
from .layers import LayerKind, SubjectObservation

# 电视/移动端通用安全区边距，避免内容贴边被裁切或与系统 UI 重叠。
DEFAULT_SAFE_MARGIN_X = 0.04
DEFAULT_SAFE_MARGIN_Y = 0.05


@dataclass(frozen=True, slots=True)
class Zone:
    """一个可放置图层的候选区域。

    preference 越小越优先（0 为最优先）；allowed 限定可使用该区域的层。
    """

    zone_id: str
    anchor: str
    rect: Rect
    preference: int
    allowed: frozenset[LayerKind]

    def accepts(self, kind: LayerKind) -> bool:
        return kind in self.allowed


def _anchor_rect(anchor: str, mx: float, my: float) -> Rect:
    """把 3x3 网格的某个锚点映射为安全区内矩形。"""

    col, row = anchor.split("_")
    cell_w = (1.0 - 2 * mx) / 3.0
    cell_h = (1.0 - 2 * my) / 3.0
    col_idx = {"left": 0, "center": 1, "right": 2}[col]
    row_idx = {"top": 0, "middle": 1, "bottom": 2}[row]
    x = mx + col_idx * cell_w
    y = my + row_idx * cell_h
    return Rect(clamp01(x), clamp01(y), cell_w, cell_h)


def build_default_zones(
    margin_x: float = DEFAULT_SAFE_MARGIN_X,
    margin_y: float = DEFAULT_SAFE_MARGIN_Y,
) -> tuple[Zone, ...]:
    """生成 9 个锚点区域及各层候选优先级。

    字幕优先底部居中（阅读习惯），手语优先右下角（电视手语惯例），
    两者候选序列刻意错开，降低争抢同一区域的概率。
    """

    anchors = (
        "left_top", "center_top", "right_top",
        "left_middle", "center_middle", "right_middle",
        "left_bottom", "center_bottom", "right_bottom",
    )
    subtitle_order = (
        "center_bottom", "left_bottom", "right_bottom",
        "center_middle", "left_middle", "right_middle",
        "center_top", "left_top", "right_top",
    )
    sign_order = (
        "right_bottom", "left_bottom", "right_middle",
        "left_middle", "center_bottom", "right_top",
        "left_top", "center_middle", "center_top",
    )
    sub_rank = {anchor: rank for rank, anchor in enumerate(subtitle_order)}
    sign_rank = {anchor: rank for rank, anchor in enumerate(sign_order)}

    zones: list[Zone] = []
    for anchor in anchors:
        rect = _anchor_rect(anchor, margin_x, margin_y)
        preference = min(sub_rank[anchor], sign_rank[anchor])
        zones.append(
            Zone(
                zone_id=f"zone_{anchor}",
                anchor=anchor,
                rect=rect,
                preference=preference,
                allowed=frozenset({LayerKind.SUBTITLE, LayerKind.SIGN_PIP}),
            )
        )
    return tuple(zones)


def candidate_zones(
    zones: tuple[Zone, ...], kind: LayerKind
) -> tuple[Zone, ...]:
    """按层的候选优先级返回可用区域。"""

    if kind == LayerKind.SUBTITLE:
        order = (
            "center_bottom", "left_bottom", "right_bottom", "center_middle",
            "left_middle", "right_middle", "center_top", "left_top",
            "right_top",
        )
    else:
        order = (
            "right_bottom", "left_bottom", "right_middle", "left_middle",
            "center_bottom", "right_top", "left_top", "center_middle",
            "center_top",
        )
    rank = {anchor: idx for idx, anchor in enumerate(order)}
    usable = [zone for zone in zones if zone.accepts(kind)]
    return tuple(sorted(usable, key=lambda zone: rank[zone.anchor]))


def subject_avoid_rect(
    subject: SubjectObservation,
    pad_x: float = 0.02,
    pad_y: float = 0.02,
) -> Rect | None:
    """主体避让区 = 主体检测框 + 呼吸边距。

    无画面语义时返回 None，由调用方决定转人工确认。
    """

    if not subject.has_semantics or subject.rect is None:
        return None
    return subject.rect.expanded(pad_x, pad_y)
