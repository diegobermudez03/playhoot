package program

import "encoding/json"

type wireMetadata struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	Version         string `json:"version"`
	LanguageVersion string `json:"language_version"`
}

// encodeMetadata encodes value, an ordinary (non-interface) struct, as its
// JSON wire representation. Every field is always emitted, including
// empty strings.
func encodeMetadata(path string, value Metadata) (json.RawMessage, error) {
	return json.Marshal(wireMetadata{
		ID:              value.ID,
		Name:            value.Name,
		Description:     value.Description,
		Version:         value.Version,
		LanguageVersion: value.LanguageVersion,
	})
}

// decodeMetadata decodes data as a Metadata. Because Metadata is an
// ordinary struct rather than a closed interface, JSON null is not a valid
// encoding of it and produces a path-aware structural error instead of a
// silent zero value.
func decodeMetadata(path string, data json.RawMessage) (Metadata, error) {
	var wire wireMetadata
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return Metadata{}, err
	}
	return Metadata{
		ID:              wire.ID,
		Name:            wire.Name,
		Description:     wire.Description,
		Version:         wire.Version,
		LanguageVersion: wire.LanguageVersion,
	}, nil
}
