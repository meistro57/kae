package qdrantclient

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/qdrant/go-client/qdrant"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Client wraps the Qdrant gRPC client with helpers for KAE/Lens operations.
type Client struct {
	inner *qdrant.Client
	cfg   Config
}

// Config holds connection parameters for the Qdrant client.
type Config struct {
	Host   string
	Port   int
	APIKey string
}

// New creates a new Qdrant client.
func New(cfg Config) (*Client, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	qcfg := &qdrant.Config{
		Host: cfg.Host,
		Port: cfg.Port,
	}
	if cfg.APIKey != "" {
		qcfg.APIKey = cfg.APIKey
		qcfg.UseTLS = true
	}

	// Apply dial options
	_ = opts // qdrant client handles its own dialing

	inner, err := qdrant.NewClient(qcfg)
	if err != nil {
		return nil, fmt.Errorf("creating qdrant client: %w", err)
	}

	return &Client{inner: inner, cfg: cfg}, nil
}

// Close shuts down the underlying gRPC connection.
func (c *Client) Close() error {
	return c.inner.Close()
}

// EnsureCollection creates a collection if it does not already exist.
func (c *Client) EnsureCollection(ctx context.Context, name string, vectorSize uint64) error {
	exists, err := c.inner.CollectionExists(ctx, name)
	if err != nil {
		return fmt.Errorf("checking collection existence: %w", err)
	}
	if exists {
		return nil
	}

	err = c.inner.CreateCollection(ctx, &qdrant.CreateCollection{
		CollectionName: name,
		VectorsConfig: qdrant.NewVectorsConfig(&qdrant.VectorParams{
			Size:     vectorSize,
			Distance: qdrant.Distance_Cosine,
		}),
	})
	if err != nil {
		return fmt.Errorf("creating collection %q: %w", name, err)
	}
	return nil
}

// CreatePayloadIndex creates an index on a payload field for fast filtering.
func (c *Client) CreatePayloadIndex(ctx context.Context, collection, field string, schemaType qdrant.FieldType) error {
	_, err := c.inner.CreateFieldIndex(ctx, &qdrant.CreateFieldIndexCollection{
		CollectionName: collection,
		FieldName:      field,
		FieldType:      &schemaType,
	})
	if err != nil {
		return fmt.Errorf("creating payload index on %s.%s: %w", collection, field, err)
	}
	return nil
}

// UpsertPoints upserts a batch of points into a collection.
func (c *Client) UpsertPoints(ctx context.Context, collection string, points []*qdrant.PointStruct) error {
	wait := true
	_, err := c.inner.Upsert(ctx, &qdrant.UpsertPoints{
		CollectionName: collection,
		Wait:           &wait,
		Points:         points,
	})
	if err != nil {
		return fmt.Errorf("upserting %d points to %q: %w", len(points), collection, err)
	}
	return nil
}

// ScrollUnprocessed scrolls for points where lens_processed is absent or false.
// Uses must_not so that points without the field (new KAE output) are included.
func (c *Client) ScrollUnprocessed(ctx context.Context, collection string, limit uint32) ([]*qdrant.RetrievedPoint, error) {
	trueVal := true
	result, err := c.inner.Scroll(ctx, &qdrant.ScrollPoints{
		CollectionName: collection,
		Filter: &qdrant.Filter{
			MustNot: []*qdrant.Condition{
				qdrant.NewMatchBool("lens_processed", trueVal),
			},
		},
		Limit:       &limit,
		WithPayload: qdrant.NewWithPayload(true),
		WithVectors: qdrant.NewWithVectors(true),
	})
	if err != nil {
		return nil, fmt.Errorf("scrolling unprocessed points: %w", err)
	}
	return result, nil
}

// MarkProcessed sets lens_processed=true on a list of point IDs.
// IDs may be UUIDs or decimal-string representations of uint64 numeric IDs.
func (c *Client) MarkProcessed(ctx context.Context, collection string, ids []string) error {
	pointIDs := make([]*qdrant.PointId, len(ids))
	for i, id := range ids {
		pointIDs[i] = ParsePointID(id)
	}

	wait := true
	_, err := c.inner.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: collection,
		Wait:           &wait,
		Payload: map[string]*qdrant.Value{
			"lens_processed": qdrant.NewValueBool(true),
		},
		PointsSelector: qdrant.NewPointsSelectorIDs(pointIDs),
	})
	if err != nil {
		return fmt.Errorf("marking %d points as processed: %w", len(ids), err)
	}
	return nil
}

