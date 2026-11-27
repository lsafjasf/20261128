// Package batchout provides crash-recoverable, no-clobber publication of
// batch-generated files.
//
// Each Run stages files outside the visible output tree, records an fsynced
// write-ahead journal, and publishes files with atomic hard links that fail if
// a target already exists. A task lock keyed by inputs and parameters prevents
// concurrent duplicate execution. Committed provenance is stored under the
// output directory's .batch state root and can be queried with Lookup.
package batchout
