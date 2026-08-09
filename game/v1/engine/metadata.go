package engine

// Metadata is the compiled identity and versioning information of one
// game version, carried over unchanged from program.Metadata.
type Metadata struct {
	ID              string
	Name            string
	Description     string
	Version         string
	LanguageVersion string
}
