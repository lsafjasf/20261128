package classify

import "testing"

func TestClass(t *testing.T) {
	cases := map[string]string{
		"photo.CR2": ClassMedia,
		"clip.mov":  ClassMedia,
		"song.flac": ClassMedia,
		"photo.xmp": ClassSidecar,
		"clip.srt":  ClassSidecar,
		"notes.txt": ClassOther,
		"PHOTO.JPG": ClassMedia,
		"meta.JSON": ClassSidecar,
	}
	for name, want := range cases {
		if got := Class(name); got != want {
			t.Errorf("Class(%q) = %q, want %q", name, got, want)
		}
	}
	if MediaKind("a.mp4") != "video" {
		t.Fatal("expected video kind for mp4")
	}
	if MIME("clip.cr3") == "application/octet-stream" {
		t.Fatal("expected registered CR3 MIME type")
	}
	if Stem("clip.mp4.srt") != "clip.mp4" {
		t.Fatalf("stem mismatch: %q", Stem("clip.mp4.srt"))
	}
}
