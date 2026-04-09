package ingestion

// SourceChunk is a piece of text from any ingestion source
type SourceChunk struct {
	Text   string
	Source string // URL or title
	Topic  string // concept this relates to
}
