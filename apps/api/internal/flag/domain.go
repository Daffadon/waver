package flag

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"regexp"
	"strings"
	"time"

	"golang.org/x/mod/semver"
)

var keyRe = regexp.MustCompile(`^[a-z0-9][a-z0-9\-_.]*$`)

// Rule targets one boolean value. First match wins.
type Rule struct {
	ID         string   `json:"id"`
	Attr       string   `json:"attr"`
	Op         string   `json:"op"`
	Values     []string `json:"values"`
	Value      bool     `json:"value"`
	RolloutPct int      `json:"rollout_pct"`
}

// Flag is boolean-only v1.
type Flag struct {
	Key          string `json:"key"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	Enabled      bool   `json:"enabled"`
	DefaultValue bool   `json:"default_value"`
	Rules        []Rule `json:"rules"`
	StartsAt     *int64 `json:"starts_at,omitempty"`
	EndsAt       *int64 `json:"ends_at,omitempty"`
	// Version is this flag's revision: the winning Lamport clock. It is what a
	// client sends back to save with a conflict check.
	Version   int64 `json:"version"`
	UpdatedAt int64 `json:"updated_at"`
}

// Result is the OFREP evaluation outcome.
type Result struct {
	Value   bool
	Reason  string
	Variant string
}

func ValidateKey(k string) error {
	if !keyRe.MatchString(k) || len(k) > 128 {
		return fmt.Errorf("bad key: use [a-z0-9-_.], max 128")
	}
	return nil
}

func ValidateRules(raw []Rule) error {
	seen := map[string]bool{}
	for _, r := range raw {
		if r.ID == "" || seen[r.ID] {
			return fmt.Errorf("rule id must be unique and non-empty")
		}
		seen[r.ID] = true
		if r.Attr == "" {
			return fmt.Errorf("rule %s: attr required", r.ID)
		}
		switch r.Op {
		case "eq", "ne", "in", "not_in", "contains", "starts_with", "ends_with", "gt", "lt", "semver_eq":
		default:
			return fmt.Errorf("rule %s: bad op %s", r.ID, r.Op)
		}
		if len(r.Values) == 0 {
			return fmt.Errorf("rule %s: values required", r.ID)
		}
		if r.RolloutPct < 0 || r.RolloutPct > 100 {
			return fmt.Errorf("rule %s: rollout_pct 0-100", r.ID)
		}
	}
	return nil
}

// Eval applies kill-switch, UTC schedule, then first-match rules.
func (f Flag) Eval(ctx map[string]any, now time.Time) Result {
	now = now.UTC()
	if !f.Enabled {
		return Result{Value: f.DefaultValue, Reason: "DISABLED", Variant: variantOf(f.DefaultValue)}
	}
	if f.StartsAt != nil && now.Unix() < *f.StartsAt {
		return Result{Value: f.DefaultValue, Reason: "DISABLED", Variant: variantOf(f.DefaultValue)}
	}
	if f.EndsAt != nil && now.Unix() > *f.EndsAt {
		return Result{Value: f.DefaultValue, Reason: "DISABLED", Variant: variantOf(f.DefaultValue)}
	}
	targetingKey, _ := ctx["targetingKey"].(string)
	for _, r := range f.Rules {
		got := stringify(ctx[r.Attr])
		if !matchOp(r.Op, got, r.Values) {
			continue
		}
		if r.RolloutPct < 100 {
			if targetingKey == "" || bucket(f.Key, targetingKey) >= r.RolloutPct {
				continue
			}
		}
		return Result{Value: r.Value, Reason: "TARGETING_MATCH", Variant: variantOf(r.Value)}
	}
	return Result{Value: f.DefaultValue, Reason: "DEFAULT", Variant: variantOf(f.DefaultValue)}
}

func variantOf(b bool) string {
	if b {
		return "on"
	}
	return "off"
}

// bucket hashes flag+targetingKey to 0-99. Stable per subject.
func bucket(flagKey, subject string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(flagKey + ":" + subject))
	return int(h.Sum32() % 100)
}

func stringify(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		// ponytail: shortest stable form, upgrade path is canonical JSON encode
		s := fmt.Sprintf("%v", t)
		return strings.TrimSuffix(strings.TrimSuffix(s, "0"), ".")
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, _ := json.Marshal(t)
		return string(b)
	}
}

func matchOp(op, got string, want []string) bool {
	switch op {
	case "eq":
		return got == want[0]
	case "ne":
		return got != want[0]
	case "in":
		for _, w := range want {
			if got == w {
				return true
			}
		}
		return false
	case "not_in":
		for _, w := range want {
			if got == w {
				return false
			}
		}
		return true
	case "contains":
		return strings.Contains(got, want[0])
	case "starts_with":
		return strings.HasPrefix(got, want[0])
	case "ends_with":
		return strings.HasSuffix(got, want[0])
	case "gt", "lt":
		if len(want) == 0 {
			return false
		}
		if op == "gt" {
			return got > want[0]
		}
		return got < want[0]
	case "semver_eq":
		g, w := normSemver(got), normSemver(want[0])
		if !semver.IsValid(g) || !semver.IsValid(w) {
			return false
		}
		return semver.Compare(g, w) == 0
	}
	return false
}

func normSemver(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "v") {
		s = "v" + s
	}
	return s
}
