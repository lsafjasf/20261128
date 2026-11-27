// Package audit 汇总多份 sidecar 的断言,建立文件身份与证据链,
// 并给出发布裁决:同名冲突或证据矛盾阻断发布,未知算法降级为不可判定。
package audit

import (
	"sidecaraudit/internal/sidecar"
)

// EvidenceStatus 是单条断言的核验状态。
type EvidenceStatus string

const (
	Verified    EvidenceStatus = "verified"     // 与载荷重算结果一致
	Mismatch    EvidenceStatus = "mismatch"     // 与载荷重算结果矛盾(阻断)
	MissingFile EvidenceStatus = "missing-file" // 载荷中找不到目标文件
	NotChecked  EvidenceStatus = "not-checked"  // 未提供载荷,未重算
)

// Evidence 是一条断言及其核验结果,构成证据链的一环。
type Evidence struct {
	Claim      sidecar.Claim  `json:"claim"`
	Status     EvidenceStatus `json:"status"`
	Recomputed string         `json:"recomputed,omitempty"`
}

// Conflict 描述同一 (路径, 算法) 下出现多个不同摘要的同名冲突。
type Conflict struct {
	Path      string          `json:"path"`
	Algorithm string          `json:"algorithm"`
	Digests   []string        `json:"digests"`
	Sources   []sidecar.Claim `json:"sources"`
}

// UndecidableItem 是不可判定项:未知算法或摘要格式非法,无法参与比对。
type UndecidableItem struct {
	Path      string        `json:"path"`
	Algorithm string        `json:"algorithm"`
	Reason    string        `json:"reason"`
	Claim     sidecar.Claim `json:"claim"`
}

// FileIdentity 是一个规范化路径下的全部断言与核验证据。
type FileIdentity struct {
	Path        string              `json:"path"`
	Digests     map[string][]string `json:"digests"` // 算法 -> 去重后的摘要集合
	Evidence    []Evidence          `json:"evidence"`
	Conflict    bool                `json:"conflict"`
	Undecidable bool                `json:"undecidable"`
}

// Report 是完整审计报告,Verdict 为 "pass" 或 "blocked"。
type Report struct {
	GeneratedAt    string            `json:"generated_at"`
	Sidecars       []string          `json:"sidecars"`
	PayloadRoot    string            `json:"payload_root,omitempty"`
	Identities     []FileIdentity    `json:"identities"`
	Conflicts      []Conflict        `json:"conflicts"`
	Undecidable    []UndecidableItem `json:"undecidable"`
	Mismatches     []Evidence        `json:"mismatches"`
	Verdict        string            `json:"verdict"`
	BlockedReasons []string          `json:"blocked_reasons,omitempty"`
}
