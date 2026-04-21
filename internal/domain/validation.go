package domain

import (
	"fmt"
	"strings"
)

type ValidationError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type ValidationErrors []ValidationError

func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "validation failed"
	}

	messages := make([]string, 0, len(e))
	for _, err := range e {
		messages = append(messages, fmt.Sprintf("%s: %s", err.Field, err.Message))
	}

	return strings.Join(messages, "; ")
}

func (e ValidationErrors) Has(code string) bool {
	for _, err := range e {
		if err.Code == code {
			return true
		}
	}
	return false
}

func ValidateFlag(flag Flag) error {
	var errs ValidationErrors

	if strings.TrimSpace(flag.Key) == "" {
		errs = append(errs, validationError("key", "required", "flag key is required"))
	}
	if strings.TrimSpace(flag.Default) == "" {
		errs = append(errs, validationError("default", "required", "default variant is required"))
	}
	if flag.Version < 0 {
		errs = append(errs, validationError("version", "invalid", "version must be zero or greater"))
	}

	variantNames := make(map[string]struct{}, len(flag.Variants))
	totalWeight := 0

	if len(flag.Variants) == 0 {
		errs = append(errs, validationError("variants", "required", "at least one variant is required"))
	}

	for i, variant := range flag.Variants {
		field := fmt.Sprintf("variants[%d]", i)
		name := strings.TrimSpace(variant.Name)
		if name == "" {
			errs = append(errs, validationError(field+".name", "required", "variant name is required"))
		} else if _, exists := variantNames[name]; exists {
			errs = append(errs, validationError(field+".name", "duplicate", "variant name must be unique"))
		} else {
			variantNames[name] = struct{}{}
		}

		if variant.Weight < 0 {
			errs = append(errs, validationError(field+".weight", "invalid", "variant weight must be zero or greater"))
		}
		totalWeight += variant.Weight
	}

	if len(flag.Variants) > 0 && totalWeight != ExpectedWeightTotal {
		errs = append(errs, validationError("variants", "invalid_weight_total", fmt.Sprintf("variant weights must sum to %d", ExpectedWeightTotal)))
	}

	if flag.Default != "" {
		if _, exists := variantNames[flag.Default]; !exists {
			errs = append(errs, validationError("default", "unknown_variant", "default variant must exist in variants"))
		}
	}

	priorities := make(map[int]struct{}, len(flag.Rules))
	for i, rule := range flag.Rules {
		field := fmt.Sprintf("rules[%d]", i)
		errs = append(errs, validateRule(field, rule, variantNames, priorities)...)
	}

	if len(errs) > 0 {
		return errs
	}

	return nil
}

func validateRule(field string, rule Rule, variantNames map[string]struct{}, priorities map[int]struct{}) ValidationErrors {
	var errs ValidationErrors

	if strings.TrimSpace(rule.Attribute) == "" {
		errs = append(errs, validationError(field+".attribute", "required", "rule attribute is required"))
	}
	if !supportedOperator(rule.Operator) {
		errs = append(errs, validationError(field+".operator", "unsupported", "rule operator is not supported"))
	}
	if len(rule.Values) == 0 {
		errs = append(errs, validationError(field+".values", "required", "rule values are required"))
	}
	for i, value := range rule.Values {
		if strings.TrimSpace(value) == "" {
			errs = append(errs, validationError(fmt.Sprintf("%s.values[%d]", field, i), "required", "rule value is required"))
		}
	}
	if rule.Operator == OperatorEq && len(rule.Values) != 1 {
		errs = append(errs, validationError(field+".values", "invalid", "eq rules must have exactly one value"))
	}
	if strings.TrimSpace(rule.Variant) == "" {
		errs = append(errs, validationError(field+".variant", "required", "rule variant is required"))
	} else if _, exists := variantNames[rule.Variant]; !exists {
		errs = append(errs, validationError(field+".variant", "unknown_variant", "rule variant must exist in variants"))
	}
	if rule.Priority < 0 {
		errs = append(errs, validationError(field+".priority", "invalid", "rule priority must be zero or greater"))
	} else if _, exists := priorities[rule.Priority]; exists {
		errs = append(errs, validationError(field+".priority", "duplicate", "rule priority must be unique"))
	} else {
		priorities[rule.Priority] = struct{}{}
	}

	return errs
}

func supportedOperator(operator string) bool {
	switch operator {
	case OperatorEq, OperatorIn:
		return true
	default:
		return false
	}
}

func validationError(field string, code string, message string) ValidationError {
	return ValidationError{
		Field:   field,
		Code:    code,
		Message: message,
	}
}
