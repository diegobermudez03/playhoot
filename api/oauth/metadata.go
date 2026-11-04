package oauth

import (
	"net/http"

	"github.com/diegobermudez03/playhoot/identity"
)

// resourceMetadata is the MCP resource's Protected Resource Metadata (RFC
// 9728).
type resourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	ScopesSupported        []string `json:"scopes_supported"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ResourceName           string   `json:"resource_name"`
	ResourceDocumentation  string   `json:"resource_documentation,omitempty"`
}

// serverMetadata is the Authorization Server Metadata (RFC 8414).
type serverMetadata struct {
	Issuer                                   string   `json:"issuer"`
	AuthorizationEndpoint                    string   `json:"authorization_endpoint"`
	TokenEndpoint                            string   `json:"token_endpoint"`
	RegistrationEndpoint                     string   `json:"registration_endpoint"`
	RevocationEndpoint                       string   `json:"revocation_endpoint"`
	ResponseTypesSupported                   []string `json:"response_types_supported"`
	ResponseModesSupported                   []string `json:"response_modes_supported"`
	GrantTypesSupported                      []string `json:"grant_types_supported"`
	TokenEndpointAuthMethodsSupported        []string `json:"token_endpoint_auth_methods_supported"`
	RevocationEndpointAuthMethodsSupported   []string `json:"revocation_endpoint_auth_methods_supported"`
	CodeChallengeMethodsSupported            []string `json:"code_challenge_methods_supported"`
	ScopesSupported                          []string `json:"scopes_supported"`
	AuthorizationResponseISSParameterSupport bool     `json:"authorization_response_iss_parameter_supported"`
	ClientIDMetadataDocumentSupported        bool     `json:"client_id_metadata_document_supported"`
}

func (h *Handler) handleResourceMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	writePublicJSON(w, resourceMetadata{
		Resource: h.config.Resource, AuthorizationServers: []string{h.config.Issuer},
		ScopesSupported: []string{identity.AIScopeGames}, BearerMethodsSupported: []string{"header"},
		ResourceName: "Playhoot", ResourceDocumentation: h.config.Documentation,
	})
}

func (h *Handler) handleServerMetadata(w http.ResponseWriter, r *http.Request) {
	writePublicJSON(w, serverMetadata{
		Issuer:                                   h.config.Issuer,
		AuthorizationEndpoint:                    h.config.Issuer + "/oauth/authorize",
		TokenEndpoint:                            h.config.Issuer + "/oauth/token",
		RegistrationEndpoint:                     h.config.Issuer + "/oauth/register",
		RevocationEndpoint:                       h.config.Issuer + "/oauth/revoke",
		ResponseTypesSupported:                   []string{"code"},
		ResponseModesSupported:                   []string{"query"},
		GrantTypesSupported:                      []string{"authorization_code", "refresh_token"},
		TokenEndpointAuthMethodsSupported:        []string{"none"},
		RevocationEndpointAuthMethodsSupported:   []string{"none"},
		CodeChallengeMethodsSupported:            []string{"S256"},
		ScopesSupported:                          []string{identity.AIScopeGames},
		AuthorizationResponseISSParameterSupport: true,
		ClientIDMetadataDocumentSupported:        false,
	})
}

// writePublicJSON answers a discovery document, which any client may read
// and cache briefly.
func writePublicJSON(w http.ResponseWriter, body any) {
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = encodeJSON(w, body)
}
