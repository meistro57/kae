package embeddings

import "testing"

func TestCosineSimilarity(t *testing.T) {
	t.Parallel()

	t.Run("identical vectors", func(t *testing.T) {
		t.Parallel()
		a := []float32{1, 2, 3}
		b := []float32{1, 2, 3}
		sim := CosineSimilarity(a, b)
		if sim < 0.9999 || sim > 1.0001 {
			t.Fatalf("expected similarity close to 1.0, got %f", sim)
		}
	})

	t.Run("orthogonal vectors", func(t *testing.T) {
		t.Parallel()
		a := []float32{1, 0}
		b := []float32{0, 1}
		sim := CosineSimilarity(a, b)
		if sim != 0 {
			t.Fatalf("expected 0 similarity, got %f", sim)
		}
	})

	t.Run("different lengths", func(t *testing.T) {
		t.Parallel()
		a := []float32{1, 2}
		b := []float32{1, 2, 3}
		sim := CosineSimilarity(a, b)
		if sim != 0 {
			t.Fatalf("expected 0 similarity for mismatched lengths, got %f", sim)
		}
	})
}

func TestSqrt(t *testing.T) {
	t.Parallel()

	if got := sqrt(0); got != 0 {
		t.Fatalf("expected sqrt(0)=0, got %f", got)
	}

	got := sqrt(9)
	if got < 2.9999 || got > 3.0001 {
		t.Fatalf("expected sqrt(9) close to 3, got %f", got)
	}
}
