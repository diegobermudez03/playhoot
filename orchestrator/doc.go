// Package orchestrator is the coordination layer for cross-domain write
// operations (ARCHITECTURE.md -> Cross-Domain Writes). It holds no code
// itself: each group of related operations lives in a subpackage named for
// the behavior it coordinates (for example playlifecycle), never for a
// single domain. See README.md for how those subpackages are organized.
package orchestrator