// ClearProcessedFlags resets lens_processed to false for all points in the
// collection that are not Lens-generated correction chunks. This is used by
// the --reprocess manual run mode to force a full re-scan of the knowledge base.
func (c *Client) ClearProcessedFlags(ctx context.Context, collection string) error {
	wait := true
	_, err := c.inner.SetPayload(ctx, &qdrant.SetPayloadPoints{
		CollectionName: collection,
		Wait:           &wait,
		Payload: map[string]*qdrant.Value{
			"lens_processed": qdrant.NewValueBool(false),
		},
		// Apply to all points that are NOT Lens correction chunks.
		// Correction chunks (lens_correction=true) should never be re-processed.
		PointsSelector: &qdrant.PointsSelector{
			PointsSelectorOneOf: &qdrant.PointsSelector_Filter{
				Filter: &qdrant.Filter{
					MustNot: []*qdrant.Condition{
						qdrant.NewMatchBool("lens_correction", true),
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("clearing processed flags in %q: %w", collection, err)
	}
	return nil
}

// QueryNeighbors finds the top-N nearest neighbors of a given vector.
func (c *Client) QueryNeighbors(ctx context.Context, collection string, vector []float32, limit uint64, scoreThreshold float32) ([]*qdrant.ScoredPoint, error) {
	result, err := c.inner.Query(ctx, &qdrant.QueryPoints{
		CollectionName: collection,
		Query:          qdrant.NewQueryDense(vector),
		Limit:          &limit,
		ScoreThreshold: &scoreThreshold,
		WithPayload:    qdrant.NewWithPayload(true),
		WithVectors:    qdrant.NewWithVectors(false),
	})
	if err != nil {
		return nil, fmt.Errorf("querying neighbors: %w", err)
	}
	return result, nil
}

// CountNearby counts how many points exist near a vector above a score threshold.
// Used by the density calculator.
func (c *Client) CountNearby(ctx context.Context, collection string, vector []float32, scoreThreshold float32) (int, error) {
	limit := uint64(200) // cap the density probe
	results, err := c.QueryNeighbors(ctx, collection, vector, limit, scoreThreshold)
	if err != nil {
		return 0, err
	}
	return len(results), nil
}

// GetCollectionInfo returns point count and status for a collection.
func (c *Client) GetCollectionInfo(ctx context.Context, collection string) (*qdrant.CollectionInfo, error) {
	info, err := c.inner.GetCollectionInfo(ctx, collection)
	if err != nil {
		return nil, fmt.Errorf("getting collection info for %q: %w", collection, err)
	}
	return info, nil
}

// PayloadToMap converts a Qdrant payload map to a plain Go map for JSON marshaling.
func PayloadToMap(payload map[string]*qdrant.Value) map[string]any {
	result := make(map[string]any, len(payload))
	for k, v := range payload {
		result[k] = valueToAny(v)
	}
	return result
}

// PayloadToJSON marshals a Qdrant payload to JSON bytes.
func PayloadToJSON(payload map[string]*qdrant.Value) ([]byte, error) {
	return json.Marshal(PayloadToMap(payload))
}

// PointIDStr returns a stable string representation of a PointId,
// handling both UUID and numeric ID forms.
func PointIDStr(id *qdrant.PointId) string {
	if id == nil {
		return ""
	}
	if uuid := id.GetUuid(); uuid != "" {
		return uuid
	}
	return strconv.FormatUint(id.GetNum(), 10)
}

// ParsePointID parses a string ID back into a *qdrant.PointId.
// Numeric decimal strings become numeric IDs; anything else becomes a UUID.
func ParsePointID(s string) *qdrant.PointId {
	if n, err := strconv.ParseUint(s, 10, 64); err == nil {
		return qdrant.NewIDNum(n)
	}
	return qdrant.NewID(s)
}

func valueToAny(v *qdrant.Value) any {
	if v == nil {
		return nil
	}
	switch k := v.Kind.(type) {
	case *qdrant.Value_StringValue:
		return k.StringValue
	case *qdrant.Value_IntegerValue:
		return k.IntegerValue
	case *qdrant.Value_DoubleValue:
		return k.DoubleValue
	case *qdrant.Value_BoolValue:
		return k.BoolValue
	case *qdrant.Value_NullValue:
		return nil
	case *qdrant.Value_ListValue:
		items := make([]any, len(k.ListValue.Values))
		for i, item := range k.ListValue.Values {
			items[i] = valueToAny(item)
		}
		return items
	case *qdrant.Value_StructValue:
		m := make(map[string]any, len(k.StructValue.Fields))
		for fk, fv := range k.StructValue.Fields {
			m[fk] = valueToAny(fv)
		}
		return m
	default:
		return nil
	}
}

// WithAPIKeyContext attaches an API key to a context for gRPC metadata.
func WithAPIKeyContext(ctx context.Context, apiKey string) context.Context {
	if apiKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, "api-key", apiKey)
}
