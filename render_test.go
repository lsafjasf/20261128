package renderpipe

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"
)

func TestRenderTimezoneIndependent(t *testing.T) {
	project := makeProject(t, 12)
	outRoot := t.TempDir()

	oldLocal := time.Local
	t.Cleanup(func() { time.Local = oldLocal })
	t.Setenv("TZ", "UTC")
	time.Local = time.UTC
	first, err := Render(project, filepath.Join(outRoot, "first"))
	if err != nil {
		t.Fatalf("first render: %v", err)
	}

	// Metadata must not participate in the source hash.
	future := time.Date(2035, 7, 8, 9, 10, 11, 0, time.FixedZone("future", 11*3600))
	for _, name := range []string{"frame-1.png", "frame-2.png"} {
		if err := os.Chtimes(filepath.Join(project, "frames", name), future, future); err != nil {
			t.Fatal(err)
		}
	}

	time.Local = time.FixedZone("renderer-plus-eight", 8*3600)
	t.Setenv("TZ", "Asia/Shanghai")
	second, err := Render(project, filepath.Join(outRoot, "second"))
	if err != nil {
		t.Fatalf("second render: %v", err)
	}

	if first.TaskID != second.TaskID {
		t.Fatalf("task IDs differ: %s != %s", first.TaskID, second.TaskID)
	}
	if first.SourceHash != second.SourceHash {
		t.Fatalf("source hashes differ: %s != %s", first.SourceHash, second.SourceHash)
	}
	if filepath.Base(first.Directory) != filepath.Base(second.Directory) {
		t.Fatalf("artifact directory names differ")
	}
	if first.MediaHash != second.MediaHash || first.FrameSetHash != second.FrameSetHash {
		t.Fatalf("artifact hashes differ: media %s/%s frame set %s/%s",
			first.MediaHash, second.MediaHash, first.FrameSetHash, second.FrameSetHash)
	}

	firstMedia := mustRead(t, first.MediaPath)
	secondMedia := mustRead(t, second.MediaPath)
	if !bytes.Equal(firstMedia, secondMedia) {
		t.Fatal("media bytes differ across timezones")
	}
	if len(first.Frames) != 12 || len(second.Frames) != 12 {
		t.Fatalf("unexpected frame count: %d/%d", len(first.Frames), len(second.Frames))
	}
	for i := range first.Frames {
		if filepath.Base(first.Frames[i]) != filepath.Base(second.Frames[i]) {
			t.Fatalf("frame %d name differs: %s != %s",
				i, filepath.Base(first.Frames[i]), filepath.Base(second.Frames[i]))
		}
		if !bytes.Equal(mustRead(t, first.Frames[i]), mustRead(t, second.Frames[i])) {
			t.Fatalf("frame %d bytes differ across timezones", i)
		}
	}
}

func TestFramesAreNaturallySortedAndSequentiallyNamed(t *testing.T) {
	project := makeProject(t, 12)
	out := t.TempDir()
	plan, err := PlanTask(project, out)
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= 12; i++ {
		want := "frames/frame-" + itoa(i) + ".png"
		if plan.Frames[i-1] != want {
			t.Fatalf("frame %d order = %q, want %q", i, plan.Frames[i-1], want)
		}
	}

	result, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyArtifact(result.Directory)
	if err != nil {
		t.Fatal(err)
	}
	for i, frame := range manifest.Frames {
		want := "frames/" + frameName(i+1, len(manifest.Frames))
		if frame.Path != want {
			t.Fatalf("frame %d = %q, want %q", i+1, frame.Path, want)
		}
	}
}

func TestRenderReusesSuccessfulArtifact(t *testing.T) {
	project := makeProject(t, 3)
	out := t.TempDir()
	first, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	manifestBefore := mustRead(t, filepath.Join(first.Directory, ManifestFileName))
	mediaBefore := mustRead(t, first.MediaPath)
	marker := filepath.Join(first.Directory, "keep-me")
	if err := os.WriteFile(marker, []byte("do not replace"), 0o644); err != nil {
		t.Fatal(err)
	}

	second, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Reused {
		t.Fatal("second render did not reuse the successful artifact")
	}
	if second.Directory != first.Directory || second.MediaHash != first.MediaHash {
		t.Fatal("reused artifact identity changed")
	}
	if !bytes.Equal(manifestBefore, mustRead(t, filepath.Join(first.Directory, ManifestFileName))) {
		t.Fatal("successful manifest was modified")
	}
	if !bytes.Equal(mediaBefore, mustRead(t, first.MediaPath)) {
		t.Fatal("successful media was modified")
	}
	if string(mustRead(t, marker)) != "do not replace" {
		t.Fatal("existing artifact contents were touched")
	}
}

