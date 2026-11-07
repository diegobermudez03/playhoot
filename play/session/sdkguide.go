package session

import "errors"

// The SDK guide pages an agent asks for by name.
const (
	// SDKGuideIndex is the guide's compact capability index.
	SDKGuideIndex = "capabilities.md"
	// SDKGuideStart is where the guide starts.
	SDKGuideStart = "index.md"
)

// ErrSDKGuidePageNotFound is returned for a page the guide does not have.
var ErrSDKGuidePageNotFound = errors.New("sdk guide page not found")

// ErrSDKGuideUnavailable is returned when no toolchain (and so no guide) is
// configured, or it cannot be read.
var ErrSDKGuideUnavailable = errors.New("sdk guide unavailable")

// SDKGuidePage is one page of the SDK guide that belongs to the toolchain
// Play builds with, in Markdown; ToolchainVersion identifies that toolchain.
type SDKGuidePage struct {
	Name             string
	Content          string
	ToolchainVersion string
}
