package store

import (
	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
)

type Store struct {
	flags map[string]*eval.CompiledFlag
}

func New(flags ...*eval.CompiledFlag) *Store {
	byKey := make(map[string]*eval.CompiledFlag, len(flags))
	for _, flag := range flags {
		if flag == nil {
			continue
		}
		byKey[flag.Key] = cloneFlag(flag)
	}

	return &Store{
		flags: byKey,
	}
}

func Empty() *Store {
	return New()
}

func (s *Store) GetFlag(key string) (*eval.CompiledFlag, bool) {
	if s == nil {
		return nil, false
	}

	flag, ok := s.flags[key]
	return flag, ok
}

func (s *Store) Evaluate(flagKey string, ctx *domain.Context) (string, bool) {
	flag, ok := s.GetFlag(flagKey)
	if !ok {
		return "", false
	}

	return eval.Evaluate(flag, ctx), true
}

func (s *Store) Len() int {
	if s == nil {
		return 0
	}
	return len(s.flags)
}

func cloneFlag(flag *eval.CompiledFlag) *eval.CompiledFlag {
	clone := *flag
	clone.Rules = append([]eval.CompiledRule(nil), flag.Rules...)
	clone.Rollout = append([]eval.WeightedVariant(nil), flag.Rollout...)
	return &clone
}
