"""字幕 / 手语 / 画面主体三方冲突检测。"""

from __future__ import annotations

from dataclasses import dataclass
from enum import Enum

from .geometry import Rect
from .layers import FrameSample, LayerKind, LayerObservation, SubjectObservation
from .zones import subject_avoid_rect


class ConflictType(str, Enum):
    LAYER_OVERLAP = "layer_overlap"            # 两个可访问性层互相遮挡
    SUBJECT_OCCLUSION = "subject_occlusion"    # 可访问性层压住画面主体
    THREE_WAY = "three_way"                    # 三方在同一区域冲突
    NO_SEMANTICS = "no_semantics"              # 无画面语义，需人工确认


@dataclass(frozen=True, slots=True)
class Conflict:
    """一次冲突事件。

    parties 为冲突方图层类型；overlap 为交集矩形；severity 取
    [0,1]，对层间冲突取交叠占较小层面积比例，对主体遮挡取
    交叠占主体避让区面积比例。
    """

    t: float
    shot_id: str
    type: ConflictType
    parties: tuple[LayerKind, ...]
    overlap: Rect | None
    severity: float
    needs_human_confirmation: bool
    detail: str


def _severity(overlap_rect: Rect, reference: Rect) -> float:
    if reference.area <= 1e-6:
        return 0.0
    return min(1.0, overlap_rect.area / reference.area)


def _pair_overlap(a: LayerObservation, b: LayerObservation) -> Rect | None:
    return a.rect.intersection(b.rect)


def detect_conflicts(sample: FrameSample) -> tuple[Conflict, ...]:
    """检测单帧内的全部冲突。

    规则：
    1. 字幕与手语画中画不允许任何交叠（层间硬约束）。
    2. 可访问性层与主体避让区交叠即记主体遮挡。
    3. 同一交集同时涉及字幕、手语、主体，升级为三方冲突。
    4. 主体无画面语义时输出 NO_SEMANTICS，并强制人工确认。
    """

    conflicts: list[Conflict] = []
    layers = sample.visible_layers()
    avoid = subject_avoid_rect(sample.subject)

    if not sample.subject.has_semantics:
        conflicts.append(
            Conflict(
                t=sample.t,
                shot_id=sample.shot_id,
                type=ConflictType.NO_SEMANTICS,
                parties=(LayerKind.SUBJECT,),
                overlap=None,
                severity=1.0,
                needs_human_confirmation=True,
                detail=(
                    f"t={sample.t:.3f} 镜头 {sample.shot_id} 缺少画面语义"
                    "（检测缺测/置信度过低），无法判断主体是否被遮挡，"
                    "必须人工确认布局"
                ),
            )
        )

    # 1) 可访问性层两两交叠
    layer_pair_hits: list[tuple[LayerObservation, LayerObservation, Rect]] = []
    for i, left in enumerate(layers):
        for right in layers[i + 1:]:
            hit = _pair_overlap(left, right)
            if hit is not None:
                layer_pair_hits.append((left, right, hit))
                reference = min(
                    (left.rect, right.rect), key=lambda rect: rect.area
                )
                conflicts.append(
                    Conflict(
                        t=sample.t,
                        shot_id=sample.shot_id,
                        type=ConflictType.LAYER_OVERLAP,
                        parties=(left.kind, right.kind),
                        overlap=hit,
                        severity=_severity(hit, reference),
                        needs_human_confirmation=False,
                        detail=(
                            f"t={sample.t:.3f} {left.kind.name} 与 "
                            f"{right.kind.name} 在同一区域互相遮挡"
                        ),
                    )
                )

    # 2) 层与主体避让区交叠
    subject_hits: dict[LayerKind, Rect] = {}
    if avoid is not None:
        for layer in layers:
            hit = layer.rect.intersection(avoid)
            if hit is not None:
                subject_hits[layer.kind] = hit
                conflicts.append(
                    Conflict(
                        t=sample.t,
                        shot_id=sample.shot_id,
                        type=ConflictType.SUBJECT_OCCLUSION,
                        parties=(layer.kind, LayerKind.SUBJECT),
                        overlap=hit,
                        severity=_severity(hit, avoid),
                        needs_human_confirmation=not
                        sample.subject.has_semantics,
                        detail=(
                            f"t={sample.t:.3f} {layer.kind.name} 压住画面主体"
                        ),
                    )
                )

    # 3) 三方冲突：层间交叠区域同时与主体避让区相交
    for left, right, layer_hit in layer_pair_hits:
        if (left.kind in subject_hits and right.kind in subject_hits
                and layer_hit.intersects(avoid)):
            common = layer_hit.intersection(avoid)
            conflicts.append(
                Conflict(
                    t=sample.t,
                    shot_id=sample.shot_id,
                    type=ConflictType.THREE_WAY,
                    parties=(left.kind, right.kind, LayerKind.SUBJECT),
                    overlap=common,
                    severity=_severity(common, avoid),
                    needs_human_confirmation=not
                    sample.subject.has_semantics,
                    detail=(
                        f"t={sample.t:.3f} 字幕、手语、画面主体三方争抢同一"
                        "区域"
                    ),
                )
            )

    return tuple(conflicts)


def detect_timeline(
    samples: tuple[FrameSample, ...],
) -> tuple[Conflict, ...]:
    """对整条时间轴做冲突检测。"""

    all_conflicts: list[Conflict] = []
    for sample in samples:
        all_conflicts.extend(detect_conflicts(sample))
    return tuple(all_conflicts)
