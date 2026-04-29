package eval

import (
	"sort"

	"launchdarkly/internal/domain"
)

type CompileError struct {
	Err error
}

func (e CompileError) Error() string {
	return "compile flag: " + e.Err.Error()
}

func (e CompileError) Unwrap() error {
	return e.Err
}

func CompileFlag(flag domain.Flag) (*CompiledFlag, error) {
	if err := domain.ValidateFlag(flag); err != nil {
		return nil, CompileError{Err: err}
	}

	rules := append([]domain.Rule(nil), flag.Rules...)
	sort.Slice(rules, func(i, j int) bool {
		return rules[i].Priority < rules[j].Priority
	})

	compiled := &CompiledFlag{
		Key:     flag.Key,
		Enabled: flag.Enabled,
		Default: flag.Default,
		Version: flag.Version,
		Rules:   make([]CompiledRule, 0, len(rules)),
		Rollout: make([]WeightedVariant, 0, len(flag.Variants)),
	}

	for _, variant := range flag.Variants {
		compiled.Rollout = append(compiled.Rollout, WeightedVariant{
			Name:   variant.Name,
			Weight: variant.Weight,
		})
		compiled.totalWeight += variant.Weight
	}

	for _, rule := range rules {
		compiled.Rules = append(compiled.Rules, CompiledRule{
			Match:    compileRule(rule),
			Variant:  rule.Variant,
			Priority: rule.Priority,
		})
	}

	return compiled, nil
}

func compileRule(rule domain.Rule) func(*domain.Context) bool {
	get := compileAttributeGetter(rule.Attribute)

	switch rule.Operator {
	case domain.OperatorEq:
		expected := rule.Values[0]
		return func(ctx *domain.Context) bool {
			actual, ok := get(ctx)
			return ok && actual == expected
		}
	case domain.OperatorIn:
		values := append([]string(nil), rule.Values...)
		return func(ctx *domain.Context) bool {
			actual, ok := get(ctx)
			if !ok {
				return false
			}
			for _, value := range values {
				if actual == value {
					return true
				}
			}
			return false
		}
	default:
		return func(*domain.Context) bool {
			return false
		}
	}
}

func compileAttributeGetter(attribute string) func(*domain.Context) (string, bool) {
	switch attribute {
	case "user_id":
		return func(ctx *domain.Context) (string, bool) {
			if ctx == nil || ctx.UserID == "" {
				return "", false
			}
			return ctx.UserID, true
		}
	case "country":
		return func(ctx *domain.Context) (string, bool) {
			if ctx == nil || ctx.Country == "" {
				return "", false
			}
			return ctx.Country, true
		}
	case "plan":
		return func(ctx *domain.Context) (string, bool) {
			if ctx == nil || ctx.Plan == "" {
				return "", false
			}
			return ctx.Plan, true
		}
	default:
		return func(ctx *domain.Context) (string, bool) {
			if ctx == nil || ctx.Custom == nil {
				return "", false
			}
			value, ok := ctx.Custom[attribute]
			if !ok || value == "" {
				return "", false
			}
			return value, true
		}
	}
}
