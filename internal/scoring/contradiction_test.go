package scoring

import "testing"

func TestScoreComputesWeightedValuesAndAnomaly(t *testing.T) {
	t.Parallel()

	ev := []Evidence{
		{Source: "s1", Stance: StanceSupport, Weight: 1.0},
		{Source: "s2", Stance: StanceContradict, Weight: 2.0},
		{Source: "s3", Stance: StanceSilent, Weight: 1.0},
	}

	cs := Score("test claim", ev)

	if cs.Total != 4.0 {
		t.Fatalf("expected total=4.0 got %f", cs.Total)
	}
	if cs.Support != 1.0 || cs.Contradiction != 2.0 || cs.Silence != 1.0 {
		t.Fatalf("unexpected weights: %+v", cs)
	}
	if !cs.IsAnomaly {
		t.Fatal("expected anomaly due to contradiction dominating support")
	}
	if cs.Explanation == "" {
		t.Fatal("expected explanation to be populated")
	}
}

func TestScoreDefaultsZeroWeightToOne(t *testing.T) {
	t.Parallel()

	ev := []Evidence{{Source: "s1", Stance: StanceSupport, Weight: 0}}
	cs := Score("claim", ev)
	if cs.Total != 1.0 || cs.Support != 1.0 {
		t.Fatalf("expected implicit weight=1.0, got total=%f support=%f", cs.Total, cs.Support)
	}
}

func TestClassifyStance(t *testing.T) {
	t.Parallel()

	claim := "quantum consciousness"
	cases := []struct {
		name    string
		passage string
		want    Stance
	}{
		{
			name:    "contradiction",
			passage: "There is no evidence for quantum consciousness and the idea is debunked.",
			want:    StanceContradict,
		},
		{
			name:    "silence",
			passage: "Quantum effects in brains remains unclear and further research is needed.",
			want:    StanceSilent,
		},
		{
			name:    "support",
			passage: "Quantum consciousness may explain aspects of cognition.",
			want:    StanceSupport,
		},
		{
			name:    "irrelevant",
			passage: "Bananas are yellow and fruit salads are lovely.",
			want:    StanceSilent,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := ClassifyStance(claim, tc.passage)
			if got != tc.want {
				t.Fatalf("expected %v, got %v", tc.want, got)
			}
		})
	}
}
