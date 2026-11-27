// Package cea708 decodes ANSI/CTA-708 (formerly CEA-708) digital television
// closed-caption byte streams using only the Go standard library.
//
// The decoder models the caption channel in three layers:
//
//   - PacketAssembler rebuilds DTVCC caption channel packets from the cc_data
//     triples carried in SMPTE 334 / SEI payloads.
//   - ParsePacket splits a reassembled packet into per-service blocks.
//   - ServiceDecoder is a stateful machine for one caption service. It keeps
//     the four graphic designation slots (G0..G3), the current GL/GR
//     invocation, transient single shifts, the eight caption windows and any
//     downloaded glyphs, and turns a service block into Tokens and Commands.
//
// Multiplexer ties the layers together and keeps one ServiceDecoder alive per
// service number across packets.
//
// Character sets. G0 is printable ASCII (with the musical note at 0x7F), G1 is
// ISO 8859-1 Latin-1, G2 holds the extended miscellaneous symbols and G3 the
// closed-caption sign. Locking shifts change which slot feeds the GL/GR region,
// single shifts reach G2/G3 for one byte, table select re-designates a slot,
// and EXT1 reaches G2/G3 directly. Every invocation is reflected in
// Token.Table.
//
// Custom glyphs. A DefineCustomChar command (an EXT1 C3 sequence) installs a
// bitmap at a (slot, graphic index) pair. References to that pair then decode
// to KindCustomChar tokens. Glyphs live until the pair is redefined or a Reset
// restores power-on state.
//
// Unknown codes. Reserved controls, unsupported table pages and unmapped
// graphic indices are never rendered as literal characters: they are emitted as
// KindUnknown tokens (or raw Commands) preserving the exact source bytes.
package cea708
