package qdrantclient

import (
	"testing"

	"github.com/qdrant/go-client/qdrant"
)

// ── PayloadToMap / valueToAny ─────────────────────────────────────────────────

func TestPayloadToMap_StringField(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"title": qdrant.NewValueString("Quantum Entanglement"),
	}
	m := PayloadToMap(payload)
	if m["title"] != "Quantum Entanglement" {
		t.Errorf("got %v, want %q", m["title"], "Quantum Entanglement")
	}
}

func TestPayloadToMap_IntField(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"cycle": qdrant.NewValueInt(42),
	}
	m := PayloadToMap(payload)
	if m["cycle"] != int64(42) {
		t.Errorf("got %v (%T), want int64(42)", m["cycle"], m["cycle"])
	}
}

func TestPayloadToMap_BoolField(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"lens_processed": qdrant.NewValueBool(true),
	}
	m := PayloadToMap(payload)
	if m["lens_processed"] != true {
		t.Errorf("got %v, want true", m["lens_processed"])
	}
}

func TestPayloadToMap_DoubleField(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"score": qdrant.NewValueDouble(0.87),
	}
	m := PayloadToMap(payload)
	if m["score"] != 0.87 {
		t.Errorf("got %v, want 0.87", m["score"])
	}
}

func TestPayloadToMap_NilValue(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"empty": nil,
	}
	m := PayloadToMap(payload)
	if m["empty"] != nil {
		t.Errorf("got %v, want nil", m["empty"])
	}
}

func TestPayloadToMap_ListValue(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"domains": {
			Kind: &qdrant.Value_ListValue{
				ListValue: &qdrant.ListValue{
					Values: []*qdrant.Value{
						qdrant.NewValueString("physics"),
						qdrant.NewValueString("philosophy"),
					},
				},
			},
		},
	}
	m := PayloadToMap(payload)
	list, ok := m["domains"].([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", m["domains"])
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 items, got %d", len(list))
	}
	if list[0] != "physics" || list[1] != "philosophy" {
		t.Errorf("got %v, want [physics philosophy]", list)
	}
}

func TestPayloadToMap_EmptyPayload(t *testing.T) {
	m := PayloadToMap(map[string]*qdrant.Value{})
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

func TestPayloadToJSON_RoundTrip(t *testing.T) {
	payload := map[string]*qdrant.Value{
		"type":       qdrant.NewValueString("connection"),
		"confidence": qdrant.NewValueDouble(0.91),
		"reviewed":   qdrant.NewValueBool(false),
	}
	b, err := PayloadToJSON(payload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(b) == 0 {
		t.Error("expected non-empty JSON output")
	}
	// Must be valid JSON
	s := string(b)
	if s[0] != '{' {
		t.Errorf("expected JSON object, got: %s", s)
	}
}
