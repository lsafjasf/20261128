// Package wavinfo reads and rewrites the LIST/INFO metadata of WAV (RIFF)
// files without touching any other part of the container.
//
// # Design guarantees
//
// The audio data chunk and every other non-INFO sub-chunk are stored with
// their exact on-disk bytes (header, payload and RIFF padding). Rewriting a
// tag only rebuilds the LIST INFO chunk; untouched chunks are copied verbatim
// and the outer RIFF size is recomputed to stay consistent. A WAV file without
// a LIST INFO chunk gets one inserted immediately before the data chunk.
//
// # INFO field handling
//
// Standard INFO identifiers (IART, INAM, IPRD, ...) are serialized in the
// authoritative order defined by the RIFF specification, exposed through
// CanonicalOrder. Unrecognized identifiers are never discarded: they keep
// their raw payload bytes (including non-text data) and their original
// relative order, and are emitted after the known fields.
//
// Project field names such as "title", "album" or "artist" are resolved onto
// INFO identifiers through ResolveField. When project metadata and a value
// parsed from the file name disagree, ApplyProjectFields reports the conflict
// and applies a Preference (embedded tags win by default).
package wavinfo
