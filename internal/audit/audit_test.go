package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func sha256Of(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func writeSidecar(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAgreementPasses(t *testing.T) {
	dir := t.TempDir()
	d := sha256Of("hello")
	a := writeSidecar(t, dir, "a.sha256", d+"  app.bin\n")
	b := writeSidecar(t, dir, "b.json",
		`[{"path":"./app.bin","algorithm":"sha256","digest":"`+d+`"}]`)
	rep, err := Run([]string{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "pass" {
		t.Fatalf("一致断言应通过, 得到 %s: %v", rep.Verdict, rep.BlockedReasons)
	}
	if len(rep.Identities) != 1 || rep.Identities[0].Path != "app.bin" {
		t.Fatalf("路径归一化后应合并为同一身份: %+v", rep.Identities)
	}
	if len(rep.Identities[0].Evidence) != 2 {
		t.Errorf("证据链应含 2 条来源: %+v", rep.Identities[0].Evidence)
	}
}

func TestSameNameConflictBlocks(t *testing.T) {
	dir := t.TempDir()
	a := writeSidecar(t, dir, "a.sha256", sha256Of("v1")+"  app.bin\n")
	b := writeSidecar(t, dir, "b.sha256", sha256Of("v2")+"  app.bin\n")
	rep, err := Run([]string{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "blocked" {
		t.Fatalf("同名冲突必须阻断, 得到 %s", rep.Verdict)
	}
	if len(rep.Conflicts) != 1 || len(rep.Conflicts[0].Digests) != 2 {
		t.Fatalf("冲突记录异常: %+v", rep.Conflicts)
	}
	if len(rep.Conflicts[0].Sources) != 2 {
		t.Errorf("冲突应保留双方出处: %+v", rep.Conflicts[0].Sources)
	}
}

func TestUnknownAlgoDowngradesNotBlocks(t *testing.T) {
	dir := t.TempDir()
	d := sha256Of("x")
	a := writeSidecar(t, dir, "a.json",
		`[{"path":"f.bin","algorithm":"blake3","digest":"`+d+`"}]`)
	b := writeSidecar(t, dir, "b.sha256", d+"  f.bin\n")
	rep, err := Run([]string{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "pass" {
		t.Fatalf("未知算法默认不阻断, 得到 %s", rep.Verdict)
	}
	if len(rep.Undecidable) != 1 || rep.Undecidable[0].Algorithm != "blake3" {
		t.Fatalf("应产生 1 条不可判定项: %+v", rep.Undecidable)
	}

	repStrict, err := Run([]string{a, b}, Options{Strict: true})
	if err != nil {
		t.Fatal(err)
	}
	if repStrict.Verdict != "blocked" {
		t.Errorf("strict 模式下不可判定项应阻断, 得到 %s", repStrict.Verdict)
	}
}

func TestPayloadVerification(t *testing.T) {
	dir := t.TempDir()
	payload := t.TempDir()
	if err := os.WriteFile(filepath.Join(payload, "ok.bin"), []byte("good"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, "bad.bin"), []byte("tampered"), 0o644); err != nil {
		t.Fatal(err)
	}
	a := writeSidecar(t, dir, "a.sha256",
		sha256Of("good")+"  ok.bin\n"+sha256Of("original")+"  bad.bin\n"+sha256Of("ghost")+"  gone.bin\n")
	b := writeSidecar(t, dir, "b.sha256",
		sha256Of("good")+"  ok.bin\n"+sha256Of("original")+"  bad.bin\n"+sha256Of("ghost")+"  gone.bin\n")
	rep, err := Run([]string{a, b}, Options{PayloadRoot: payload})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Verdict != "blocked" {
		t.Fatalf("载荷矛盾必须阻断, 得到 %s", rep.Verdict)
	}
	if len(rep.Mismatches) != 2 { // 两份 sidecar 对 bad.bin 各一条
		t.Errorf("bad.bin 应产生 2 条矛盾证据, 得到 %d", len(rep.Mismatches))
	}
	status := map[string]EvidenceStatus{}
	for _, id := range rep.Identities {
		status[id.Path] = id.Evidence[0].Status
	}
	if status["ok.bin"] != Verified || status["bad.bin"] != Mismatch || status["gone.bin"] != MissingFile {
		t.Errorf("核验状态异常: %v", status)
	}
}

func TestInvalidDigestDowngrades(t *testing.T) {
	dir := t.TempDir()
	a := writeSidecar(t, dir, "a.sha256", strings.Repeat("z", 64)+"  f.bin\n")
	b := writeSidecar(t, dir, "b.sha256", sha256Of("x")+"  f.bin\n")
	rep, err := Run([]string{a, b}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Undecidable) != 1 {
		t.Fatalf("非法十六进制摘要应降级为不可判定: %+v", rep.Undecidable)
	}
	if rep.Verdict != "pass" {
		t.Errorf("非法摘要不应与合法断言构成冲突, 得到 %s", rep.Verdict)
	}
}
