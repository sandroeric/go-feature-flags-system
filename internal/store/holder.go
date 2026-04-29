package store

import (
	"sync/atomic"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
)

type Holder struct {
	current    atomic.Value // stores *Store
	generation atomic.Uint64
}

func NewHolder(initial *Store) *Holder {
	if initial == nil {
		initial = Empty()
	}

	holder := &Holder{}
	holder.generation.Store(initial.Generation())
	holder.current.Store(initial)
	return holder
}

func (h *Holder) Current() *Store {
	if h == nil {
		return Empty()
	}

	current := h.current.Load()
	if current == nil {
		return Empty()
	}

	return current.(*Store)
}

func (h *Holder) Swap(newStore *Store) {
	if newStore == nil {
		newStore = Empty()
	}

	generation := h.generation.Add(1)
	h.current.Store(newStore.withGeneration(generation))
}

func (h *Holder) GetFlag(key string) (*eval.CompiledFlag, bool) {
	return h.Current().GetFlag(key)
}

func (h *Holder) Evaluate(flagKey string, ctx *domain.Context) (string, bool) {
	return h.Current().Evaluate(flagKey, ctx)
}
