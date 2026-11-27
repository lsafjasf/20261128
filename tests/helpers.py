"""测试公共构造工具。"""

from accessibility_layout.geometry import Rect
from accessibility_layout.layers import (
    FrameSample,
    LayerKind,
    LayerObservation,
    SubjectObservation,
)


def subtitle(x=0.25, y=0.86, w=0.5, h=0.1) -> LayerObservation:
    return LayerObservation(
        kind=LayerKind.SUBTITLE,
        rect=Rect(x, y, w, h),
        min_width=0.3,
        min_height=0.06,
    )


def sign(x=0.72, y=0.72, w=0.22, h=0.24) -> LayerObservation:
    return LayerObservation(
        kind=LayerKind.SIGN_PIP,
        rect=Rect(x, y, w, h),
        min_width=0.14,
        min_height=0.16,
    )


def subject(x=0.3, y=0.2, w=0.4, h=0.5, confidence=0.95) -> SubjectObservation:
    return SubjectObservation(
        rect=Rect(x, y, w, h), confidence=confidence, has_semantics=True
    )


def no_semantics() -> SubjectObservation:
    return SubjectObservation(
        rect=None, confidence=0.0, has_semantics=False
    )


def frame(
    t=0.0,
    shot="s1",
    subject_obs=None,
    subtitle_obs=None,
    sign_obs=None,
) -> FrameSample:
    return FrameSample(
        t=t,
        shot_id=shot,
        subject=subject_obs if subject_obs is not None else subject(),
        subtitle=subtitle_obs,
        sign=sign_obs,
    )
