package audit

import (
	"crypto/subtle"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sidecaraudit/internal/algo"
	"sidecaraudit/internal/sidecar"
)

// addIdentity 为单个路径建立身份:聚合断言、检测同名冲突、核验载荷证据。
func (rep *Report) addIdentity(path string, claims []sidecar.Claim, opts Options) {
	id := FileIdentity{Path: path, Digests: map[string][]string{}}
	byAlgo := map[string]map[string][]sidecar.Claim{} // 算法 -> 摘要 -> 断言列表

	for _, c := range claims {
		if c.Unknown {
			id.Undecidable = true
			rep.Undecidable = append(rep.Undecidable, UndecidableItem{
				Path: path, Algorithm: c.Algorithm,
				Reason: "未知摘要算法,无法比对", Claim: c,
			})
			continue
		}
		a, _ := algo.Lookup(c.Algorithm)
		if !algo.ValidDigest(a, c.Digest) {
			id.Undecidable = true
			rep.Undecidable = append(rep.Undecidable, UndecidableItem{
				Path: path, Algorithm: c.Algorithm,
				Reason: "摘要不是该算法的合法十六进制串", Claim: c,
			})
			continue
		}
		if byAlgo[c.Algorithm] == nil {
			byAlgo[c.Algorithm] = map[string][]sidecar.Claim{}
		}
		byAlgo[c.Algorithm][c.Digest] = append(byAlgo[c.Algorithm][c.Digest], c)
		id.Evidence = append(id.Evidence, rep.verify(c, opts))
	}

	for alg, digests := range byAlgo {
		list := make([]string, 0, len(digests))
		for d := range digests {
			list = append(list, d)
		}
		sort.Strings(list)
		id.Digests[alg] = list
		if len(list) > 1 {
			id.Conflict = true
			con := Conflict{Path: path, Algorithm: alg, Digests: list}
			for _, d := range list {
				con.Sources = append(con.Sources, digests[d]...)
			}
			rep.Conflicts = append(rep.Conflicts, con)
		}
	}

	rep.Identities = append(rep.Identities, id)
}

// verify 对单条断言做载荷重算核验;未提供载荷时标记 not-checked。
func (rep *Report) verify(c sidecar.Claim, opts Options) Evidence {
	ev := Evidence{Claim: c, Status: NotChecked}
	if opts.PayloadRoot == "" {
		return ev
	}
	root := filepath.Clean(opts.PayloadRoot)
	full := filepath.Join(root, filepath.FromSlash(c.Path))
	if full != root && !strings.HasPrefix(full, root+string(os.PathSeparator)) {
		ev.Status = MissingFile // 路径逃逸出载荷目录,按缺失处理
		return ev
	}
	f, err := os.Open(full)
	if err != nil {
		ev.Status = MissingFile
		return ev
	}
	defer f.Close()

	a, _ := algo.Lookup(c.Algorithm)
	h := a.NewHash()
	if _, err := io.Copy(h, f); err != nil {
		ev.Status = MissingFile
		return ev
	}
	ev.Recomputed = hex.EncodeToString(h.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(ev.Recomputed), []byte(c.Digest)) == 1 {
		ev.Status = Verified
	} else {
		ev.Status = Mismatch
		rep.Mismatches = append(rep.Mismatches, ev)
	}
	return ev
}
