package imsc

import "testing"

// TestDocumentConflictAcrossCues models an external URI that is reached from
// two cues but resolves to different fetched payloads (e.g. a mutated asset).
// The merge must be blocked and both payloads retained with separate keys.
func TestDocumentConflictAcrossCues(t *testing.T) {
	doc := NewDocument()
	original := []byte("PNG-ORIGINAL")
	tampered := []byte("PNG-TAMPERED")

	r1, conflict1 := doc.Resources.RegisterContent("images/logo.png", KindExternal, "image/png", original, "")
	r2, conflict2 := doc.Resources.RegisterContent("images/logo.png", KindExternal, "image/png", tampered, "")

	if conflict1 != nil || conflict2 == nil {
		t.Fatalf("only the second registration should conflict")
	}
	if r1.Key == r2.Key {
		t.Fatal("same URI with different content must not merge")
	}
	if doc.Resources.Len() != 2 {
		t.Fatalf("both payloads must be retained, got %d", doc.Resources.Len())
	}

	doc.Cues = append(doc.Cues,
		&Cue{ID: "c1", ResourceKey: r1.Key, ImageURI: "images/logo.png", RegionID: "r"},
		&Cue{ID: "c2", ResourceKey: r2.Key, ImageURI: "images/logo.png", RegionID: "r"},
	)

	if doc.Cues[0].ResourceKey == doc.Cues[1].ResourceKey {
		t.Fatal("cues must keep distinct resource keys after a conflict")
	}
	bound, _ := doc.Resources.Lookup("images/logo.png")
	if bound.Key != r1.Key {
		t.Fatal("URI binding must remain on the first-seen resource")
	}
}

// TestNoConflictWhenDifferentURIsShareBytes ensures legitimate de-duplication
// still happens for identical content reached via different paths.
func TestNoConflictWhenDifferentURIsShareBytes(t *testing.T) {
	reg := NewRegistry()
	payload := []byte("SHARED-BYTES")
	r1, c1 := reg.RegisterContent("a/logo.png", KindExternal, "image/png", payload, "")
	r2, c2 := reg.RegisterContent("b/logo.png", KindExternal, "image/png", payload, "")
	if c1 != nil || c2 != nil {
		t.Fatalf("identical bytes must never conflict: %v %v", c1, c2)
	}
	if r1.Key != r2.Key || reg.Len() != 1 {
		t.Fatal("identical content must collapse to one hash entry")
	}
}
