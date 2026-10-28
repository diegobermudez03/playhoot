package identity

import (
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	identitypkg "github.com/diegobermudez03/playhoot/identity"
)

// userResponse is who a credential stands for.
type userResponse struct {
	UserUUID    string `json:"user_uuid"`
	DisplayName string `json:"display_name"`
	Level       string `json:"level"`
}

// credentialsResponse is what every endpoint that opens or renews a session
// returns. The refresh token travels in the body, so web and mobile clients
// share one contract and keep it in their own storage.
type credentialsResponse struct {
	User            userResponse `json:"user"`
	AccessToken     string       `json:"access_token"`
	AccessExpiresAt string       `json:"access_expires_at"`
	// AccessExpiresIn is the access token's lifetime in seconds, so a client
	// whose clock differs from the server's can time its renewal.
	AccessExpiresIn int64  `json:"access_expires_in"`
	RefreshToken    string `json:"refresh_token"`
}

func toUserResponse(user identitypkg.User) userResponse {
	return userResponse{UserUUID: user.UUID, DisplayName: user.DisplayName, Level: string(user.Level)}
}

// writeCredentials writes credentials, never to be cached.
func writeCredentials(w http.ResponseWriter, status int, credentials identitypkg.Credentials) {
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, status, credentialsResponse{
		User:            toUserResponse(credentials.User),
		AccessToken:     credentials.AccessToken,
		AccessExpiresAt: credentials.AccessExpiresAt.UTC().Format(time.RFC3339),
		AccessExpiresIn: int64(credentials.AccessExpiresAt.Sub(credentials.AccessIssuedAt) / time.Second),
		RefreshToken:    credentials.RefreshToken,
	})
}

// complete reports whether credentials carry everything a client needs.
func complete(credentials identitypkg.Credentials) bool {
	return credentials.User.UUID != "" && credentials.AccessToken != "" && credentials.RefreshToken != ""
}
