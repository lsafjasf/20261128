package audit

import (
	"fmt"
	"sort"
	"time"

	"sidecaraudit/internal/sidecar"
)

// Options 控制审计行为。
type Options struct {
	PayloadRoot string // 载荷目录;非空时对每条已知算法断言重算核验
	Strict      bool   // true 时不可判定项也阻断发布
}

// Run 解析全部 sidecar,建立身份与证据链,返回报告。
func Run(sidecarFiles []string, opts Options) (*Report, error) {
	rep := &Report{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Sidecars:    sidecarFiles,
		PayloadRoot: opts.PayloadRoot,
	}

	var claims []sidecar.Claim
	for _, f := range sidecarFiles {
		cs, err := sidecar.ParseFile(f)
		if err != nil {
			return nil, fmt.Errorf("解析 sidecar 失败: %w", err)
		}
		claims = append(claims, cs...)
	}

	byPath := map[string][]sidecar.Claim{}
	for _, c := range claims {
		byPath[c.Path] = append(byPath[c.Path], c)
	}
	paths := make([]string, 0, len(byPath))
	for p := range byPath {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	for _, p := range paths {
		rep.addIdentity(p, byPath[p], opts)
	}

	rep.Verdict = "pass"
	for _, c := range rep.Conflicts {
		rep.BlockedReasons = append(rep.BlockedReasons,
			fmt.Sprintf("同名冲突: %s (%s) 出现 %d 个不同摘要", c.Path, c.Algorithm, len(c.Digests)))
	}
	for _, m := range rep.Mismatches {
		rep.BlockedReasons = append(rep.BlockedReasons,
			fmt.Sprintf("证据矛盾: %s (%s) 断言与载荷重算不一致,来源 %s:%d",
				m.Claim.Path, m.Claim.Algorithm, m.Claim.Sidecar, m.Claim.Line))
	}
	if opts.Strict {
		for _, u := range rep.Undecidable {
			rep.BlockedReasons = append(rep.BlockedReasons,
				fmt.Sprintf("不可判定(strict): %s 算法 %q: %s", u.Path, u.Algorithm, u.Reason))
		}
	}
	if len(rep.BlockedReasons) > 0 {
		rep.Verdict = "blocked"
	}
	return rep, nil
}
