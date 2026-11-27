# opusmap

`opusmap` inspects multichannel Ogg Opus streams without an Opus decoder. It
parses `OpusHead`, expands the channel mapping table into decoded-stream
sources, and validates the packed Opus framing of every audio packet.

It is intended for files where mapping family 2 looks playable but only a
prefix of the output channels has audio. The usual cause is a mapping table
that references decoded-channel indices at or above `N + M`. With `N=2, M=1`
and `mapping=[0,1,2,3,4,5]`, only indices 0, 1, and 2 are valid; channels 3
through 5 are illegal and players commonly turn them into silence.

## Build and run

```sh
go build ./cmd/opusmap
./opusmap -json problem.opus
```

Exit status is `0` for a consistent stream, `2` for diagnostic findings, and
`1` for I/O or unrecoverable container/header errors.

Text output includes:

- `OpusHead` fields: channel count, family, stream count `N`, coupled count
  `M`, and the raw mapping table.
- One entry per coded stream, including whether it is coupled to two channels
  and which output channels consume it.
- One entry per physical output channel. Family 2 channels get ACN names plus
  the optional non-diegetic left/right pair. Each source is `mono`,
  `stereo_left`, `stereo_right`, `silence`, or `invalid`.
- Every issue with zero-based `PacketSeq` and the completing Ogg page.

## Framing

For a mapping with `N` streams, an Ogg audio packet contains `N-1` Opus packets
in RFC 6716 Appendix B self-delimiting framing and a final packet in regular
framing. `opusmap` parses codes 0, 1, 2, and 3, including VBR/CBR code 3
padding, two-byte frame lengths, and zero-byte DTX frames. The final regular
packet consumes the remaining payload, so a short final frame is valid rather
than treated as an incomplete stream.

Ogg packets may span pages. Lacing values of 255 continue a packet; a final
lacing value below 255, including zero, completes it. A fragment left at EOF
is reported as `truncated_packet`.

## Library

The main entry point is:

```go
report, err := opusmap.Scan(reader)
```

Lower-level helpers are exported for custom pipelines:

- `DecodeOpusHead` parses a header and retains partial results plus all issues.
- `ReconstructMapping` builds stream coupling and physical-channel views.
- `ParseMultistreamPacket` splits one packed audio packet into `N` Opus
  framing records.
- `ReadOggPage` reads and validates one Ogg page and its CRC.

The implementation uses only the Go standard library.
