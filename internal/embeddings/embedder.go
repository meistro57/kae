// Package embeddings provides a lightweight, API-free embedding using feature
// hashing (random indexing). No external service required.
package embeddings

import (
	"hash/fnv"
	"math"
	"strings"
)

// Dim is the vector dimension.
const Dim = 128

// Embed converts text to a normalised float32 vector using feature hashing.
// Words are mapped deterministically to dimensions; the result is L2-normalised.
func Embed(text string) []float32 {
	vec := make([]float32, Dim)
	words := strings.Fields(strings.ToLower(text))
	if len(words) == 0 {
		return vec
	}
	for _, word := range words {
		// position hash
		h1 := fnv.New32a()
		h1.Write([]byte(word))
		idx := int(h1.Sum32()%uint32(Dim))

		// sign hash
		h2 := fnv.New32a()
		h2.Write([]byte(word + "\x00"))
		if h2.Sum32()%2 == 0 {
			vec[idx] += 1
		} else {
			vec[idx] -= 1
		}
	}
	// L2 normalise
	var norm float64
	for _, v := range vec {
		norm += float64(v) * float64(v)
	}
	if norm > 0 {
		scale := float32(1.0 / math.Sqrt(norm))
		for i := range vec {
			vec[i] *= scale
		}
	}
	return vec
}
