package collections

// KnowledgePoint represents a point in the kae_knowledge Qdrant collection.
// This is written by KAE and read by Lens.
type KnowledgePoint struct {
	Title         string  `json:"title"`
	Content       string  `json:"content"`
	Source        string  `json:"source"`
	Domain        string  `json:"domain"`
	URL           string  `json:"url"`
	IngestedAt    int64   `json:"ingested_at"`
	LensProcessed bool    `json:"lens_processed"`
	AnomalyScore  float64 `json:"anomaly_score"`
	ChunkIndex    int     `json:"chunk_index"`
	ParentID      string  `json:"parent_id"`
}

// KnowledgeCollectionName is the Qdrant collection name for KAE ingested knowledge.
const KnowledgeCollectionName = "kae_knowledge"

// KnowledgeVectorSize is the embedding dimension for knowledge points.
const KnowledgeVectorSize = 1536
