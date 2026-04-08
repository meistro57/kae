package ingestion

import (
	"net/http"
	"time"
)

// httpClient is shared across all ingestion sources.
// A 15-second timeout prevents any slow/hung source from stalling the engine.
var httpClient = &http.Client{Timeout: 15 * time.Second}
