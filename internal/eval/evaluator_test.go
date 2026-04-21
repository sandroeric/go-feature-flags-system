package eval

import (
	"errors"
	"testing"

	"launchdarkly/internal/domain"
)

func TestCompileFlagRejectsInvalidFlag(t *testing.T) {
	flag := validFlag()
	flag.Default = "missing"

	_, err := CompileFlag(flag)
	if err == nil {
		t.Fatal("CompileFlag() error = nil, want error")
	}

	var validationErrors domain.ValidationErrors
	if !errors.As(err, &validationErrors) {
		t.Fatalf("CompileFlag() error = %T, want domain.ValidationErrors", err)
	}
}

func TestCompileFlagSortsRulesByPriority(t *testing.T) {
	flag := validFlag()
	flag.Rules = []domain.Rule{
		{
			Attribute: "country",
			Operator:  domain.OperatorEq,
			Values:    []string{"BR"},
			Variant:   "off",
			Priority:  20,
		},
		{
			Attribute: "plan",
			Operator:  domain.OperatorEq,
			Values:    []string{"pro"},
			Variant:   "on",
			Priority:  10,
		},
	}

	compiled := mustCompile(t, flag)

	if compiled.Rules[0].Priority != 10 {
		t.Fatalf("first rule priority = %d, want 10", compiled.Rules[0].Priority)
	}
}

func TestEvaluateDisabledFlagReturnsDefault(t *testing.T) {
	flag := validFlag()
	flag.Enabled = false
	compiled := mustCompile(t, flag)

	got := Evaluate(compiled, &domain.Context{
		UserID:  "123",
		Country: "BR",
	})

	if got != "off" {
		t.Fatalf("Evaluate() = %q, want %q", got, "off")
	}
}

func TestEvaluateReturnsMatchingRuleVariant(t *testing.T) {
	compiled := mustCompile(t, validFlag())

	got := Evaluate(compiled, &domain.Context{
		UserID:  "123",
		Country: "BR",
	})

	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestEvaluateUsesRulePriority(t *testing.T) {
	flag := validFlag()
	flag.Rules = []domain.Rule{
		{
			Attribute: "country",
			Operator:  domain.OperatorEq,
			Values:    []string{"BR"},
			Variant:   "off",
			Priority:  2,
		},
		{
			Attribute: "plan",
			Operator:  domain.OperatorEq,
			Values:    []string{"pro"},
			Variant:   "on",
			Priority:  1,
		},
	}
	compiled := mustCompile(t, flag)

	got := Evaluate(compiled, &domain.Context{
		UserID:  "123",
		Country: "BR",
		Plan:    "pro",
	})

	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestEvaluateSupportsInRules(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Operator = domain.OperatorIn
	flag.Rules[0].Values = []string{"BR", "US"}
	compiled := mustCompile(t, flag)

	got := Evaluate(compiled, &domain.Context{
		UserID:  "123",
		Country: "US",
	})

	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestEvaluateSupportsCustomAttributes(t *testing.T) {
	flag := validFlag()
	flag.Rules[0].Attribute = "region"
	flag.Rules[0].Values = []string{"latam"}
	compiled := mustCompile(t, flag)

	got := Evaluate(compiled, &domain.Context{
		UserID: "123",
		Custom: map[string]string{
			"region": "latam",
		},
	})

	if got != "on" {
		t.Fatalf("Evaluate() = %q, want %q", got, "on")
	}
}

func TestEvaluateMissingUserIDReturnsDefault(t *testing.T) {
	flag := validFlag()
	flag.Rules = nil
	compiled := mustCompile(t, flag)

	got := Evaluate(compiled, &domain.Context{})

	if got != "off" {
		t.Fatalf("Evaluate() = %q, want %q", got, "off")
	}
}

func TestBucketIsDeterministic(t *testing.T) {
	first := bucket("user-123", "checkout")
	second := bucket("user-123", "checkout")

	if first != second {
		t.Fatalf("bucket() = %d then %d, want stable result", first, second)
	}
}

func TestBucketUsesFlagKey(t *testing.T) {
	first := bucket("user-123", "checkout")
	second := bucket("user-123", "search")

	if first == second {
		t.Fatalf("bucket() returned %d for different flag keys", first)
	}
}

func TestEvaluateRolloutIsDeterministic(t *testing.T) {
	flag := validFlag()
	flag.Rules = nil
	compiled := mustCompile(t, flag)
	ctx := &domain.Context{UserID: "user-123"}

	first := Evaluate(compiled, ctx)
	second := Evaluate(compiled, ctx)

	if first != second {
		t.Fatalf("Evaluate() = %q then %q, want stable result", first, second)
	}
}

func TestPickVariantUsesWeights(t *testing.T) {
	flag := &CompiledFlag{
		Default:     "off",
		totalWeight: 100,
		Rollout: []WeightedVariant{
			{Name: "off", Weight: 25},
			{Name: "on", Weight: 75},
		},
	}

	tests := []struct {
		name   string
		bucket uint32
		want   string
	}{
		{name: "first range", bucket: 24, want: "off"},
		{name: "second range start", bucket: 25, want: "on"},
		{name: "second range end", bucket: 99, want: "on"},
		{name: "out of range fallback", bucket: 100, want: "off"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pickVariant(flag, tt.bucket)
			if got != tt.want {
				t.Fatalf("pickVariant() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvaluateHandlesNilInputs(t *testing.T) {
	if got := Evaluate(nil, &domain.Context{UserID: "123"}); got != "" {
		t.Fatalf("Evaluate(nil, ctx) = %q, want empty string", got)
	}

	compiled := mustCompile(t, validFlag())
	if got := Evaluate(compiled, nil); got != "off" {
		t.Fatalf("Evaluate(flag, nil) = %q, want %q", got, "off")
	}
}

func BenchmarkEvaluateRuleMatch(b *testing.B) {
	compiled := mustCompile(b, validFlag())
	ctx := &domain.Context{
		UserID:  "user-123",
		Country: "BR",
	}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Evaluate(compiled, ctx)
	}
}

func BenchmarkEvaluateRollout(b *testing.B) {
	flag := validFlag()
	flag.Rules = nil
	compiled := mustCompile(b, flag)
	ctx := &domain.Context{UserID: "user-123"}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Evaluate(compiled, ctx)
	}
}

func mustCompile(tb testing.TB, flag domain.Flag) *CompiledFlag {
	tb.Helper()

	compiled, err := CompileFlag(flag)
	if err != nil {
		tb.Fatalf("CompileFlag() error = %v", err)
	}

	return compiled
}

func validFlag() domain.Flag {
	return domain.Flag{
		Key:     "checkout",
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
