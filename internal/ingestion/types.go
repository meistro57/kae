package ingestion

// SourceChunk is a piece of text from any ingestion source
type SourceChunk struct {
	Text             string
	Source           string  // URL or title
	RunTopic         string  // what the run is currently exploring (run-level metadata)
	SemanticDomain   string  // what this chunk is actually about (content classification)
	DomainConfidence float64 // classifier confidence 0.0–1.0
}
