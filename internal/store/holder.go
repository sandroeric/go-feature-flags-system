package store

import (
	"sync/atomic"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/eval"
)

type Holder struct {
	current atomic.Value // stores *Store
}

func NewHolder(initial *Store) *Holder {
	if initial == nil {
		initial = Empty()
	}

	holder := &Holder{}
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

	h.current.Store(newStore)
}

func (h *Holder) GetFlag(key string) (*eval.CompiledFlag, bool) {
	return h.Current().GetFlag(key)
}

func (h *Holder) Evaluate(flagKey string, ctx *domain.Context) (string, bool) {
	return h.Current().Evaluate(flagKey, ctx)
}
