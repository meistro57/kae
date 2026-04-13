package collections

// FindingType classifies what kind of synthesis finding Lens produced.
type FindingType string

const (
	FindingConnection    FindingType = "connection"    // unexpected cross-domain link
	FindingContradiction FindingType = "contradiction" // conflicting claims between nodes
	FindingCluster       FindingType = "cluster"       // emergent concept group not tagged by KAE
	FindingAnomaly       FindingType = "anomaly"       // outlier or mainstream consensus break
)

// LensFinding represents a point in the kae_lens_findings Qdrant collection.
// This is written and read exclusively by Lens.
type LensFinding struct {
	Type           FindingType `json:"type"`
	Confidence     float64     `json:"confidence"`
	SourcePointIDs []string    `json:"source_point_ids"`
	Domains        []string    `json:"domains"`
	Summary        string      `json:"summary"`
	ReasoningTrace string      `json:"reasoning_trace"`
	// Correction is a data-grounded resolution produced for anomaly and
	// contradiction findings. Empty for connection and cluster types.
	Correction    string `json:"correction,omitempty"`
	EmbeddingText string `json:"embedding_text"`
	CreatedAt     int64  `json:"created_at"`
	Reviewed      bool   `json:"reviewed"`
	BatchID       string `json:"batch_id"`
}

// LensFindingsCollectionName is the Qdrant collection name for Lens synthesis findings.
const LensFindingsCollectionName = "kae_lens_findings"

// LensFindingVectorSize matches the knowledge collection embedding dimension.
const LensFindingVectorSize = 1536
