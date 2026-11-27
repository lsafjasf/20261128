import unittest

from accessibility_layout.conflict import (
    ConflictType,
    detect_conflicts,
    detect_timeline,
)
from accessibility_layout.layers import LayerKind

from tests.helpers import frame, no_semantics, sign, subject, subtitle


class ConflictDetectionTest(unittest.TestCase):
    def test_layer_overlap_detected(self) -> None:
        sample = frame(
            subtitle_obs=subtitle(0.25, 0.8, 0.5, 0.12),
            sign_obs=sign(0.6, 0.78, 0.25, 0.2),
        )
        conflicts = detect_conflicts(sample)
        types = {conflict.type for conflict in conflicts}
        self.assertIn(ConflictType.LAYER_OVERLAP, types)
        overlap = next(
            c for c in conflicts if c.type == ConflictType.LAYER_OVERLAP
        )
        self.assertEqual(
            set(overlap.parties),
            {LayerKind.SUBTITLE, LayerKind.SIGN_PIP},
        )
        self.assertGreater(overlap.severity, 0.0)

    def test_subject_occlusion_detected(self) -> None:
        sample = frame(
            subject_obs=subject(0.2, 0.7, 0.4, 0.25),
            sign_obs=sign(0.25, 0.72, 0.22, 0.22),
        )
        conflicts = detect_conflicts(sample)
        types = {conflict.type for conflict in conflicts}
        self.assertIn(ConflictType.SUBJECT_OCCLUSION, types)

    def test_three_way_conflict(self) -> None:
        # 字幕与手语在底部交叠，且主体恰好延伸到同一片区域。
        sample = frame(
            subject_obs=subject(0.2, 0.7, 0.6, 0.25),
            subtitle_obs=subtitle(0.2, 0.78, 0.5, 0.12),
            sign_obs=sign(0.55, 0.74, 0.25, 0.2),
        )
        conflicts = detect_conflicts(sample)
        types = {conflict.type for conflict in conflicts}
        self.assertIn(ConflictType.THREE_WAY, types)
        three_way = next(
            c for c in conflicts if c.type == ConflictType.THREE_WAY
        )
        self.assertEqual(
            set(three_way.parties),
            {LayerKind.SUBTITLE, LayerKind.SIGN_PIP, LayerKind.SUBJECT},
        )

    def test_clean_layout_has_no_conflict(self) -> None:
        sample = frame(
            subject_obs=subject(0.3, 0.1, 0.3, 0.4),
            subtitle_obs=subtitle(0.25, 0.88, 0.5, 0.08),
            sign_obs=sign(0.74, 0.08, 0.2, 0.22),
        )
        self.assertEqual(detect_conflicts(sample), ())

    def test_no_semantics_forces_human_confirmation(self) -> None:
        sample = frame(
            subject_obs=no_semantics(),
            subtitle_obs=subtitle(),
            sign_obs=sign(),
        )
        conflicts = detect_conflicts(sample)
        no_sem = [
            c for c in conflicts if c.type == ConflictType.NO_SEMANTICS
        ]
        self.assertEqual(len(no_sem), 1)
        self.assertTrue(no_sem[0].needs_human_confirmation)

    def test_detect_timeline_aggregates(self) -> None:
        timeline = (
            frame(t=0.0, subtitle_obs=subtitle(), sign_obs=sign()),
            frame(
                t=1.0,
                subject_obs=no_semantics(),
                subtitle_obs=subtitle(),
                sign_obs=sign(),
            ),
        )
        conflicts = detect_timeline(timeline)
        self.assertTrue(
            any(c.type == ConflictType.NO_SEMANTICS for c in conflicts)
        )


if __name__ == "__main__":
    unittest.main()
