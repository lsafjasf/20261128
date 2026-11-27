package audit

import (
	"fmt"
	"io"
	"sort"
)

// RenderText 输出人可读的审计报告。
func RenderText(w io.Writer, rep *Report) {
	fmt.Fprintf(w, "审计时间: %s\n", rep.GeneratedAt)
	fmt.Fprintf(w, "sidecar 数: %d", len(rep.Sidecars))
	if rep.PayloadRoot != "" {
		fmt.Fprintf(w, "  载荷目录: %s", rep.PayloadRoot)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "\n== 文件身份 (%d) ==\n", len(rep.Identities))
	for _, id := range rep.Identities {
		status := "OK"
		if id.Conflict {
			status = "CONFLICT"
		} else if id.Undecidable {
			status = "UNDECIDABLE"
		}
		fmt.Fprintf(w, "[%s] %s\n", status, id.Path)
		algos := make([]string, 0, len(id.Digests))
		for a := range id.Digests {
			algos = append(algos, a)
		}
		sort.Strings(algos)
		for _, a := range algos {
			fmt.Fprintf(w, "    %s: %v\n", a, id.Digests[a])
		}
		for _, ev := range id.Evidence {
			fmt.Fprintf(w, "    <- %s:%d  %s\n", ev.Claim.Sidecar, ev.Claim.Line, ev.Status)
		}
	}

	if len(rep.Conflicts) > 0 {
		fmt.Fprintf(w, "\n== 同名冲突 (%d,阻断发布) ==\n", len(rep.Conflicts))
		for _, c := range rep.Conflicts {
			fmt.Fprintf(w, "  %s [%s]\n", c.Path, c.Algorithm)
			for _, s := range c.Sources {
				fmt.Fprintf(w, "    %s  <- %s:%d\n", s.Digest, s.Sidecar, s.Line)
			}
		}
	}
	if len(rep.Mismatches) > 0 {
		fmt.Fprintf(w, "\n== 证据矛盾 (%d,阻断发布) ==\n", len(rep.Mismatches))
		for _, m := range rep.Mismatches {
			fmt.Fprintf(w, "  %s [%s] 断言 %s,重算 %s <- %s:%d\n",
				m.Claim.Path, m.Claim.Algorithm, m.Claim.Digest, m.Recomputed,
				m.Claim.Sidecar, m.Claim.Line)
		}
	}
	if len(rep.Undecidable) > 0 {
		fmt.Fprintf(w, "\n== 不可判定项 (%d) ==\n", len(rep.Undecidable))
		for _, u := range rep.Undecidable {
			fmt.Fprintf(w, "  %s [%s] %s <- %s:%d\n",
				u.Path, u.Algorithm, u.Reason, u.Claim.Sidecar, u.Claim.Line)
		}
	}

	fmt.Fprintf(w, "\n裁决: %s\n", rep.Verdict)
	for _, r := range rep.BlockedReasons {
		fmt.Fprintf(w, "  - %s\n", r)
	}
}
