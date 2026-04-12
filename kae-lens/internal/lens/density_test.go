package lens

import (
	"testing"

	"github.com/meistro/kae/internal/config"
)

// testDensityConfig returns a config matching the defaults in lens.yaml.
func testDensityConfig() *config.LensConfig {
	return &config.LensConfig{
		Density: config.DensityConfig{
			DensityBuckets: config.DensityBuckets{
				VerySparseMax: 0,
				SparseMax:     10,
				MediumMax:     50,
				DenseMax:      200,
			},
			Thresholds: config.DensityThresholds{
				VerySparseWidth: 50,
				SparseWidth:     35,
				MediumWidth:     20,
				DenseWidth:      12,
				VeryDenseWidth:  6,
			},
			ScoreThresholds: config.DensityScoreThresholds{
				Sparse: 0.60,
				Dense:  0.80,
			},
		},
	}
}

func TestClassify(t *testing.T) {
	dc := &DensityCalculator{cfg: testDensityConfig()}
	// Medium threshold is computed as (sparse+dense)/2 — derive it the same way
	// the implementation does to avoid float32 literal precision mismatches.
	mediumThreshold := (float32(0.60) + float32(0.80)) / 2

	tests := []struct {
		count         int
		wantLabel     string
		wantWidth     int
		wantThreshold float32
	}{
		{0, "very_sparse", 50, 0.60},
		{1, "sparse", 35, 0.60},
		{10, "sparse", 35, 0.60},
		{11, "medium", 20, mediumThreshold},
		{50, "medium", 20, mediumThreshold},
		{51, "dense", 12, 0.80},
		{200, "dense", 12, 0.80},
		{201, "very_dense", 6, 0.80},
		{9999, "very_dense", 6, 0.80},
	}

	for _, tt := range tests {
		t.Run(tt.wantLabel, func(t *testing.T) {
			p := dc.classify(tt.count)
			if p.Label != tt.wantLabel {
				t.Errorf("count=%d: got label %q, want %q", tt.count, p.Label, tt.wantLabel)
			}
			if p.SearchWidth != tt.wantWidth {
				t.Errorf("count=%d: got width %d, want %d", tt.count, p.SearchWidth, tt.wantWidth)
			}
			if p.ScoreThreshold != tt.wantThreshold {
				t.Errorf("count=%d: got threshold %.2f, want %.2f", tt.count, p.ScoreThreshold, tt.wantThreshold)
			}
			if p.NearbyCount != tt.count {
				t.Errorf("count=%d: NearbyCount not set correctly, got %d", tt.count, p.NearbyCount)
			}
		})
	}
}

func TestMediumProfileThreshold(t *testing.T) {
	dc := &DensityCalculator{cfg: testDensityConfig()}
	p := dc.mediumProfile(25)

	// Medium threshold must be exactly the midpoint of sparse and dense
	want := (float32(0.60) + float32(0.80)) / 2
	if p.ScoreThreshold != want {
		t.Errorf("medium threshold: got %.3f, want %.3f", p.ScoreThreshold, want)
	}
	if p.Label != "medium" {
		t.Errorf("medium label: got %q", p.Label)
	}
}
