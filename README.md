# cea708

A pure-Go (standard library only) decoder for ANSI/CTA-708 (formerly CEA-708)
DTV closed-caption streams.

The decoder maintains the full caption state instead of treating control bytes
as text:

- **Four graphic character sets** — G0 (ASCII + musical note), G1 (Latin-1),
  G2 (extended symbols) and G3 (the `[CC]` sign) — kept as four independent
  designation slots.
- **Current window state** — eight windows with Define/Clear/Display/Hide/
  Toggle/Delete, pen location and a row/column cell grid.
- **Code-table switching** — locking shifts (GL/GR), single shifts (SS2/SS3),
  two-byte table select and the EXT1 introducer, all reflected on each
  `Token.Table`.
- **Custom character lifetime** — bitmap glyphs installed by a
  DefineCustomChar command are referenced by `(slot, index)`, survive across
  packets, and are dropped on redefinition or Reset.
- **Unknown codes preserved** — reserved controls and unmapped indices are
  emitted as unknown tokens/commands carrying their exact raw bytes, never
  printed as literal characters.

## Layout

- `cea708/` — the decoder package (packet assembly, service blocks, the stateful
  service decoder, character tables, window model).
- `cmd/cea708dump/` — small CLI that decodes a hex DTVCC packet.

## Usage

```go
mux := cea708.NewMultiplexer()

// Reassemble cc_data triples into DTVCC packets, then decode:
for _, result := range mux.PushPacket(packet) {
    for _, tok := range result.Tokens {
        // tok.Kind, tok.Table (G0..G3), tok.Rune, tok.Raw
    }
    for _, cmd := range result.Commands {
        // window/pen/custom-char commands
    }
}
```

### CLI

```
go run ./cmd/cea708dump 0d3898000000000f00804142e91033193510a01090005a00805a0500
```

## Tests

```
go test ./...
```
