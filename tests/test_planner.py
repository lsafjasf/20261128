import unittest

from accessibility_layout.geometry import Rect
from accessibility_layout.layers import LayerKind, Timeline
from accessibility_layout.planner import (
    ActionType,
    LayoutPlanner,
    PlannerConfig,
)
from accessibility_layout.zones import Zone

from tests.helpers import frame, no_semantics, sign, subject, subtitle


def make_planner(**config) -> LayoutPlanner:
    return LayoutPlanner(PlannerConfig(**config))


class InitialPlacementTest(unittest.TestCase):
    def test_layers_land_in_distinct_preferred_zones(self) -> None:
        planner = make_planner()
        plan = planner.plan(Timeline((
            frame(
                t=0.0,
                subject_obs=subject(0.3, 0.08, 0.3, 0.35),
                subtitle_obs=subtitle(),
                sign_obs=sign(),
            ),
        )))
        sub_action = plan.actions_for(LayerKind.SUBTITLE)[0]
        sign_action = plan.actions_for(LayerKind.SIGN_PIP)[0]
        self.assertEqual(sub_action.zone_id, "zone_center_bottom")
        self.assertEqual(sign_action.zone_id, "zone_right_bottom")
        self.assertFalse(
            sub_action.target_rect.intersects(sign_action.target_rect)
        )

    def test_subtitle_wins_preferred_zone_by_priority(self) -> None:
        planner = make_planner()
        plan = planner.plan(Timeline((
            frame(t=0.0, subtitle_obs=subtitle(), sign_obs=sign()),
        )))
        sub_action = plan.actions_for(LayerKind.SUBTITLE)[0]
        sign_action = plan.actions_for(LayerKind.SIGN_PIP)[0]
        self.assertEqual(sub_action.zone_id, "zone_center_bottom")
        self.assertNotEqual(sign_action.zone_id, sub_action.zone_id)


class ConflictResolutionTest(unittest.TestCase):
    def _timeline_subject_invades_bottom(self, cut: bool):
        top_subject = subject(0.3, 0.08, 0.3, 0.3)
        bottom_subject = subject(0.05, 0.7, 0.9, 0.27)
        return Timeline((
            frame(0.0, "s1", top_subject, subtitle(), sign()),
            frame(0.5, "s1", top_subject, subtitle(), sign()),
            frame(1.0, "s1", top_subject, subtitle(), sign()),
            frame(
                1.5, "s2" if cut else "s1", bottom_subject,
                subtitle(0.2, 0.8, 0.5, 0.14), sign(0.6, 0.76, 0.25, 0.22)
            ),
            frame(
                2.0, "s2" if cut else "s1", bottom_subject,
                subtitle(0.2, 0.8, 0.5, 0.14), sign(0.6, 0.76, 0.25, 0.22)
            ),
            frame(
                2.5, "s2" if cut else "s1", bottom_subject,
                subtitle(0.2, 0.8, 0.5, 0.14), sign(0.6, 0.76, 0.25, 0.22)
            ),
        ))

    def test_conflict_move_waits_for_debounce_and_dwell(self) -> None:
        planner = make_planner()
        plan = planner.plan(self._timeline_subject_invades_bottom(cut=False))
        moves = [
            a for a in plan.actions
            if a.type in (ActionType.MOVE, ActionType.MOVE_AND_RESIZE)
        ]
        later_moves = [a for a in moves if a.t_start >= 1.5]
        self.assertTrue(later_moves)
        # 1.5s 才出现新冲突，防抖 0.4s => 最早 2.0s 生效，绝不在 1.5s 跳变。
        self.assertGreaterEqual(min(a.t_start for a in later_moves), 2.0)

        avoid = Rect.from_xyxy(0.03, 0.68, 0.97, 0.99)
        for action in later_moves:
            self.assertEqual(
                action.target_rect.overlap_area(avoid), 0.0,
                f"{action.kind} 迁移后仍压住主体避让区",
            )

    def test_shot_cut_triggers_immediate_relayout(self) -> None:
        planner = make_planner()
        plan = planner.plan(self._timeline_subject_invades_bottom(cut=True))
        cut_moves = [
            a for a in plan.actions
            if a.type in (ActionType.MOVE, ActionType.MOVE_AND_RESIZE)
            and a.t_start == 1.5
        ]
        self.assertTrue(cut_moves, "切镜头后必须立即重排，不等防抖窗口")

    def test_transient_conflict_does_not_move_layout(self) -> None:
        top_subject = subject(0.3, 0.08, 0.3, 0.3)
        bottom_subject = subject(0.05, 0.7, 0.9, 0.27)
        timeline = Timeline((
            frame(0.0, "s1", top_subject, subtitle(), sign()),
            frame(0.5, "s1", top_subject, subtitle(), sign()),
            frame(1.0, "s1", top_subject, subtitle(), sign()),
            frame(1.5, "s1", bottom_subject, subtitle(), sign()),
            frame(2.0, "s1", top_subject, subtitle(), sign()),
            frame(2.5, "s1", top_subject, subtitle(), sign()),
        ))
        plan = make_planner().plan(timeline)
        moves = [
            a for a in plan.actions
            if a.type in (ActionType.MOVE, ActionType.MOVE_AND_RESIZE)
        ]
        self.assertEqual(
            len(moves), 2, "瞬时（单帧）冲突不应触发布局迁移"
        )


