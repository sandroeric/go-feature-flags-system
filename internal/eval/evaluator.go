package eval

import "launchdarkly/internal/domain"

const (
	fnvOffset32 = 2166136261
	fnvPrime32  = 16777619
)

func Evaluate(flag *CompiledFlag, ctx *domain.Context) string {
	if flag == nil {
		return ""
	}
	if !flag.Enabled {
		return flag.Default
	}
	if ctx == nil {
		return flag.Default
	}

	for _, rule := range flag.Rules {
		if rule.Match(ctx) {
			return rule.Variant
		}
	}

	if ctx.UserID == "" || flag.totalWeight <= 0 {
		return flag.Default
	}

	b := bucket(ctx.UserID, flag.Key) % uint32(flag.totalWeight)
	return pickVariant(flag, b)
}

func bucket(userID string, flagKey string) uint32 {
	h := uint32(fnvOffset32)
	h = writeString(h, userID)
	h = writeByte(h, ':')
	h = writeString(h, flagKey)
	return h
}

func writeString(hash uint32, value string) uint32 {
	for i := 0; i < len(value); i++ {
		hash = writeByte(hash, value[i])
	}
	return hash
}

func writeByte(hash uint32, value byte) uint32 {
	hash ^= uint32(value)
	hash *= fnvPrime32
	return hash
}

func pickVariant(flag *CompiledFlag, bucket uint32) string {
	var cumulative uint32
	for _, variant := range flag.Rollout {
		cumulative += uint32(variant.Weight)
		if bucket < cumulative {
			return variant.Name
		}
	}

	return flag.Default
}
