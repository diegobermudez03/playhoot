package codec

import (
	"encoding/json"

	"github.com/diegobermudez03/playhoot/game/language/v1/program"
)

type wirePlayerPolicy struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

func encodePlayerPolicy(path string, value program.PlayerPolicy) (json.RawMessage, error) {
	return json.Marshal(wirePlayerPolicy{
		Min: value.Min,
		Max: value.Max,
	})
}

func decodePlayerPolicy(path string, data json.RawMessage) (program.PlayerPolicy, error) {
	if len(data) == 0 {
		return program.PlayerPolicy{}, nil
	}
	var wire wirePlayerPolicy
	if err := decodeOrdinaryObject(path, data, &wire); err != nil {
		return program.PlayerPolicy{}, err
	}
	return program.PlayerPolicy{
		Min: wire.Min,
		Max: wire.Max,
	}, nil
}
