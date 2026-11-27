# renderpipe

`renderpipe` 是一个单模块 Go 渲染器，用内容寻址方式生成稳定的任务 ID、源哈希、帧文件名和媒体哈希。任务标识和产物路径不读取本地时间、时区、文件 mtime、文件所有者或权限，因此同一工程在不同时区的服务器上得到相同的相对目录和媒体内容。

项目只使用 Go 标准库，无外部依赖。

## 工程格式

最小工程只需要 `frames/` 目录：

```text
project/
  frames/
    shot-1.png
    shot-2.png
    shot-3.png
```

也可以放置 `project.json`：

```json
{
  "version": 1,
  "fps": 12,
  "frame_dir": "frames",
  "frames": ["frames/shot-2.png", "frames/shot-1.png"]
}
```

- `frames` 省略时，扫描 `frame_dir` 下的 `.png`、`.jpg`、`.jpeg`、`.gif`，按路径的自然顺序排序。例如 `frame-2.png` 排在 `frame-10.png` 前。
- `frames` 存在时，严格按声明顺序渲染，便于显式控制镜头顺序；重复路径会被拒绝。
- `fps` 默认为 `10`，允许范围是 `1..1000`。
- 所有帧必须具有相同尺寸。输出帧统一编码为 PNG，媒体输出为 GIF。

## 确定性规则

### 源哈希

`source_sha256` 的输入为：

```text
"renderpipe/source/v1"
按规范化相对路径字典序排序后的每个普通文件：
    路径、文件内容 SHA-256、文件字节数
```

其中每个字段使用 64 位大端长度前缀，避免路径拼接歧义。路径统一使用 `/`。源哈希不包含绝对路径、mtime、ctime、所有者、权限、设备号或本地时区。

固定忽略目录 `.git`、`.hg`、`.svn`、`.renderpipe`；当输出目录位于工程内时，整个输出目录也会从源哈希和帧扫描中排除。源工程中的符号链接会被拒绝，以避免不同主机解析出不同内容。

### 配置哈希与任务 ID

`config_sha256` 覆盖渲染器版本、`fps`、帧命名方案、媒体文件名、帧数量和规范化后的帧路径及其顺序。

```text
task_id = "rt1-" + SHA-256(
    "renderpipe/task/v1",
    source_sha256,
    config_sha256
)
```

`task_id` 是 `rt1-` 加 64 位小写十六进制摘要，不包含时间戳。

### 输出目录和文件名

```text
<output>/<task_id>/
  frames/
    frame-000001.png
    frame-000002.png
    ...
  animation.gif
  manifest.json
```

序号从 1 开始。默认至少 6 位零填充；帧数达到 7 位时自动扩展宽度。`frame_set_sha256` 按输出帧名、帧哈希和大小计算，`media_sha256` 是 `animation.gif` 的 SHA-256。`manifest.json` 不含时间字段或主机绝对路径。

## 成功产物保护

渲染先写入输出目录下的临时 staging 目录。全部帧、媒体和最后写入的 `manifest.json` 都完成后，才通过原子目录重命名发布到 `<output>/<task_id>`。

- 相同任务已存在且清单、帧和媒体哈希全部校验通过时，直接复用，不重写任何文件。
- 已存在路径不完整、内容损坏或不是本任务时，返回 `ErrArtifactConflict`，不会删除、覆盖或修复它。
- 没有“隐式强制覆盖”开关。新源码会得到新任务 ID 和新目录，旧成功产物保持不变。
- 发布前会重新计算源哈希；渲染期间源码发生变化时返回 `ErrSourceChanged`。

## CLI

```bash
# 只计算任务，不写文件
go run ./cmd/renderpipe plan --project ./project --output ./renders

# 渲染或安全复用
go run ./cmd/renderpipe render --project ./project --output ./renders

# 校验已有产物的清单与全部文件哈希
go run ./cmd/renderpipe verify ./renders/rt1-...
```

`render` 和 `plan` 支持 `--json`。主要字段：

```text
task_id
source_sha256
config_sha256
frame_set_sha256
media_sha256
artifact
media
```

## 测试

```bash
go test ./...
```

测试覆盖不同 `TZ` 下的媒体字节一致性、mtime 不参与哈希、自然帧排序、显式帧顺序、嵌套输出、成功产物复用、损坏产物拒绝覆盖、不完整目录保留以及源变更后发布新目录。
