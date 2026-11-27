# 20261128

Repository initialized for 20261128.

## LC3 sample-accurate timeline (Go)

LC3 frames are either **10 ms** or **7.5 ms** long. A re-packetized player that
charges every frame a fixed duration drifts the moment the encoder switches
duration or emits a short tail frame. This library parses each frame's
`duration`, `octets` and 16-bit sequence number and reconstructs the exact PCM
sample interval for every frame plus the gaps left by lost packets. Pure Go
standard library only.

### Layout

- `lc3/codec.go` — legal rates/durations and the exact per-frame sample-count
  table (`Ns`): 80/60, 160/120, 240/180, 320/240, 480/360 at
  8/16/24/32/48 kHz for 10 ms / 7.5 ms.
- `lc3/timeline.go` — `Frame`, `Segment`, `Build` / `BuildWithOptions`; derives
  contiguous `[Start, End)` sample intervals and sequence-based loss gaps.
- `lc3/packet.go`, `lc3/packetize.go` — compact re-packetized wire format
  (16-bit seq, duration flag, per-frame octet length), marshal/parse, including
  truncated-tail detection.
- `lc3/bitstream.go` — parser for the Bluetooth LC3 reference bitstream
  (18-byte header, little-endian `size + payload` records).
- `cmd/lc3demo` — runnable example with a mode switch, a lost packet and a
  short tail frame.

### Key behavior

- Per-frame timing: 7.5 ms is exact in microseconds but not integer
  milliseconds, so all math is done in PCM samples.
- Loss detection: forward distance in 16-bit sequence space (handles wrap at
  65535 -> 0). Missing frame count is exact; gap sample length uses a selectable
  policy (`GapUseResumingDuration` default, or `GapUseFixedDuration`).
- Tail underrun: a final frame declaring more octets than the buffer holds is
  kept and flagged `Truncated`, while still occupying its full nominal span.

### Run

```sh
go test ./...
go run ./cmd/lc3demo
```
