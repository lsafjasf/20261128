// Package rendercache provides content-addressed caching for dependency-aware
// render stages.
//
// Cache keys include the project, stage implementation version, stage config,
// content fingerprints for every declared resource, and the keys of direct
// upstream stages. Unknown resource versions fail before any cache lookup, so
// stale assets cannot be reused from another project or an older run.
package rendercache
