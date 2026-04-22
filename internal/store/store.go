package store

import (
	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
)

type Store struct {
	flags      map[string]*eval.CompiledFlag
	generation uint64
	version    int
}

func New(flags ...*eval.CompiledFlag) *Store {
	return NewWithGeneration(0, flags...)
}

func NewWithGeneration(generation uint64, flags ...*eval.CompiledFlag) *Store {
	byKey := make(map[string]*eval.CompiledFlag, len(flags))
	version := 0
	for _, flag := range flags {
		if flag == nil {
			continue
		}
		byKey[flag.Key] = cloneFlag(flag)
		if flag.Version > version {
			version = flag.Version
		}
	}

	return &Store{
		flags:      byKey,
		generation: generation,
		version:    version,
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

func (s *Store) Generation() uint64 {
	if s == nil {
		return 0
	}
	return s.generation
}

func (s *Store) Version() int {
	if s == nil {
		return 0
	}
	return s.version
}

func (s *Store) withGeneration(generation uint64) *Store {
	if s == nil {
		return NewWithGeneration(generation)
	}

	return &Store{
		flags:      s.flags,
		generation: generation,
		version:    s.version,
	}
}

func cloneFlag(flag *eval.CompiledFlag) *eval.CompiledFlag {
	clone := *flag
	clone.Rules = append([]eval.CompiledRule(nil), flag.Rules...)
	clone.Rollout = append([]eval.WeightedVariant(nil), flag.Rollout...)
	return &clone
}
