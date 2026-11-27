# releaseaudit

纯 Go 的三版本发行包矩阵与引用校验器。它把影院、流媒体、移动端成片放进同一张矩阵，检查画幅、音频布局、字幕、发行窗口是否齐全，并阻止相似文件名造成的串版。

## 核心规则

- 默认要求恰好存在 `theatrical`、`streaming`、`mobile` 三个版本；缺少或重复都会报错。
- 版本可从 `id`、`label` 或 `window.channel` 识别，支持 `cinema`、`ott`、`phone`、`影院`、`流媒体`、`移动端` 等别名。
- 资源路径中的版本词也会被识别。例如移动端版本引用 `theatrical/...`，即使文件名只差一个词，也会报 `cross_version_reference`。
- 同一资源路径被两个版本声明，报 `asset_shared_across_versions`。
- 画幅会与 `width`/`height` 交叉核对；音频布局会与 `channels` 交叉核对。
- 发行窗口必须可解析、起止顺序正确，且窗口渠道不能与版本类型冲突。
- 版本私有 manifest 只能被该版本声明；跨版本共用的 manifest 必须放进 `sharedManifests`。
- 共享 manifest 必须为每个版本提供目标，且每个目标的引用必须能证明属于该版本。JSON 共享 manifest 的实际内容还会与声明目标逐项比对。

## 快速开始

```sh
go run ./cmd/release-audit examples/release-package/release.json
go run ./cmd/release-audit -format json examples/release-package/release.json
go run ./cmd/release-audit -format csv -out matrix.csv examples/release-package/release.json
```

默认输出 Markdown：

```text
## 发行矩阵：Example Feature

| 版本 | 画幅 | 音频布局 | 字幕 | 发行窗口 | Manifest | 状态 |
| --- | --- | --- | --- | --- | --- | --- |
| theatrical | 2.39:1 (4096×1716) | 5.1 (6ch) | en (full), zh-Hans (sdh) | 2026-12-01 → 2027-02-28 | shared/release.json | OK |
| streaming | 16:9 (3840×2160) | 5.1 (6ch) | en (full) | 2027-03-01 → 2029-02-28 | shared/release.json | OK |
| mobile | 9:16 (1080×1920) | stereo (2ch) | en (full) | 2027-03-01 → 2029-02-28 | shared/release.json | OK |
```

退出码：

- `0`：没有阻断错误。
- `1`：存在错误；使用 `-strict` 时警告也会阻断。
- `2`：参数、输入 JSON 或输出格式错误。

CLI 参数：

```text
-root DIR       包根目录，默认是清单文件所在目录
-format FORMAT  markdown、json 或 csv
-out FILE       将报告写入文件
-skip-files     跳过资源与 manifest 文件存在性检查
-strict         警告也返回退出码 1
```

不传清单路径时，依次查找 `release.json`、`release-package.json`、`manifest.json`；传入 `-` 可从标准输入读取。

## 清单格式

```json
{
  "schemaVersion": 1,
  "title": "Feature",
  "versions": [
    {
      "id": "theatrical",
      "label": "影院",
      "window": {
        "channel": "theatrical",
        "start": "2026-12-01",
        "end": "2027-02-28"
      },
      "video": {
        "path": "theatrical/feature_theatrical.mxf",
        "aspectRatio": "2.39:1",
        "width": 4096,
        "height": 1716
      },
      "audio": {
        "path": "theatrical/feature_theatrical_51.wav",
        "layout": "5.1",
        "channels": 6,
        "language": "en"
      },
      "subtitles": [
        {
          "path": "theatrical/feature_theatrical_en.srt",
          "language": "en",
          "kind": "full"
        }
      ],
      "manifests": [
        {
          "path": "manifests/theatrical.json",
          "kind": "version",
          "references": ["theatrical/feature_theatrical.mxf"]
        }
      ]
    }
  ],
  "sharedManifests": [
    {
      "path": "shared/release.json",
      "kind": "package",
      "targets": [
        {
          "version": "theatrical",
          "references": ["theatrical/feature_theatrical.mxf"]
        },
        {
          "version": "streaming",
          "references": ["streaming/feature_streaming.mxf"]
        },
        {
          "version": "mobile",
          "references": ["mobile/feature_mobile.mp4"]
        }
      ]
    }
  ]
}
```

日期接受 `YYYY-MM-DD`、RFC3339、`YYYY-MM-DD HH:MM:SS`。路径默认相对包根目录；`http://`、`https://` 引用不检查本地文件，但仍参与版本标记校验。

### 共享 manifest

`sharedManifests[].targets` 必须覆盖清单中的每个版本。每个目标至少需要一个引用，引用可按以下任一方式证明归属：

1. 引用路径带有与目标版本一致的版本词；
2. 引用路径等于该版本已声明的视频、音频或字幕资源。

指向其他版本资源会报 `cross_version_reference`。全部引用都无法证明归属时报 `manifest_target_not_versioned`。共享 manifest 若为 `.json`，文件内的 `targets` 也必须与声明完全一致，否则报 `manifest_content_mismatch`。

## 主要诊断

| 类别 | 诊断码 |
| --- | --- |
| 版本缺项 | `missing_version`、`duplicate_version`、`unknown_version` |
| 成片缺项 | `missing_video_path`、`missing_aspect_ratio`、`missing_audio_path`、`missing_audio_layout`、`missing_subtitles`、`missing_subtitle_language`、`missing_subtitle_path` |
| 内容矛盾 | `invalid_aspect_ratio`、`aspect_ratio_mismatch`、`audio_layout_mismatch`、`duplicate_subtitle`、`invalid_release_window`、`window_version_mismatch` |
| 文件问题 | `missing_asset`、`invalid_asset`、`path_outside_root`、`invalid_manifest` |
| 交叉引用 | `cross_version_reference`、`ambiguous_version_reference`、`asset_shared_across_versions`、`implicitly_shared_manifest`、`manifest_path_conflict`、`manifest_path_version_mismatch`、`manifest_unknown_target`、`missing_manifest_target`、`manifest_target_not_versioned`、`manifest_content_mismatch` |

Markdown 和 JSON 报告会列出每个问题的版本、路径和消息；CSV 会把问题码汇总到对应行的 `issues` 列。`summary.missing` 统计缺项发现数，`summary.crossReferences` 统计串版及共享 manifest 指向问题的数量。

## Go API

```go
pkg, err := releaseaudit.LoadFile("release.json")
report := releaseaudit.Audit(pkg, releaseaudit.Options{Root: "."})
if report.HasErrors() {
    text, _ := report.Render(releaseaudit.FormatMarkdown)
    fmt.Print(text)
}
```

实现只使用 Go 标准库。文件检查验证路径、存在性和普通文件类型，不调用 ffprobe 等外部程序，也不解析媒体容器；画幅、声道数和发行窗口以清单声明为准，再做内部一致性校验。

## 验证

```sh
go test ./...
go vet ./...
```

测试覆盖完整三版本矩阵、缺版本/缺目标、画幅与声道矛盾、相似文件名串版、共享 manifest 错指向、JSON 内容漂移、文件缺失及 CLI 退出码。
