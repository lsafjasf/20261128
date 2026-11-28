// Package mxfindex scans MXF KLV streams, validates partition and index
// boundaries, and rebuilds a seekable edit-unit index from Index Table
// Segments plus observed essence elements.
//
// The package is deliberately conservative: Result.Success is true only when
// the rebuilt index is complete. Missing, truncated, or unmappable index data
// yields StatusIncomplete even when best-effort recovery ranges are returned.
package mxfindex
