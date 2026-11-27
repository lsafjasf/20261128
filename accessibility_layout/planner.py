"""布局规划器：把冲突检测结果转换为可执行的布局动作。

规划策略：
1. 按优先级依次摆放字幕（高）与手语（低），先摆者占据区域，
   后摆者不得与其重叠（层间硬约束，任何成本都不可违反）。
2. 每个候选位置计算成本：主体遮挡 > 锚点偏好 > 位置抖动 > 缩放。
3. 持续时间由最小驻留 + 防抖窗口控制，避免布局来回跳；
   切镜头（shot_id 变化）立即重新评估，不受驻留时间限制。
4. 无画面语义或无合法位置时输出 MANUAL_REVIEW，必须人工确认。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum

from .conflict import Conflict, ConflictType, detect_conflicts
from .geometry import Rect
from .layers import FrameSample, LayerKind, LayerObservation, Timeline
from .zones import Zone, build_default_zones, candidate_zones, subject_avoid_rect


class ActionType(str, Enum):
    HOLD = "hold"                       # 保持当前布局
    MOVE = "move"                       # 平移到新锚点
    RESIZE = "resize"                   # 缩小到区域可容纳尺寸
    MOVE_AND_RESIZE = "move_and_resize"
    MANUAL_REVIEW = "manual_review"     # 转人工确认


@dataclass(frozen=True, slots=True)
class PlannerConfig:
    """规划参数（时间单位均为秒）。"""

    min_dwell: float = 1.5
    debounce_window: float = 0.4
    subject_pad_x: float = 0.02
    subject_pad_y: float = 0.02
    subject_weight: float = 10.0
    preference_weight: float = 1.0
    move_weight: float = 3.0
    resize_weight: float = 4.0


@dataclass(frozen=True, slots=True)
class LayoutAction:
    """一条可执行布局指令。

    渲染侧按 target_rect 放置图层；human_confirmation_required=True
    时该指令不得自动生效，必须等待人工裁决。
    """

    t_start: float
    t_end: float
    shot_id: str
    kind: LayerKind
    type: ActionType
    from_rect: Rect | None
    target_rect: Rect | None
    zone_id: str | None
    reason: str
    human_confirmation_required: bool = False
    conflicts: tuple[Conflict, ...] = field(default_factory=tuple)

    @property
    def duration(self) -> float:
        return self.t_end - self.t_start


@dataclass(frozen=True, slots=True)
class LayoutPlan:
    """整条时间轴的规划结果。"""

    actions: tuple[LayoutAction, ...]
    conflicts: tuple[Conflict, ...]
    manual_review_segments: tuple[LayoutAction, ...]

    def actions_for(self, kind: LayerKind) -> tuple[LayoutAction, ...]:
        return tuple(action for action in self.actions if action.kind == kind)

    @property
    def needs_human_confirmation(self) -> bool:
        return bool(self.manual_review_segments)


@dataclass(slots=True)
class _LayerState:
    """规划过程中单个层的运行时状态。"""

    kind: LayerKind
    current_rect: Rect | None = None
    current_zone: str | None = None
    moved_at: float = -1e9
    pending_rect: Rect | None = None
    pending_since: float | None = None


class LayoutPlanner:
    """逐帧采样驱动的布局规划器。"""

    def __init__(
        self,
        config: PlannerConfig | None = None,
        zones: tuple[Zone, ...] | None = None,
    ) -> None:
        self.config = config or PlannerConfig()
        self.zones = zones or build_default_zones()

    def plan(self, timeline: Timeline) -> LayoutPlan:
        samples = timeline.samples
        all_conflicts: list[Conflict] = []
        actions: list[LayoutAction] = []
        manual: list[LayoutAction] = []
        states = {
            LayerKind.SUBTITLE: _LayerState(LayerKind.SUBTITLE),
            LayerKind.SIGN_PIP: _LayerState(LayerKind.SIGN_PIP),
        }
        prev_shot: str | None = None

        for idx, sample in enumerate(samples):
            frame_conflicts = detect_conflicts(sample)
            all_conflicts.extend(frame_conflicts)
            shot_cut = prev_shot is not None and sample.shot_id != prev_shot
            occupied: list[Rect] = []

            for kind in (LayerKind.SUBTITLE, LayerKind.SIGN_PIP):
                observation = (
                    sample.subtitle
                    if kind == LayerKind.SUBTITLE
                    else sample.sign
                )
                state = states[kind]
                if observation is None or not observation.visible:
                    self._close_action(actions, state, kind, sample.t)
                    state.current_rect = None
                    state.current_zone = None
                    state.moved_at = -1e9
                    state.pending_rect = None
                    state.pending_since = None
                    continue

                no_semantics = not sample.subject.has_semantics
                choice = self._choose_placement(
                    observation, sample, occupied, state, shot_cut
                )
                if choice is None or no_semantics:
                    last_action = self._last_open(actions, kind)
                    if (
                        last_action is not None
                        and last_action.type == ActionType.MANUAL_REVIEW
                    ):
                        # 人工确认段持续期间不重复建段，等待人工裁决。
                        occupied.append(
                            choice.rect if choice is not None
                            else state.current_rect
                        )
                        continue
                    action = self._emit_manual(
                        sample, kind, observation, state,
                        frame_conflicts, choice, no_semantics
                    )
                    manual.append(action)
                    actions.append(action)
                    if choice is not None:
                        occupied.append(choice.rect)
                    elif state.current_rect is not None:
                        occupied.append(state.current_rect)
                    continue

                action = self._commit_choice(
                    sample, idx, samples, observation, state, choice,
                    frame_conflicts, actions, shot_cut
                )
                if action is not None:
                    occupied.append(action.target_rect)
                else:
                    occupied.append(state.current_rect)
            prev_shot = sample.shot_id

        self._finalize(actions, samples)
        return LayoutPlan(
            actions=tuple(actions),
            conflicts=tuple(all_conflicts),
            manual_review_segments=tuple(manual),
        )

    # ------------------------------------------------------------------
    # 候选与打分
    # ------------------------------------------------------------------

    def _choose_placement(
        self,
        observation: LayerObservation,
        sample: FrameSample,
        occupied: list[Rect],
        state: _LayerState,
        shot_cut: bool,
    ):
        avoid = subject_avoid_rect(
            sample.subject,
            self.config.subject_pad_x,
            self.config.subject_pad_y,
        )
        best = None
        for rank, zone in enumerate(candidate_zones(self.zones, observation.kind)):
            candidate = self._fit_in_zone(observation, zone)
            if candidate is None:
                continue
            if any(candidate.rect.intersects(rect) for rect in occupied):
                continue  # 层间硬约束：绝不与已摆放的可访问性层重叠
            cost = self._cost(
                candidate, zone, rank, avoid, state, observation
            )
            if best is None or cost < best.cost:
                best = _Placement(zone, candidate, cost)
        return best

    def _fit_in_zone(self, observation: LayerObservation, zone: Zone):
        fits = (
            observation.rect.width <= zone.rect.width + 1e-6
            and observation.rect.height <= zone.rect.height + 1e-6
        )
        if fits:
            width, height, scale = (
                observation.rect.width, observation.rect.height, 1.0
            )
        else:
            scale = min(
                zone.rect.width / observation.rect.width,
                zone.rect.height / observation.rect.height,
            )
            width = max(observation.min_width,
                        observation.rect.width * scale)
            height = max(observation.min_height,
                         observation.rect.height * scale)
        if width > zone.rect.width + 1e-6 or height > zone.rect.height + 1e-6:
            return None
        x, y = _anchor_align(zone.anchor, zone.rect, width, height)
        rect = Rect(x, y, width, height)
        resized = (
            abs(width - observation.rect.width) > 1e-6
            or abs(height - observation.rect.height) > 1e-6
        )
        return _Candidate(rect, resized, scale)

    def _cost(
        self,
        candidate: "_Candidate",
        zone: Zone,
        rank: int,
        avoid: Rect | None,
        state: _LayerState,
        observation: LayerObservation,
    ) -> float:
        cfg = self.config
        subject_penalty = 0.0
        if avoid is not None:
            overlap = candidate.rect.overlap_area(avoid)
            if overlap > 1e-9:
                # 硬偏好：存在主体遮挡的候选排在所有零遮挡候选之后，
                # 遮挡面积只用于在“都有遮挡”的候选间排序。
                subject_penalty = 1.0 + overlap
        preference_penalty = rank * cfg.preference_weight
        move_penalty = 0.0
        if state.current_rect is not None:
            move_penalty = (
                abs(candidate.rect.x - state.current_rect.x)
                + abs(candidate.rect.y - state.current_rect.y)
            )
        resize_penalty = (
            cfg.resize_weight if candidate.resized else 0.0
        )
        return (
            cfg.subject_weight * subject_penalty
            + preference_penalty
            + cfg.move_weight * move_penalty
            + resize_penalty
        )

    # ------------------------------------------------------------------
    # 提交、防抖与人工确认
    # ------------------------------------------------------------------

    def _commit_choice(
        self,
        sample: FrameSample,
        idx: int,
        samples: tuple[FrameSample, ...],
        observation: LayerObservation,
        state: _LayerState,
        choice: _Placement,
        frame_conflicts: tuple[Conflict, ...],
        actions: list[LayoutAction],
        shot_cut: bool,
    ) -> LayoutAction | None:
        target = choice.candidate.rect

        if state.current_rect is None:
            action = self._open_move(
                sample, observation, state, choice, frame_conflicts,
                reason="图层首次出现，按候选优先级落位",
            )
            actions.append(action)
            return action

        same_place = (
            state.current_zone == choice.zone.zone_id
            and _nearly_equal(target, state.current_rect)
        )
        if same_place:
            state.pending_rect = None
            state.pending_since = None
            return self._last_open(actions, observation.kind)

        # 切镜头：主体位置可能已整体变化，跳过防抖立即重排。
        if shot_cut:
            action = self._open_move(
                sample, observation, state, choice, frame_conflicts,
                reason=f"切镜头至 {sample.shot_id}，立即重新布局",
            )
            actions.append(action)
            return action

        # 防抖：新位置需稳定持续 debounce_window 才生效；
        # 未确认期间保持当前位置，避免画面来回跳。
        if state.pending_rect is None or not _nearly_equal(
            state.pending_rect, target
        ):
            state.pending_rect = target
            state.pending_since = sample.t
            return self._last_open(actions, observation.kind)

        elapsed = sample.t - (state.pending_since or sample.t)
        dwell = sample.t - state.moved_at
        if elapsed >= self.config.debounce_window and dwell >= self.config.min_dwell:
            reason = self._move_reason(observation, choice, frame_conflicts)
            action = self._open_move(
                sample, observation, state, choice, frame_conflicts, reason
            )
            actions.append(action)
            return action
        return self._last_open(actions, observation.kind)

    def _open_move(
        self,
        sample: FrameSample,
        observation: LayerObservation,
        state: _LayerState,
        choice: _Placement,
        frame_conflicts: tuple[Conflict, ...],
        reason: str,
    ) -> LayoutAction:
        action_type = (
            ActionType.MOVE_AND_RESIZE if choice.candidate.resized
            else ActionType.MOVE
        )
        action = LayoutAction(
            t_start=sample.t,
            t_end=sample.t,
            shot_id=sample.shot_id,
            kind=observation.kind,
            type=action_type,
            from_rect=state.current_rect,
            target_rect=choice.candidate.rect,
            zone_id=choice.zone.zone_id,
            reason=reason,
            human_confirmation_required=False,
            conflicts=tuple(
                conflict for conflict in frame_conflicts
                if observation.kind in conflict.parties
            ),
        )
        state.current_rect = choice.candidate.rect
        state.current_zone = choice.zone.zone_id
        state.moved_at = sample.t
        state.pending_rect = None
        state.pending_since = None
        return action

    def _emit_manual(
        self,
        sample: FrameSample,
        kind: LayerKind,
        observation: LayerObservation,
        state: _LayerState,
        frame_conflicts: tuple[Conflict, ...],
        choice: _Placement | None,
        no_semantics: bool,
    ) -> LayoutAction:
        if no_semantics:
            reason = (
                "无画面语义：无法确认主体位置，布局变更必须人工确认"
            )
        else:
            reason = (
                "所有候选区域均无法满足层间不重叠硬约束，"
                "需人工裁决（缩放/降级/遮挡取舍）"
            )
        fallback = choice.candidate.rect if choice is not None else state.current_rect
        action = LayoutAction(
            t_start=sample.t,
            t_end=sample.t,
            shot_id=sample.shot_id,
            kind=kind,
            type=ActionType.MANUAL_REVIEW,
            from_rect=state.current_rect,
            target_rect=fallback,
            zone_id=choice.zone.zone_id if choice is not None else state.current_zone,
            reason=reason,
            human_confirmation_required=True,
            conflicts=tuple(
                conflict for conflict in frame_conflicts
                if kind in conflict.parties
                or conflict.type == ConflictType.NO_SEMANTICS
            ),
        )
        if fallback is not None:
            state.current_rect = fallback
            state.moved_at = sample.t
        return action

    def _move_reason(
        self,
        observation: LayerObservation,
        choice: _Placement,
        frame_conflicts: tuple[Conflict, ...],
    ) -> str:
        related = [
            conflict for conflict in frame_conflicts
            if observation.kind in conflict.parties
        ]
        if any(conflict.type == ConflictType.THREE_WAY for conflict in related):
            return "检测到字幕/手语/主体三方冲突，迁移至低冲突区域"
        if any(
            conflict.type == ConflictType.LAYER_OVERLAP
            for conflict in related
        ):
            return "字幕与手语争抢同一区域，按优先级重新分配锚点"
        if any(
            conflict.type == ConflictType.SUBJECT_OCCLUSION
            for conflict in related
        ):
            return "当前位置遮挡画面主体，迁移至避让区外"
        return f"周期性重排至更优锚点 {choice.zone.anchor}"

    @staticmethod
    def _last_open(
        actions: list[LayoutAction], kind: LayerKind
    ) -> LayoutAction | None:
        for action in reversed(actions):
            if action.kind == kind:
                return action
        return None

    def _close_action(
        self,
        actions: list[LayoutAction],
        state: _LayerState,
        kind: LayerKind,
        t: float,
    ) -> None:
        last_idx = None
        for idx in range(len(actions) - 1, -1, -1):
            if actions[idx].kind == kind:
                last_idx = idx
                break
        if last_idx is not None and actions[last_idx].t_end < t:
            last = actions[last_idx]
            actions[last_idx] = LayoutAction(
                t_start=last.t_start,
                t_end=t,
                shot_id=last.shot_id,
                kind=last.kind,
                type=last.type,
                from_rect=last.from_rect,
                target_rect=last.target_rect,
                zone_id=last.zone_id,
                reason=last.reason,
                human_confirmation_required=last.human_confirmation_required,
                conflicts=last.conflicts,
            )
        state.pending_rect = None
        state.pending_since = None

    def _finalize(
        self, actions: list[LayoutAction],
        samples: tuple[FrameSample, ...],
    ) -> None:
        if not samples:
            return
        end_t = samples[-1].t
        # 每个层的动作按时间闭合：被后续动作取代的段在后续动作起点结束；
        # 已显式闭合（图层消失）的段保持不变；最后一段延伸到时间轴终点。
        for kind in (LayerKind.SUBTITLE, LayerKind.SIGN_PIP):
            indices = [
                idx for idx, action in enumerate(actions)
                if action.kind == kind
            ]
            for order, idx in enumerate(indices):
                action = actions[idx]
                if action.t_end > action.t_start:
                    continue
                if order + 1 < len(indices):
                    next_t = actions[indices[order + 1]].t_start
                else:
                    next_t = end_t
                actions[idx] = LayoutAction(
                    t_start=action.t_start,
                    t_end=max(next_t, action.t_start),
                    shot_id=action.shot_id,
                    kind=action.kind,
                    type=action.type,
                    from_rect=action.from_rect,
                    target_rect=action.target_rect,
                    zone_id=action.zone_id,
                    reason=action.reason,
                    human_confirmation_required=action.human_confirmation_required,
                    conflicts=action.conflicts,
                )


def _nearly_equal(a: Rect | None, b: Rect | None, tol: float = 1e-6) -> bool:
    if a is None or b is None:
        return a is b
    return (
        abs(a.x - b.x) <= tol and abs(a.y - b.y) <= tol
        and abs(a.width - b.width) <= tol
        and abs(a.height - b.height) <= tol
    )


def _anchor_align(
    anchor: str, zone_rect: Rect, width: float, height: float
) -> tuple[float, float]:
    """按锚点语义把图层贴到区域的对应边/角，剩余空间居中。"""

    col, row = anchor.split("_")
    if col == "left":
        x = zone_rect.x
    elif col == "right":
        x = zone_rect.right - width
    else:
        x = zone_rect.x + (zone_rect.width - width) / 2.0
    if row == "top":
        y = zone_rect.y
    elif row == "bottom":
        y = zone_rect.bottom - height
    else:
        y = zone_rect.y + (zone_rect.height - height) / 2.0
    return x, y


@dataclass(frozen=True, slots=True)
class _Candidate:
    rect: Rect
    resized: bool
    scale: float


@dataclass(frozen=True, slots=True)
class _Placement:
    zone: Zone
    candidate: _Candidate
    cost: float

    @property
    def rect(self) -> Rect:
        return self.candidate.rect
