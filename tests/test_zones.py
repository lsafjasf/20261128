import unittest

from accessibility_layout.layers import LayerKind
from accessibility_layout.zones import (
    build_default_zones,
    candidate_zones,
    subject_avoid_rect,
)

from tests.helpers import no_semantics, subject


class ZonesTest(unittest.TestCase):
    def test_nine_safe_zones(self) -> None:
        zones = build_default_zones()
        self.assertEqual(len(zones), 9)
        for zone in zones:
            self.assertGreaterEqual(zone.rect.x, 0.0)
            self.assertLessEqual(zone.rect.bottom, 1.0)
            self.assertTrue(zone.accepts(LayerKind.SUBTITLE))
            self.assertTrue(zone.accepts(LayerKind.SIGN_PIP))
            self.assertFalse(zone.accepts(LayerKind.SUBJECT))

    def test_candidate_priority_differs_between_layers(self) -> None:
        zones = build_default_zones()
        sub = candidate_zones(zones, LayerKind.SUBTITLE)
        sign = candidate_zones(zones, LayerKind.SIGN_PIP)
        self.assertEqual(sub[0].anchor, "center_bottom")
        self.assertEqual(sign[0].anchor, "right_bottom")
        self.assertEqual(len(sub), 9)
        self.assertEqual(len(sign), 9)

    def test_subject_avoid_rect_padding(self) -> None:
        avoid = subject_avoid_rect(subject(0.3, 0.3, 0.2, 0.2), 0.05, 0.05)
        self.assertIsNotNone(avoid)
        self.assertAlmostEqual(avoid.x, 0.25)
        self.assertAlmostEqual(avoid.bottom, 0.55)

    def test_no_semantics_returns_none_avoid(self) -> None:
        self.assertIsNone(subject_avoid_rect(no_semantics()))


if __name__ == "__main__":
    unittest.main()