func TestCorruptSuccessfulArtifactIsNotOverwritten(t *testing.T) {
	project := makeProject(t, 2)
	out := t.TempDir()
	result, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("corrupt")
	if err := os.WriteFile(result.MediaPath, corrupt, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Render(project, out)
	if !errors.Is(err, ErrArtifactConflict) {
		t.Fatalf("render error = %v, want ErrArtifactConflict", err)
	}
	if !bytes.Equal(mustRead(t, result.MediaPath), corrupt) {
		t.Fatal("corrupt existing artifact was overwritten")
	}
	if _, err := VerifyArtifact(result.Directory); !errors.Is(err, ErrInvalidArtifact) {
		t.Fatalf("verify error = %v, want ErrInvalidArtifact", err)
	}
}

func TestIncompleteArtifactPathIsNotDeleted(t *testing.T) {
	project := makeProject(t, 2)
	out := t.TempDir()
	plan, err := PlanTask(project, out)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(plan.Directory, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(plan.Directory, "partial.data")
	if err := os.WriteFile(sentinel, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = Render(project, out)
	if !errors.Is(err, ErrArtifactConflict) {
		t.Fatalf("render error = %v, want ErrArtifactConflict", err)
	}
	if string(mustRead(t, sentinel)) != "keep" {
		t.Fatal("incomplete artifact contents were deleted")
	}
}

func TestSourceChangePublishesNewDirectory(t *testing.T) {
	project := makeProject(t, 3)
	out := t.TempDir()
	first, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	firstMedia := mustRead(t, first.MediaPath)

	writeFrame(t, filepath.Join(project, "frames", "frame-2.png"), color.RGBA{R: 250, A: 255})
	second, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID == second.TaskID || first.Directory == second.Directory {
		t.Fatal("source change did not produce a new task")
	}
	if !bytes.Equal(firstMedia, mustRead(t, first.MediaPath)) {
		t.Fatal("previous successful artifact was modified")
	}
	if _, err := VerifyArtifact(first.Directory); err != nil {
		t.Fatalf("previous artifact is no longer valid: %v", err)
	}
	if _, err := VerifyArtifact(second.Directory); err != nil {
		t.Fatalf("new artifact is invalid: %v", err)
	}
}

func TestNestedOutputDoesNotChangeSourceHash(t *testing.T) {
	project := makeProject(t, 2)
	out := filepath.Join(project, "generated")
	first, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Render(project, out)
	if err != nil {
		t.Fatal(err)
	}
	if first.TaskID != second.TaskID || !second.Reused {
		t.Fatalf("nested output affected task identity: %s/%s reused=%v",
			first.TaskID, second.TaskID, second.Reused)
	}
}

func TestProjectFrameListIsExplicitlyOrdered(t *testing.T) {
	project := t.TempDir()
	writeFrame(t, filepath.Join(project, "frames", "a.png"), color.RGBA{R: 255, A: 255})
	writeFrame(t, filepath.Join(project, "frames", "b.png"), color.RGBA{G: 255, A: 255})
	config := `{"version":1,"fps":5,"frames":["frames/b.png","frames/a.png"]}`
	if err := os.WriteFile(filepath.Join(project, ProjectFileName), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Render(project, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := VerifyArtifact(result.Directory)
	if err != nil {
		t.Fatal(err)
	}
	firstColor := pngColor(t, filepath.Join(result.Directory, filepath.FromSlash(manifest.Frames[0].Path)))
	if firstColor.G < 200 || firstColor.R > 50 {
		t.Fatalf("first output frame should come from b.png, got %#v", firstColor)
	}
}

func TestConcurrentRenderPublishesOneArtifact(t *testing.T) {
	project := makeProject(t, 4)
	out := t.TempDir()
	const workers = 8
	results := make([]RenderResult, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			results[index], errs[index] = Render(project, out)
		}(i)
	}
	close(start)
	wg.Wait()

	created := 0
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("worker %d: %v", i, errs[i])
		}
		if results[i].TaskID != results[0].TaskID || results[i].MediaHash != results[0].MediaHash {
			t.Fatalf("worker %d got a different artifact", i)
		}
		if !results[i].Reused {
			created++
		}
	}
	if created != 1 {
		t.Fatalf("created %d artifacts, want exactly 1", created)
	}
	if _, err := VerifyArtifact(results[0].Directory); err != nil {
		t.Fatalf("published artifact is invalid: %v", err)
	}
}

func makeProject(t *testing.T, count int) string {
	t.Helper()
	root := t.TempDir()
	for i := 1; i <= count; i++ {
		writeFrame(t, filepath.Join(root, "frames", "frame-"+itoa(i)+".png"),
			color.RGBA{R: uint8(i), G: uint8(20 + i), B: uint8(40 + i), A: 255})
	}
	return root
}

func writeFrame(t *testing.T, name string, c color.RGBA) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 8, 6))
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	file, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(file, img); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func pngColor(t *testing.T, name string) color.RGBA {
	t.Helper()
	file, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img, err := png.Decode(file)
	if err != nil {
		t.Fatal(err)
	}
	return color.RGBAModel.Convert(img.At(0, 0)).(color.RGBA)
}

func mustRead(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func itoa(value int) string {
	return strconv.Itoa(value)
}
