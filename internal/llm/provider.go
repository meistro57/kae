package llm

// ChunkType classifies what a streaming Chunk contains.
type ChunkType int

const (
	ChunkText  ChunkType = iota // normal assistant output
	ChunkThink                  // reasoning / thinking content
	ChunkDone                   // stream finished cleanly
	ChunkError                  // stream encountered an error
)

// Chunk is one streaming token from a Provider.
type Chunk struct {
	Type ChunkType
	Text string
	Err  error
}

// Message is a single chat turn sent to a Provider.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Provider is the unified interface for every LLM backend.
// Stream sends a conversation and returns a channel of streaming chunks;
// the channel is closed after ChunkDone or ChunkError.
type Provider interface {
	Stream(system string, messages []Message) <-chan Chunk
	ModelName() string
}
