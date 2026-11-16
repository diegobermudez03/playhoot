// Package me is the route group of the caller's own data: the one read that
// says who they are, the two settings only they decide — the mode their
// account is in and the language they read Playhoot in — and the end of
// their identity.
package me

import (
	"context"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/routing"
	"github.com/diegobermudez03/playhoot/composer/mydata"
	"github.com/diegobermudez03/playhoot/identity"
)

//go:generate mockgen -package=me -destination=handler_mock_test.go . authorizer,dataReader,modeSetter,languageSetter,identityDeleter

// authorizer resolves an access token to its caller.
type authorizer interface {
	Authorize(ctx context.Context, token string, operation identity.Operation) (identity.User, identity.AccessDecision, error)
}

// dataReader composes everything the caller's own screens need.
type dataReader interface {
	ReadMyData(ctx context.Context, userUUID string) (mydata.MyData, error)
}

// modeSetter switches an account between its modes.
type modeSetter interface {
	SetMode(ctx context.Context, userUUID string, mode string) (identity.SetModeResult, error)
}

// languageSetter stores the language an account reads in.
type languageSetter interface {
	SetLanguage(ctx context.Context, userUUID string, language string) (identity.SetLanguageOutcome, error)
}

// identityDeleter ends the caller's own identity.
type identityDeleter interface {
	DeleteUser(ctx context.Context, userUUID string, origin identity.DeleteOrigin) (identity.DeleteUserOutcome, error)
}

// Dependencies are what this group needs.
type Dependencies struct {
	Data      dataReader
	Modes     modeSetter
	Languages languageSetter
	Deletion  identityDeleter
}

// Handler exposes the caller's own data.
type Handler struct {
	authorizer authorizer
	data       dataReader
	modes      modeSetter
	languages  languageSetter
	deletion   identityDeleter
}

// NewHandler constructs a Handler.
func NewHandler(authorizer authorizer, dependencies Dependencies) *Handler {
	return &Handler{
		authorizer: authorizer, data: dependencies.Data,
		modes: dependencies.Modes, languages: dependencies.Languages,
		deletion: dependencies.Deletion,
	}
}

// RESTRoutes returns this group's endpoints. Reading and deleting need only
// a guest's credential; the two settings belong to an account.
func (h *Handler) RESTRoutes() []routing.Route {
	return []routing.Route{
		{
			Pattern:       "GET /v1/me",
			Handler:       h.handleMyData,
			Authorization: auth.Require(h.authorizer, identity.OperationViewOwnIdentity),
		},
		{
			Pattern:       "PUT /v1/identity/me/mode",
			Handler:       h.handleSetMode,
			Authorization: auth.Require(h.authorizer, identity.OperationSetOwnMode),
		},
		{
			Pattern:       "PUT /v1/identity/me/language",
			Handler:       h.handleSetLanguage,
			Authorization: auth.Require(h.authorizer, identity.OperationSetInterfaceLanguage),
		},
		{
			Pattern:       "POST /v1/identity/me/delete",
			Handler:       h.handleDelete,
			Authorization: auth.Require(h.authorizer, identity.OperationDeleteOwnIdentity),
		},
	}
}

// WebSocketRoutes returns no endpoints: this group has no live ones.
func (h *Handler) WebSocketRoutes() []routing.Route {
	return nil
}
