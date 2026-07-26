package service

import (
	"encoding/json"

	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
)

// sanitizeSingleMachineInbound removes legacy multi-node and notification
// metadata before an inbound reaches persistence or the local runtime.
func sanitizeSingleMachineInbound(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}

	if inbound.Settings != "" {
		var settings map[string]any
		if err := json.Unmarshal([]byte(inbound.Settings), &settings); err != nil {
			return err
		}
		if clients, ok := settings["clients"].([]any); ok {
			for _, raw := range clients {
				client, ok := raw.(map[string]any)
				if !ok {
					continue
				}
				delete(client, "group")
				delete(client, "tgId")
				delete(client, "limitIp")
			}
		}
		encoded, err := json.MarshalIndent(settings, "", "  ")
		if err != nil {
			return err
		}
		inbound.Settings = string(encoded)
	}

	if inbound.StreamSettings != "" {
		var stream map[string]any
		if err := json.Unmarshal([]byte(inbound.StreamSettings), &stream); err != nil {
			return err
		}
		delete(stream, "externalProxy")
		encoded, err := json.MarshalIndent(stream, "", "  ")
		if err != nil {
			return err
		}
		inbound.StreamSettings = string(encoded)
	}
	return nil
}
