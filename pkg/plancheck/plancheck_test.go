package plancheck

import "testing"

func TestFailsBudget(t *testing.T) {
	const maxCost, factor, minDelta = 100000.0, 1.5, 100.0

	cases := []struct {
		name        string
		cost        float64
		baseline    float64
		hasBaseline bool
		want        bool
	}{
		{"incident regression", 7931, 152, true, true}, // 52x jump, the real case
		{"within factor", 200, 152, true, false},
		{"over factor but tiny delta", 12, 8, true, false}, // 1.5x but +4 < minDelta
		{"new view under ceiling", 8000, 0, false, false},
		{"new view over ceiling", 150000, 0, false, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := failsBudget(c.cost, c.baseline, c.hasBaseline, maxCost, factor, minDelta); got != c.want {
				t.Fatalf("failsBudget(%v,%v,%v)=%v want %v", c.cost, c.baseline, c.hasBaseline, got, c.want)
			}
		})
	}
}

func TestParsePlanCost(t *testing.T) {
	raw := []byte(`[{"Plan": {"Node Type": "Aggregate", "Total Cost": 42.5}}]`)
	cost, err := parsePlanCost(raw)
	if err != nil {
		t.Fatal(err)
	}
	if cost != 42.5 {
		t.Fatalf("got %v want 42.5", cost)
	}
	if _, err := parsePlanCost([]byte(`[]`)); err == nil {
		t.Fatal("expected error on empty plan")
	}
}
