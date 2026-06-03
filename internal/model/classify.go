package model

// ClassifyTier maps an issue state + nearby keyword to a display tier.
func ClassifyTier(state, keyword string) Tier {
	switch state {
	case "open":
		return TierOpen
	case "gone":
		return TierGone
	case "closed", "merged":
		if keyword != "" {
			return TierStale
		}
		return TierClosed
	}
	return TierUnknown
}
