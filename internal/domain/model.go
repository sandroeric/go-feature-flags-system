package domain

const (
	ExpectedWeightTotal = 100

	OperatorEq = "eq"
	OperatorIn = "in"
)

type Flag struct {
	Key      string    `json:"key"`
	Enabled  bool      `json:"enabled"`
	Default  string    `json:"default"`
	Variants []Variant `json:"variants"`
	Rules    []Rule    `json:"rules"`
	Version  int       `json:"version"`
}

type Variant struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

type Rule struct {
	Attribute string   `json:"attribute"`
	Operator  string   `json:"operator"`
	Values    []string `json:"values"`
	Variant   string   `json:"variant"`
	Priority  int      `json:"priority"`
}

type Context struct {
	UserID  string            `json:"user_id"`
	Country string            `json:"country,omitempty"`
	Plan    string            `json:"plan,omitempty"`
	Custom  map[string]string `json:"custom,omitempty"`
}
