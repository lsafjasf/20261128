// Command captiondemo walks a simulated live stream through the four
// guarantees of the versioned caption session:
//
//  1. delay compensation (captions lag speech by ~2s)
//  2. local, single-segment replacement without transcript reflow
//  3. disconnect/reconnect converging to the live state
//  4. rejection of revisions that would backfill displayed content
package main

import (
	"errors"
	"fmt"

	"captionsession"
)

func main() {
	session := captionsession.NewSession()

	fmt.Println("== versioned caption session demo ==")

	// 1. ASR provisional segments arrive (Upsert keeps segment identity).
	upsert(session, captionsession.Segment{ID: "u1", StartPTS: 0, EndPTS: 2000, Text: "welcome every one"})
	upsert(session, captionsession.Segment{ID: "u2", StartPTS: 2000, EndPTS: 4000, Text: "to the sho"})

	// 2. Operator corrects one line: only u2 changes, order is untouched.
	op, err := session.Replace("u2", "to the show")
	must(err)
	fmt.Printf("local replace  -> v%d %q => %q (no reflow)\n", op.Version, op.OldText, op.NewText)

	// 3. Watermark commits u1; it is now frozen.
	wm, moved, err := session.AdvanceWatermark(2000)
	must(err)
	fmt.Printf("watermark      -> v%d pts=%d moved=%t (u1 displayed, frozen)\n", wm.Version, wm.Watermark, moved)

	// 4. Late ASR revision and operator edit of displayed u1 are rejected.
	_, err = session.Upsert(captionsession.Segment{ID: "u1", StartPTS: 0, EndPTS: 2000, Text: "welcome everyone"})
	fmt.Printf("ASR backfill   -> %v\n", err)
	_, err = session.Replace("u1", "hijacked")
	fmt.Printf("edit backfill  -> %v\n", err)

	// 5. A reconnecting viewer replays the deterministic op log.
	_, ops, err := session.Replay(captionsession.InitialVersion)
	must(err)
	replayed, err := captionsession.Apply(nil, ops)
	must(err)
	fmt.Println("reconnected view:")
	for _, seg := range replayed.Segments() {
		state := "provisional"
		if replayed.IsFrozen(seg.ID) {
			state = "frozen"
		}
		fmt.Printf("  [%s] %s %q\n", state, seg.ID, seg.Text)
	}

	// 6. Compaction: old ops snapshotted; ancient clients must resync.
	snap, err := session.Compact(0)
	must(err)
	_, _, err = session.Replay(snap.BaseVersion - 1)
	if !errors.Is(err, captionsession.ErrResync) {
		panic("expected ErrResync after compaction")
	}
	fmt.Printf("compaction     -> base v%d, pre-base reconnect requires snapshot resync\n", snap.BaseVersion)

	fmt.Printf("final version  -> %d\n", session.Version())
}

func upsert(s *captionsession.Session, seg captionsession.Segment) {
	op, err := s.Upsert(seg)
	must(err)
	fmt.Printf("upsert         -> v%d %s %q\n", op.Version, seg.ID, seg.Text)
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}
