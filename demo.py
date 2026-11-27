"""端到端演示：手语画中画与底部字幕争抢区域、切镜头后的布局重排。

运行：python3 demo.py

输出包含：
1. 每个采样点检测到的冲突（层间遮挡 / 主体遮挡 / 三方冲突 / 无语义）。
2. 规划器生成的可执行布局动作（MOVE / RESIZE / MANUAL_REVIEW），
   含目标归一化矩形、所属区域、持续时间与触发原因。
3. 无画面语义时段强制人工确认的标注。
"""

from __future__ import annotations

from accessibility_layout.conflict import ConflictType, detect_conflicts
from accessibility_layout.layers import LayerKind, Timeline
from accessibility_layout.planner import ActionType, LayoutPlanner, PlannerConfig
from tests.helpers import frame, no_semantics, sign, subject, subtitle


def build_timeline() -> Timeline:
    talking_head = subject(0.32, 0.06, 0.32, 0.42)
    wide_shot_subject = subject(0.05, 0.68, 0.85, 0.28)
    return Timeline((
        # 镜头 A：主播中景，字幕与手语初始落位互不遮挡。
        frame(0.0, "shot_a", talking_head, subtitle(), sign()),
        frame(0.5, "shot_a", talking_head, subtitle(), sign()),
        frame(1.0, "shot_a", talking_head, subtitle(), sign()),
        # 镜头 B：切到宽景，主体下移到底部，三方在底部区域冲突。
        frame(
            1.5, "shot_b", wide_shot_subject,
            subtitle(0.18, 0.8, 0.52, 0.14),
            sign(0.62, 0.74, 0.26, 0.22),
        ),
        frame(
            2.0, "shot_b", wide_shot_subject,
            subtitle(0.18, 0.8, 0.52, 0.14),
            sign(0.62, 0.74, 0.26, 0.22),
        ),
        # 镜头 C：画面语义丢失（检测器缺测），必须人工确认。
        frame(2.5, "shot_c", no_semantics(), subtitle(), sign()),
        frame(3.0, "shot_c", no_semantics(), subtitle(), sign()),
        # 语义恢复，回到中景。
        frame(3.5, "shot_d", talking_head, subtitle(), sign()),
        frame(4.0, "shot_d", talking_head, subtitle(), sign()),
    ))


def print_conflicts(timeline: Timeline) -> None:
    print("=== 冲突检测 ===")
    for sample in timeline.samples:
        conflicts = detect_conflicts(sample)
        if not conflicts:
            print(f"t={sample.t:>4.1f}s [{sample.shot_id}] 无冲突")
            continue
        for conflict in conflicts:
            human = " [需人工确认]" if conflict.needs_human_confirmation else ""
            print(
                f"t={sample.t:>4.1f}s [{sample.shot_id}] "
                f"{conflict.type.value:18s} "
                f"severity={conflict.severity:.2f}{human} "
                f"parties={[p.name for p in conflict.parties]}"
            )


def print_plan(plan) -> None:
    print("\n=== 可执行布局动作 ===")
    for action in plan.actions:
        target = action.target_rect
        rect_text = (
            f"x={target.x:.3f} y={target.y:.3f} "
            f"w={target.width:.3f} h={target.height:.3f}"
            if target is not None else "无自动目标"
        )
        print(
            f"{action.t_start:>4.1f}-{action.t_end:<4.1f}s "
            f"{action.kind.name:9s} {action.type.value:15s} "
            f"{str(action.zone_id):18s} {rect_text}"
        )
        print(f"           原因: {action.reason}")
        if action.human_confirmation_required:
            print("           >>> 人工确认前不得自动生效 <<<")

    print("\n=== 持续时间约束 ===")
    for action in plan.actions:
        print(
            f"{action.kind.name:9s} {action.type.value:15s} "
            f"duration={action.duration:.2f}s"
        )


def main() -> None:
    timeline = build_timeline()
    print_conflicts(timeline)
    planner = LayoutPlanner(PlannerConfig(min_dwell=1.5, debounce_window=0.4))
    plan = planner.plan(timeline)
    print_plan(plan)

    print("\n=== 结论 ===")
    if plan.needs_human_confirmation:
        print(
            f"共 {len(plan.manual_review_segments)} 个时段需要人工确认；"
            "其余冲突已自动生成迁移/缩放动作。"
        )
    else:
        print("全部冲突均可自动消解。")


if __name__ == "__main__":
    main()
