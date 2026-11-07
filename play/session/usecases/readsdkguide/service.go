// Package readsdkguide answers pages of the SDK guide for agents from the
// toolchain Play builds with, with that toolchain's version.
package readsdkguide

import (
	"context"

	"github.com/diegobermudez03/playhoot/logging"
	sessionpkg "github.com/diegobermudez03/playhoot/play/session"
	"github.com/diegobermudez03/playhoot/play/session/internal/sdkguide"
)

// toolchain identifies the toolchain builds use.
type toolchain interface {
	Version(ctx context.Context) (string, error)
}

// pages reads the guide's pages.
type pages interface {
	Read(page string) (string, error)
}

// Service is the SDK guide use case.
type Service struct {
	pages     pages
	toolchain toolchain
}

// New constructs a Service on the toolchain checkout at root, whose version
// toolchain tells.
func New(root string, toolchain toolchain) *Service {
	return &Service{pages: sdkguide.New(root), toolchain: toolchain}
}

// ReadPage answers page of the guide (sessionpkg.SDKGuideIndex for the
// capability index). An unknown page is sessionpkg.ErrSDKGuidePageNotFound;
// no toolchain is sessionpkg.ErrSDKGuideUnavailable.
func (s *Service) ReadPage(ctx context.Context, page string) (sessionpkg.SDKGuidePage, error) {
	defer logging.Step(ctx, "readsdkguide.read_page").Close()
	logging.LogFields(ctx, logging.Field("sdk_page", page))

	content, err := s.pages.Read(page)
	if err != nil {
		return sessionpkg.SDKGuidePage{}, err
	}
	// The version names the toolchain; a guide read without it is still the
	// guide, so its failure only leaves it out.
	version, err := s.toolchain.Version(ctx)
	if err != nil {
		logging.LogError(ctx, err)
		version = ""
	}
	return sessionpkg.SDKGuidePage{Name: page, Content: content, ToolchainVersion: version}, nil
}
