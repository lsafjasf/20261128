"""归一化坐标系下的矩形几何。

所有坐标均为相对视频画幅的归一化值，原点 (0, 0) 在左上角，
x 向右、y 向下，取值范围 [0.0, 1.0]。这样同一套规则可复用于
不同分辨率（1080p / 4K / 竖屏）。
"""

from __future__ import annotations

from dataclasses import dataclass

EPS = 1e-6


@dataclass(frozen=True, slots=True)
class Rect:
    """归一化轴对齐矩形。"""

    x: float
    y: float
    width: float
    height: float

    def __post_init__(self) -> None:
        if self.width < -EPS or self.height < -EPS:
            raise ValueError(f"矩形宽高不能为负: {self}")
        for name, value in (("x", self.x), ("y", self.y),
                            ("width", self.width), ("height", self.height)):
            if not -EPS <= value <= 1.0 + EPS:
                raise ValueError(f"{name}={value} 超出归一化范围 [0, 1]")

    @classmethod
    def from_xyxy(cls, x1: float, y1: float, x2: float, y2: float) -> "Rect":
        return cls(x1, y1, max(0.0, x2 - x1), max(0.0, y2 - y1))

    @property
    def right(self) -> float:
        return self.x + self.width

    @property
    def bottom(self) -> float:
        return self.y + self.height

    @property
    def area(self) -> float:
        return self.width * self.height

    def intersection(self, other: "Rect") -> "Rect | None":
        x1 = max(self.x, other.x)
        y1 = max(self.y, other.y)
        x2 = min(self.right, other.right)
        y2 = min(self.bottom, other.bottom)
        if x2 <= x1 + EPS or y2 <= y1 + EPS:
            return None
        return Rect.from_xyxy(x1, y1, x2, y2)

    def intersects(self, other: "Rect") -> bool:
        return self.intersection(other) is not None

    def overlap_area(self, other: "Rect") -> float:
        hit = self.intersection(other)
        return 0.0 if hit is None else hit.area

    def intersection_over_union(self, other: "Rect") -> float:
        if not self.intersects(other):
            return 0.0
        union = self.area + other.area - self.overlap_area(other)
        return self.overlap_area(other) / union if union > EPS else 0.0

    def contains_point(self, px: float, py: float) -> bool:
        return (self.x - EPS <= px <= self.right + EPS
                and self.y - EPS <= py <= self.bottom + EPS)

    def expanded(self, margin_x: float, margin_y: float | None = None) -> "Rect":
        """按比例向外扩张，并裁剪到画幅 [0,1] 内。"""

        margin_y = margin_x if margin_y is None else margin_y
        return Rect.from_xyxy(
            max(0.0, self.x - margin_x),
            max(0.0, self.y - margin_y),
            min(1.0, self.right + margin_x),
            min(1.0, self.bottom + margin_y),
        )

    def moved_to(self, x: float, y: float) -> "Rect":
        return Rect(x, y, self.width, self.height)

    def resized(self, width: float, height: float) -> "Rect":
        return Rect(self.x, self.y, width, height)

    def clamp_to_frame(self) -> "Rect":
        x = min(max(0.0, self.x), max(0.0, 1.0 - self.width))
        y = min(max(0.0, self.y), max(0.0, 1.0 - self.height))
        return Rect(x, y, self.width, self.height)


def clamp01(value: float) -> float:
    return min(1.0, max(0.0, value))
