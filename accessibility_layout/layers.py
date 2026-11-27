"""可访问性图层、画面主体与时间轴采样模型。"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import IntEnum

from .geometry import Rect


class LayerKind(IntEnum):
    """可访问性层类型。

    数值越大优先级越高：字幕承载语音信息，听障用户必须可读；
    手语画中画次之；画面主体是被避让方，优先级最低，由前两者选位避开。
    """

    SUBJECT = 0
    SIGN_PIP = 1
    SUBTITLE = 2


# 优先级排序（高 -> 低），供冲突仲裁与布局规划使用。
LAYER_PRIORITY: tuple[LayerKind, ...] = (
    LayerKind.SUBTITLE,
    LayerKind.SIGN_PIP,
    LayerKind.SUBJECT,
)


@dataclass(frozen=True, slots=True)
class SubjectObservation:
    """画面主体检测结果。

    confidence 为主体检测置信度；has_semantics=False 表示当前没有
    可用的画面语义（检测器缺测/遮挡/置信度过低），此时必须转人工确认，
    不允许把“未检测到主体”误判为“该区域无主体”。
    """

    rect: Rect | None
    confidence: float
    has_semantics: bool = True
    label: str = "subject"


@dataclass(frozen=True, slots=True)
class LayerObservation:
    """单个可访问性层在某一时刻的观测：当前占位与期望尺寸。"""

    kind: LayerKind
    rect: Rect
    min_width: float
    min_height: float
    visible: bool = True


@dataclass(frozen=True, slots=True)
class FrameSample:
    """时间轴上的一个采样点。

    t 单位为秒；shot_id 标识镜头，shot_id 变化即切镜头。
    subtitle/sign 缺失表示该时刻该层不可见（无台词 / 无手语流）。
    """

    t: float
    shot_id: str
    subject: SubjectObservation
    subtitle: LayerObservation | None = None
    sign: LayerObservation | None = None

    def visible_layers(self) -> tuple[LayerObservation, ...]:
        return tuple(
            layer for layer in (self.subtitle, self.sign)
            if layer is not None and layer.visible
        )


@dataclass(frozen=True, slots=True)
class Timeline:
    """按时间升序排列的帧采样序列。"""

    samples: tuple[FrameSample, ...] = field(default_factory=tuple)

    def __post_init__(self) -> None:
        times = [sample.t for sample in self.samples]
        if times != sorted(times):
            raise ValueError("Timeline 的采样必须按时间 t 升序排列")

    def shot_segments(self) -> tuple[tuple[str, float, float], ...]:
        """返回 (shot_id, 起始时间, 结束时间) 镜头段。"""

        segments: list[list] = []
        for sample in self.samples:
            if segments and segments[-1][0] == sample.shot_id:
                segments[-1][2] = sample.t
            else:
                segments.append([sample.shot_id, sample.t, sample.t])
        return tuple((shot, start, end) for shot, start, end in segments)
