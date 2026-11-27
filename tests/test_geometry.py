import unittest

from accessibility_layout.geometry import Rect


class RectTest(unittest.TestCase):
    def test_from_xyxy_and_edges(self) -> None:
        rect = Rect.from_xyxy(0.1, 0.2, 0.4, 0.6)
        self.assertAlmostEqual(rect.x, 0.1)
        self.assertAlmostEqual(rect.y, 0.2)
        self.assertAlmostEqual(rect.width, 0.3)
        self.assertAlmostEqual(rect.height, 0.4)
        self.assertAlmostEqual(rect.right, 0.4)
        self.assertAlmostEqual(rect.bottom, 0.6)
        self.assertAlmostEqual(rect.area, 0.12)

    def test_invalid_rect_rejected(self) -> None:
        with self.assertRaises(ValueError):
            Rect(0.1, 0.1, -0.2, 0.3)
        with self.assertRaises(ValueError):
            Rect(1.2, 0.0, 0.1, 0.1)

    def test_intersection_and_overlap(self) -> None:
        a = Rect(0.0, 0.0, 0.5, 0.5)
        b = Rect(0.25, 0.25, 0.5, 0.5)
        hit = a.intersection(b)
        self.assertIsNotNone(hit)
        self.assertAlmostEqual(hit.area, 0.0625)
        self.assertTrue(a.intersects(b))
        self.assertAlmostEqual(a.overlap_area(b), 0.0625)

        disjoint = Rect(0.6, 0.6, 0.2, 0.2)
        self.assertIsNone(a.intersection(disjoint))
        self.assertFalse(a.intersects(disjoint))

    def test_iou(self) -> None:
        a = Rect(0.0, 0.0, 0.4, 0.4)
        identical = Rect(0.0, 0.0, 0.4, 0.4)
        self.assertAlmostEqual(a.intersection_over_union(identical), 1.0)
        far = Rect(0.8, 0.8, 0.1, 0.1)
        self.assertEqual(a.intersection_over_union(far), 0.0)

    def test_expanded_clamped(self) -> None:
        rect = Rect(0.0, 0.0, 0.2, 0.2)
        grown = rect.expanded(0.1)
        self.assertEqual((grown.x, grown.y), (0.0, 0.0))
        self.assertAlmostEqual(grown.right, 0.3)
        self.assertAlmostEqual(grown.bottom, 0.3)

    def test_moved_and_clamp(self) -> None:
        rect = Rect(0.0, 0.0, 0.3, 0.3)
        moved = rect.moved_to(0.9, 0.9).clamp_to_frame()
        self.assertAlmostEqual(moved.x, 0.7)
        self.assertAlmostEqual(moved.y, 0.7)

    def test_contains_point(self) -> None:
        rect = Rect(0.2, 0.2, 0.3, 0.3)
        self.assertTrue(rect.contains_point(0.35, 0.35))
        self.assertFalse(rect.contains_point(0.9, 0.9))


if __name__ == "__main__":
    unittest.main()
