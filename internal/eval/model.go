package eval

import "launchdarkly/internal/domain"

type CompiledFlag struct {
	Key     string
	Enabled bool
	Default string
	Version int

	Rules   []CompiledRule
	Rollout []WeightedVariant

	totalWeight int
}

type CompiledRule struct {
	Match    func(*domain.Context) bool
	Variant  string
	Priority int
}

type WeightedVariant struct {
	Name   string
	Weight int
}
