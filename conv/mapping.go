package conv

import (
	_ "embed"
	"encoding/json"
	log "github.com/sirupsen/logrus"
)

var replacement = map[string]uint32{}

type BedrockMapping struct {
	BedrockIdentifier string         `json:"bedrock_identifier"`
	BedrockStates     map[string]any `json:"bedrock_states,omitempty"`
}

var mappings BedrockMapping
var overrideBlockMappings map[string]uint32

//go:embed blocks.json
var blocksJSON []byte

//go:embed default_block_states.json
var blockStates []byte

func init() {
	{
		err := json.Unmarshal(blocksJSON, &mappings)
		if err != nil {
			log.Error("error", err)
			return
		}
	}
	{
		err := json.Unmarshal(blockStates, &overrideBlockMappings)
		if err != nil {
			log.Error("error", err)
			return
		}
	}

}

func LoadReplacement(f []byte) {
	{
		err := json.Unmarshal(f, &replacement)
		if err != nil {
			log.Error("error", err)
			return
		}
	}
}