class HumanConfirmationTest(unittest.TestCase):
    def test_missing_semantics_marks_manual_review(self) -> None:
        timeline = Timeline((
            frame(0.0, "s1", subject(), subtitle(), sign()),
            frame(0.5, "s1", no_semantics(), subtitle(), sign()),
            frame(1.0, "s1", no_semantics(), subtitle(), sign()),
        ))
        plan = make_planner().plan(timeline)
        self.assertTrue(plan.needs_human_confirmation)
        for action in plan.manual_review_segments:
            self.assertEqual(action.type, ActionType.MANUAL_REVIEW)
            self.assertTrue(action.human_confirmation_required)
            self.assertGreaterEqual(action.t_end, action.t_start)

    def test_no_legal_placement_escalates(self) -> None:
        tiny_zone = Zone(
            zone_id="zone_tiny",
            anchor="center_middle",
            rect=Rect(0.4, 0.4, 0.05, 0.05),
            preference=0,
            allowed=frozenset({LayerKind.SUBTITLE, LayerKind.SIGN_PIP}),
        )
        planner = LayoutPlanner(
            PlannerConfig(), zones=(tiny_zone,)
        )
        timeline = Timeline((
            frame(t=0.0, subtitle_obs=subtitle(), sign_obs=sign()),
        ))
        plan = planner.plan(timeline)
        self.assertTrue(plan.needs_human_confirmation)
        self.assertEqual(
            len(plan.manual_review_segments), 2
        )


class SegmentAndInvariantTest(unittest.TestCase):
    def test_action_segments_have_durations(self) -> None:
        timeline = Timeline((
            frame(0.0, "s1", subject(), subtitle(), sign()),
            frame(0.5, "s1", subject(), subtitle(), sign()),
            frame(1.0, "s1", subject(), subtitle(), None),
        ))
        plan = make_planner().plan(timeline)
        for action in plan.actions:
            self.assertGreater(action.duration, 0.0)

    def test_layers_never_overlap_across_timeline(self) -> None:
        bottom_subject = subject(0.05, 0.72, 0.9, 0.25)
        timeline = Timeline((
            frame(0.0, "s1", subject(), subtitle(), sign()),
            frame(0.5, "s1", subject(), subtitle(), sign()),
            frame(1.0, "s2", bottom_subject, subtitle(), sign()),
            frame(1.5, "s2", bottom_subject, subtitle(), sign()),
            frame(2.0, "s2", bottom_subject, subtitle(), sign()),
        ))
        plan = make_planner().plan(timeline)
        for sample in timeline.samples:
            active = [
                action for action in plan.actions
                if action.t_start <= sample.t <= action.t_end
                and action.target_rect is not None
            ]
            rects = [
                action.target_rect for action in active
            ]
            for i, left in enumerate(rects):
                for right in rects[i + 1:]:
                    self.assertFalse(
                        left.intersects(right),
                        f"t={sample.t} 图层间发生重叠",
                    )

    def test_empty_timeline_produces_empty_plan(self) -> None:
        plan = make_planner().plan(Timeline(()))
        self.assertEqual(plan.actions, ())
        self.assertEqual(plan.conflicts, ())
        self.assertFalse(plan.needs_human_confirmation)

    def test_layer_reappears_as_fresh_placement(self) -> None:
        timeline = Timeline((
            frame(0.0, "s1", subject(), subtitle(), sign()),
            frame(0.5, "s1", subject(), subtitle(), None),
            frame(1.0, "s1", subject(), subtitle(), sign()),
            frame(1.5, "s1", subject(), subtitle(), sign()),
        ))
        plan = make_planner().plan(timeline)
        sign_actions = plan.actions_for(LayerKind.SIGN_PIP)
        self.assertEqual(len(sign_actions), 2)
        # 消失时第一段在 0.5s 闭合；重现产生第二段，两段均有正持续时间。
        self.assertAlmostEqual(sign_actions[0].t_end, 0.5)
        self.assertGreater(sign_actions[1].t_start, sign_actions[0].t_end)
        for action in sign_actions:
            self.assertGreater(action.duration, 0.0)

    def test_manual_review_blocks_auto_apply(self) -> None:
        timeline = Timeline((
            frame(0.0, "s1", subject(), subtitle(), sign()),
            frame(0.5, "s1", no_semantics(), subtitle(), sign()),
        ))
        plan = make_planner().plan(timeline)
        for action in plan.manual_review_segments:
            self.assertTrue(action.human_confirmation_required)
            self.assertEqual(action.type, ActionType.MANUAL_REVIEW)


if __name__ == "__main__":
    unittest.main()
