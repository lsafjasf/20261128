# 可访问性层布局引擎

解决手语画中画（SIGN_PIP）与底部字幕（SUBTITLE）争抢同一区域、
切镜头后互相遮挡、并误伤画面主体（SUBJECT）的问题。输出渲染侧
可直接执行的布局动作；无画面语义时强制转人工确认，不做猜测性重排。

## 分层模型

| 层 | 类型 | 优先级 | 约束 |
| --- | --- | --- | --- |
| 字幕 | `LayerKind.SUBTITLE` | 2（最高） | 必须可读，优先底部居中 |
| 手语画中画 | `LayerKind.SIGN_PIP` | 1 | 手势可见，优先右下角 |
| 画面主体 | `LayerKind.SUBJECT` | 0（避让方） | 由前两者选位避开 |

高优先级层先落位，低优先级层不得与其重叠；层间不重叠是硬约束，
任何成本权重都不能违反。

## 区域（Zone）

安全区（默认边距 x=4%、y=5%）内划分 3×3 共 9 个锚点区域，
每层带独立候选优先级序列，刻意错开首选锚点：

- 字幕：`center_bottom → left_bottom → right_bottom → ...`
- 手语：`right_bottom → left_bottom → right_middle → ...`

区域放得下图层时保持原始尺寸；放不下时按比例缩放到不小于
`min_width/min_height`，仍放不下则该区域不可用。

## 冲突检测（三方）

`conflict.detect_conflicts` 在每个采样点输出：

- `LAYER_OVERLAP`：字幕与手语互相遮挡。
- `SUBJECT_OCCLUSION`：可访问性层与主体避让区（检测框 + 2% 呼吸边距）交叠。
- `THREE_WAY`：字幕、手语、主体在同一交集区域冲突。
- `NO_SEMANTICS`：主体检测缺测或置信度失效，`needs_human_confirmation=True`。

注意：`has_semantics=False` 不等于“画面无主体”，禁止据此自动重排。

## 持续时间与防抖

`PlannerConfig` 控制时间行为（单位秒）：

- `min_dwell=1.5`：每次布局迁移后的最小驻留时间，防止频繁跳动。
- `debounce_window=0.4`：新位置需稳定持续该时长才生效；瞬时冲突被吞掉。
- 切镜头（`shot_id` 变化）跳过防抖与驻留限制，立即重新评估。

## 可执行动作

`LayoutAction` 字段即渲染侧指令：

- `type`：`MOVE` / `RESIZE` / `MOVE_AND_RESIZE` / `HOLD` / `MANUAL_REVIEW`
- `target_rect`：归一化目标矩形（0~1，任意分辨率可换算像素）。
- `t_start/t_end`：生效区间；`duration` 为持续时间。
- `zone_id`：目标锚点区域。
- `human_confirmation_required`：为真时指令不得自动生效。

人工确认触发条件：

1. 当前帧无画面语义（检测缺测/置信度过低）。
2. 所有候选区域都无法满足层间不重叠硬约束。

## 使用方式

```python
from accessibility_layout import (
    FrameSample, LayoutPlanner, PlannerConfig, Timeline,
)

timeline = Timeline((frame_0, frame_1, ...))
plan = LayoutPlanner(PlannerConfig()).plan(timeline)
for action in plan.actions:
    if action.human_confirmation_required:
        queue_for_human_review(action)
    else:
        renderer.apply(action.target_rect, action.t_start, action.t_end)
```

运行演示：`python3 demo.py`
运行测试：`python3 -m unittest discover -s tests`
