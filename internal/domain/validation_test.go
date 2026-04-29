package domain

import (
	"errors"
	"testing"
)

func TestValidateFlagAcceptsValidFlag(t *testing.T) {
	flag := validFlag()

	if err := ValidateFlag(flag); err != nil {
		t.Fatalf("ValidateFlag() error = %v", err)
	}
}

func TestValidateFlagRequiresKey(t *testing.T) {
	flag := validFlag()
	flag.Key = " "

	errs := validateAndCollect(t, flag)

	if !errs.Has("required") {
		t.Fatalf("validation errors = %v, want required", errs)
	}
}

func TestValidateFlagRequiresDefaultVariantToExist(t *testing.T) {
	flag := validFlag()
	flag.Default = "missing"

	errs := validateAndCollect(t, flag)

	if !errs.Has("unknown_variant") {
		t.Fatalf("validation errors = %v, want unknown_variant", errs)
	}
}

func TestValidateFlagRequiresUniqueVariantNames(t *testing.T) {
	flag := validFlag()
	flag.Variants = append(flag.Variants, Variant{Name: "on", Weight: 0})

	errs := validateAndCollect(t, flag)

	if !errs.Has("duplicate") {
		t.Fatalf("validation errors = %v, want duplicate", errs)
	}
}

func TestValidateFlagRejectsNegativeVariantWeight(t *testing.T) {
	flag := validFlag()
	flag.Variants[0].Weight = -1
	flag.Variants[1].Weight = 101

	errs := validateAndCollect(t, flag)

	if !errs.Has("invalid") {
		t.Fatalf("validation errors = %v, want invalid", errs)
	}
}

func TestValidateFlagRequiresWeightsToSumToExpectedTotal(t *testing.T) {
	flag := validFlag()
	flag.Variants[0].Weight = 10
	flag.Variants[1].Weight = 20

	errs := validateAndCollect(t, flag)

	if !errs.Has("invalid_weight_total") {
		t.Fatalf("validation errors = %v, want invalid_weight_total", errs)
	}
}

func TestValidateFlagRejectsUnsupportedRuleOperator(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Operator = "contains"

	errs := validateAndCollect(t, flag)

	if !errs.Has("unsupported") {
		t.Fatalf("validation errors = %v, want unsupported", errs)
	}
}

func TestValidateFlagRequiresRuleVariantToExist(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Variant = "missing"

	errs := validateAndCollect(t, flag)

	if !errs.Has("unknown_variant") {
		t.Fatalf("validation errors = %v, want unknown_variant", errs)
	}
}

func TestValidateFlagRejectsInvalidRulePriority(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Priority = -1

	errs := validateAndCollect(t, flag)

	if !errs.Has("invalid") {
		t.Fatalf("validation errors = %v, want invalid", errs)
	}
}

func TestValidateFlagRequiresUniqueRulePriorities(t *testing.T) {
	flag := validFlag()
	flag.Rules = append(flag.Rules, Rule{
		Attribute: "plan",
		Operator:  OperatorEq,
		Values:    []string{"pro"},
		Variant:   "on",
		Priority:  1,
	})

	errs := validateAndCollect(t, flag)

	if !errs.Has("duplicate") {
		t.Fatalf("validation errors = %v, want duplicate", errs)
	}
}

func TestValidateFlagRequiresEqRulesToHaveOneValue(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Values = []string{"BR", "US"}

	errs := validateAndCollect(t, flag)

	if !errs.Has("invalid") {
		t.Fatalf("validation errors = %v, want invalid", errs)
	}
}

func TestValidateFlagAllowsInRulesToHaveMultipleValues(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Operator = OperatorIn
	flag.Rules[0].Values = []string{"BR", "US"}

	if err := ValidateFlag(flag); err != nil {
		t.Fatalf("ValidateFlag() error = %v", err)
	}
}

func validateAndCollect(t *testing.T, flag Flag) ValidationErrors {
	t.Helper()

	err := ValidateFlag(flag)
	if err == nil {
		t.Fatal("ValidateFlag() error = nil, want validation errors")
	}

	var errs ValidationErrors
	if !errors.As(err, &errs) {
		t.Fatalf("ValidateFlag() error = %T, want ValidationErrors", err)
	}

	return errs
}

func validFlag() Flag {
	return Flag{
		Key:     "checkout",
		Enabled: true,
		Default: "off",
		Variants: []Variant{
			{Name: "off", Weight: 50},
			{Name: "on", Weight: 50},
		},
		Rules: []Rule{
			{
				Attribute: "country",
				Operator:  OperatorEq,
				Values:    []string{"BR"},
				Variant:   "on",
				Priority:  1,
			},
		},
		Version: 1,
	}
}
