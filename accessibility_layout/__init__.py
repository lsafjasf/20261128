"""可访问性层布局引擎。

解决手语画中画（SIGN_PIP）、底部字幕（SUBTITLE）与画面主体（SUBJECT）
在同一区域互相争抢、切镜头后互相遮挡的问题。

公共入口：
    - Rect / 几何计算：accessibility_layout.geometry
    - 图层与时间轴模型：accessibility_layout.layers
    - 候选区域与优先级：accessibility_layout.zones
    - 冲突检测：accessibility_layout.conflict
    - 布局规划（可执行动作 + 人工确认）：accessibility_layout.planner
"""

from .geometry import Rect
from .layers import FrameSample, LayerKind, LayerObservation, SubjectObservation
from .zones import Zone, build_default_zones
from .conflict import Conflict, ConflictType, detect_conflicts
from .planner import LayoutPlanner, LayoutPlan, LayoutAction, PlannerConfig

__all__ = [
    "Rect",
    "FrameSample",
    "LayerKind",
    "LayerObservation",
    "SubjectObservation",
    "Zone",
    "build_default_zones",
    "Conflict",
    "ConflictType",
    "detect_conflicts",
    "LayoutPlanner",
    "LayoutPlan",
    "LayoutAction",
    "PlannerConfig",
]
