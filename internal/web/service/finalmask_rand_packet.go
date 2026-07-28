package service

// dropEmptyRandPackets removes the leftover empty "packet" from finalmask
// items that also carry a rand, and reports how many it cleared.
//
// xray-core treats even an empty array as a packet, and every item kind is
// exclusive: noise refuses a packet plus a rand, while header-custom requires
// exactly one item kind. Either error rejects the whole generated config.
func dropEmptyRandPackets(node any) int {
	switch value := node.(type) {
	case map[string]any:
		cleared := 0
		if packet, ok := value["packet"].([]any); ok && len(packet) == 0 && randIsSet(value["rand"]) {
			delete(value, "packet")
			cleared++
		}
		for _, child := range value {
			cleared += dropEmptyRandPackets(child)
		}
		return cleared
	case []any:
		cleared := 0
		for _, child := range value {
			cleared += dropEmptyRandPackets(child)
		}
		return cleared
	default:
		return 0
	}
}

// randIsSet reports whether a finalmask item's rand selects a random packet.
// It is a number on header-custom items and a dash-range string on noise ones.
func randIsSet(value any) bool {
	switch rand := value.(type) {
	case float64:
		return rand > 0
	case string:
		return rand != "" && rand != "0" && rand != "0-0"
	default:
		return false
	}
}
