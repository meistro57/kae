package lens

import (
	"context"
	"log"

	"github.com/meistro/kae/internal/config"
	qdrantclient "github.com/meistro/kae/internal/qdrantclient"
)

// DensityProfile is the result of a density assessment for a vector.
type DensityProfile struct {
	// NearbyCount is how many points were found near this vector.
	NearbyCount int
	// SearchWidth is the recommended number of neighbors to retrieve.
	SearchWidth int
	// ScoreThreshold is the minimum similarity score to use when searching.
	ScoreThreshold float32
	// Label is a human-readable density classification.
	Label string
}

// DensityCalculator assesses the local density of a vector in the knowledge
// collection and returns adaptive search parameters for the Reasoner.
type DensityCalculator struct {
	cfg *config.LensConfig
	qc  *qdrantclient.Client
}

// NewDensityCalculator creates a DensityCalculator.
func NewDensityCalculator(cfg *config.LensConfig, qc *qdrantclient.Client) *DensityCalculator {
	return &DensityCalculator{cfg: cfg, qc: qc}
}

// Assess probes the local density around a vector and returns adaptive
// search parameters. The probe uses a fixed high-threshold (0.85) to
// count genuinely close neighbors before deciding how wide to cast the net.
func (d *DensityCalculator) Assess(ctx context.Context, collection string, vector []float32) (*DensityProfile, error) {
	const probeThreshold = float32(0.85)

	count, err := d.qc.CountNearby(ctx, collection, vector, probeThreshold)
	if err != nil {
		// Non-fatal: fall back to medium defaults
		log.Printf("[density] probe failed, using medium defaults: %v", err)
		return d.mediumProfile(0), nil
	}

	return d.classify(count), nil
}

// classify maps a nearby-count to a DensityProfile using configured thresholds.
func (d *DensityCalculator) classify(count int) *DensityProfile {
	b := d.cfg.Density.DensityBuckets
	t := d.cfg.Density.Thresholds
	s := d.cfg.Density.ScoreThresholds

	switch {
	case count <= b.VerySparseMax:
		return &DensityProfile{
			NearbyCount:    count,
			SearchWidth:    t.VerySparseWidth,
			ScoreThreshold: s.Sparse,
			Label:          "very_sparse",
		}
	case count <= b.SparseMax:
		return &DensityProfile{
			NearbyCount:    count,
			SearchWidth:    t.SparseWidth,
			ScoreThreshold: s.Sparse,
			Label:          "sparse",
		}
	case count <= b.MediumMax:
		return d.mediumProfile(count)
	case count <= b.DenseMax:
		return &DensityProfile{
			NearbyCount:    count,
			SearchWidth:    t.DenseWidth,
			ScoreThreshold: s.Dense,
			Label:          "dense",
		}
	default:
		return &DensityProfile{
			NearbyCount:    count,
			SearchWidth:    t.VeryDenseWidth,
			ScoreThreshold: s.Dense,
			Label:          "very_dense",
		}
	}
}

func (d *DensityCalculator) mediumProfile(count int) *DensityProfile {
	t := d.cfg.Density.Thresholds
	// interpolate score threshold between sparse and dense for medium zones
	threshold := (d.cfg.Density.ScoreThresholds.Sparse + d.cfg.Density.ScoreThresholds.Dense) / 2
	return &DensityProfile{
		NearbyCount:    count,
		SearchWidth:    t.MediumWidth,
		ScoreThreshold: threshold,
		Label:          "medium",
	}
}
