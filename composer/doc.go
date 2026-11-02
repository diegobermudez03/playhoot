// Package composer is the coordination layer for cross-domain reads
// (ARCHITECTURE.md). It holds no code itself: each group of related reads
// lives in a subpackage named for the behavior it serves (for example
// gameauthoring), never for a single domain, like orchestrator's groups.
package composer
