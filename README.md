# 20261128

## MPE note-lifecycle recorder (Go)

MPE controllers borrow a member channel per gesture and recycle it on
key-up, so recording by channel merges unrelated notes and loses their
bend/pressure. This module reconstructs each note from the **(note ID,
member channel)** pair and keeps expression attached to the right note.

### Behavior

- Every note-on gets a unique monotonic note ID; channel reuse never merges
  gestures.
- Per-note pitch bend, channel pressure, polyphonic pressure and CC74
  timbre are stored on the note that owns the member channel.
- Bend/pressure arriving before note-on is **parked** on the channel and
  inherited by the next note.
- Channel exhaustion (note-on on a still-busy channel) closes the previous
  note as `StateStolen` without losing it; the new note still starts.
- Unmatched/wrong-note note-offs are counted as orphans, never kill an
  unrelated note; backward timestamps are clamped; dangling notes are
  emitted by `Flush` as `StateFlushed`.
- MPE Configuration Message (RPN 6) sets lower/upper zones; factory default
  is a lower zone with 15 members.

### Layout

- `mpe/` — library: parser, zone config, lifecycle state machine, expression routing.
- `cmd/mpedemo/` — runnable demo over a synthetic MPE byte stream.

### Run

```sh
go test ./...
go run ./cmd/mpedemo
```
