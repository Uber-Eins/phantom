package service

import (
	"encoding/json"
	"regexp"

	"github.com/Uber-Eins/phantom/v3/internal/util/common"

	"github.com/google/uuid"
)

var xmcProfileUsernamePattern = regexp.MustCompile(`^[A-Za-z0-9_]{3,16}$`)

// xmcMaskProfilesComplete reports whether an XMC finalmask carries the signed
// Minecraft session profiles xray-core has required since v26.7.28.
func xmcMaskProfilesComplete(mask map[string]any) bool {
	settings, ok := mask["settings"].(map[string]any)
	if !ok {
		return false
	}
	profiles, _ := settings["profiles"].([]any)
	if len(profiles) == 0 {
		return false
	}
	for _, entry := range profiles {
		profile, ok := entry.(map[string]any)
		if !ok {
			return false
		}
		username, _ := profile["username"].(string)
		if !xmcProfileUsernamePattern.MatchString(username) {
			return false
		}
		id, _ := profile["uuid"].(string)
		if _, err := uuid.Parse(id); err != nil {
			return false
		}
		if value, _ := profile["texturesValue"].(string); value == "" {
			return false
		}
		if signature, _ := profile["texturesSignature"].(string); signature == "" {
			return false
		}
	}
	return true
}

func isIncompleteXmcMask(entry any) bool {
	mask, ok := entry.(map[string]any)
	if !ok {
		return false
	}
	if maskType, _ := mask["type"].(string); maskType != "xmc" {
		return false
	}
	return !xmcMaskProfilesComplete(mask)
}

func incompleteXmcMaskCount(stream map[string]any) int {
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return 0
	}
	tcp, _ := finalmask["tcp"].([]any)
	count := 0
	for _, entry := range tcp {
		if isIncompleteXmcMask(entry) {
			count++
		}
	}
	return count
}

// stripIncompleteXmcMasks protects the whole generated config from legacy or
// incomplete XMC masks that the current core refuses to build.
func stripIncompleteXmcMasks(stream map[string]any) int {
	finalmask, ok := stream["finalmask"].(map[string]any)
	if !ok {
		return 0
	}
	tcp, _ := finalmask["tcp"].([]any)
	if len(tcp) == 0 {
		return 0
	}
	kept := make([]any, 0, len(tcp))
	dropped := 0
	for _, entry := range tcp {
		if isIncompleteXmcMask(entry) {
			dropped++
			continue
		}
		kept = append(kept, entry)
	}
	if dropped == 0 {
		return 0
	}
	if len(kept) == 0 {
		delete(finalmask, "tcp")
	} else {
		finalmask["tcp"] = kept
	}
	if len(finalmask) == 0 {
		delete(stream, "finalmask")
	}
	return dropped
}

func validateFinalMaskXmcProfiles(streamSettings string) error {
	if streamSettings == "" {
		return nil
	}
	var stream map[string]any
	if err := json.Unmarshal([]byte(streamSettings), &stream); err != nil {
		return nil
	}
	if incompleteXmcMaskCount(stream) == 0 {
		return nil
	}
	return common.NewError(
		"XMC finalmask requires at least one complete Minecraft profile — each needs a username (3-16 of A-Z a-z 0-9 _), a UUID, and both texture fields from Mojang's session server (XTLS/Xray-core#6487). Complete the profiles or remove the XMC mask.",
	)
}
