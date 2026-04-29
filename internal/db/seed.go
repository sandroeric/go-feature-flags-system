package db

import (
	"context"
	"errors"

	"launchdarkly/internal/domain"
)

func DevelopmentSeedFlag() domain.Flag {
	return domain.Flag{
		Key:     "checkout_flow",
		Enabled: true,
		Default: "off",
		Variants: []domain.Variant{
			{Name: "off", Weight: 50},
			{Name: "on", Weight: 50},
		},
		Rules: []domain.Rule{
			{
				Attribute: "country",
				Operator:  domain.OperatorEq,
				Values:    []string{"BR"},
				Variant:   "on",
				Priority:  1,
			},
		},
		Version: 1,
	}
}

func SeedDevelopment(ctx context.Context, repo *Repository) error {
	seed := DevelopmentSeedFlag()
	if _, err := repo.GetFlag(ctx, seed.Key); err == nil {
		return nil
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}

	_, err := repo.CreateFlag(ctx, seed)
	return err
}
