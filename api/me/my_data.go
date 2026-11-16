package me

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/diegobermudez03/playhoot/api/internal/auth"
	"github.com/diegobermudez03/playhoot/api/internal/httpx"
	"github.com/diegobermudez03/playhoot/composer/mydata"
	"github.com/diegobermudez03/playhoot/identity"
	"github.com/diegobermudez03/playhoot/logging"
)

const maxBodyBytes = 1 << 10

type methodResponse struct {
	MethodID string `json:"method_id"`
	Issuer   string `json:"issuer"`
	Address  string `json:"address"`
	Name     string `json:"name"`
	AddedAt  string `json:"added_at"`
}

type profileResponse struct {
	UserUUID    string           `json:"user_uuid"`
	DisplayName string           `json:"display_name"`
	Level       string           `json:"level"`
	Mode        string           `json:"mode"`
	Language    string           `json:"language"`
	Methods     []methodResponse `json:"methods"`
}

// declarationResponse is what the caller declared. It is null when they
// declared nothing, and declaration_known is false when Compliance could not
// be asked: the screen then shows it as unknown.
type declarationResponse struct {
	Country      string `json:"country"`
	Subdivision  string `json:"subdivision"`
	MeetsAge     bool   `json:"meets_age"`
	ThresholdAge int    `json:"threshold_age"`
	DeclaredAt   string `json:"declared_at"`
}

type limitsResponse struct {
	SavedGames         int `json:"saved_games"`
	CreationsPerWindow int `json:"creations_per_window"`
	SessionsPerWindow  int `json:"sessions_per_window"`
	WindowDays         int `json:"window_days"`
	PlayersPerSession  int `json:"players_per_session"`
	SimultaneousRooms  int `json:"simultaneous_rooms"`
	StorageMB          int `json:"storage_mb"`
}

type myDataResponse struct {
	Profile profileResponse `json:"profile"`
	// DeclarationKnown is false when the declaration could not be read at
	// all; Declaration is null when there is none to show.
	DeclarationKnown bool                 `json:"declaration_known"`
	Declaration      *declarationResponse `json:"declaration"`
	// LimitsKnown is false when the capacities could not be read.
	LimitsKnown bool            `json:"limits_known"`
	Limits      *limitsResponse `json:"limits"`
}

// handleMyData answers who the caller is, with whatever of their
// declaration and capacities could be read.
func (h *Handler) handleMyData(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	caller, ok := auth.Caller(ctx)
	if !ok {
		httpx.WriteError(w, http.StatusInternalServerError, "caller not resolved")
		return
	}
	data, err := h.data.ReadMyData(ctx, caller.UUID)
	if err != nil {
		logging.LogError(ctx, err)
		if errors.Is(err, identity.ErrUserNotFound) {
			httpx.WriteError(w, http.StatusInternalServerError, "caller not found")
			return
		}
		httpx.WriteError(w, http.StatusServiceUnavailable, "reading your data failed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	httpx.WriteJSON(w, http.StatusOK, toMyDataResponse(data))
}

func toMyDataResponse(data mydata.MyData) myDataResponse {
	methods := make([]methodResponse, 0, len(data.Profile.Methods))
	for _, method := range data.Profile.Methods {
		methods = append(methods, methodResponse{
			MethodID: method.MethodID, Issuer: string(method.Issuer), Address: method.Address,
			Name: method.Name, AddedAt: method.AddedAt.UTC().Format(time.RFC3339),
		})
	}
	answer := myDataResponse{
		Profile: profileResponse{
			UserUUID: data.Profile.UUID, DisplayName: data.Profile.DisplayName, Level: string(data.Profile.Level),
			Mode: string(identity.NormalizeMode(data.Profile.Mode)), Language: data.Profile.Language, Methods: methods,
		},
		DeclarationKnown: data.DeclarationKnown,
		LimitsKnown:      data.LimitsKnown,
	}
	if data.DeclarationKnown && data.Declaration.Found {
		answer.Declaration = &declarationResponse{
			Country: data.Declaration.Country, Subdivision: data.Declaration.Subdivision,
			MeetsAge: data.Declaration.MeetsAge, ThresholdAge: data.Declaration.ThresholdAge,
			DeclaredAt: data.Declaration.DeclaredAt.UTC().Format(time.RFC3339),
		}
	}
	if data.LimitsKnown {
		answer.Limits = &limitsResponse{
			SavedGames: data.Limits.SavedGames, CreationsPerWindow: data.Limits.CreationsPerWindow,
			SessionsPerWindow: data.Limits.SessionsPerWindow, WindowDays: data.Limits.WindowDays,
			PlayersPerSession: data.Limits.PlayersPerSession, SimultaneousRooms: data.Limits.SimultaneousRooms,
			StorageMB: data.Limits.StorageMB,
		}
	}
	return answer
}

func decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes)).Decode(into); err != nil {
		httpx.WriteError(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	return true
}
