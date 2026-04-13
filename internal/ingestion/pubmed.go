package ingestion

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	pubmedSearchURL = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/esearch.fcgi"
	pubmedFetchURL  = "https://eutils.ncbi.nlm.nih.gov/entrez/eutils/efetch.fcgi"
)

// PubMedAbstract is a paper abstract from PubMed Central.
type PubMedAbstract struct {
	PMID  string
	Title string
	Text  string // full abstract text
	URL   string
}

// PubMedSearch fetches paper abstracts from PubMed for a topic.
// No API key required (rate limited to ~3 req/s).
// Best for neuroscience, cognitive science, and consciousness research —
// domains where KAE's anomaly detection often finds the most interesting gaps.
func PubMedSearch(topic string, limit int) ([]*PubMedAbstract, error) {
	ids, err := pubmedSearchIDs(topic, limit)
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Polite delay between search and fetch
	time.Sleep(350 * time.Millisecond)

	return pubmedFetchAbstracts(ids)
}

// pubmedSearchIDs returns PubMed IDs for a topic.
func pubmedSearchIDs(topic string, limit int) ([]string, error) {
	params := url.Values{
		"db":      {"pubmed"},
		"term":    {topic},
		"retmax":  {fmt.Sprintf("%d", limit)},
		"retmode": {"json"},
		"sort":    {"relevance"},
	}

	req, err := http.NewRequest("GET", pubmedSearchURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("pubmed search request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pubmed search fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pubmed search returned %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		ESearchResult struct {
			IDList []string `json:"idlist"`
		} `json:"esearchresult"`
	}

	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("pubmed search parse: %w", err)
	}

	return result.ESearchResult.IDList, nil
}

// pubmedFetchAbstracts fetches abstract text for a list of PMIDs.
func pubmedFetchAbstracts(ids []string) ([]*PubMedAbstract, error) {
	params := url.Values{
		"db":      {"pubmed"},
		"id":      {strings.Join(ids, ",")},
		"rettype": {"abstract"},
		"retmode": {"text"},
	}

	req, err := http.NewRequest("GET", pubmedFetchURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("pubmed fetch request: %w", err)
	}
	req.Header.Set("User-Agent", "KAE-Bot/1.0 (knowledge-archaeology-engine)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pubmed fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("pubmed fetch returned %d", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// PubMed returns plain text with papers separated by blank lines.
	// Each paper block starts with a PMID line and contains title + abstract.
	return parsePubMedText(string(b), ids), nil
}

// parsePubMedText splits the plain-text PubMed response into individual abstracts.
func parsePubMedText(text string, ids []string) []*PubMedAbstract {
	// Split on double newlines between records — each record starts with "1. " or similar
	blocks := strings.Split(text, "\n\n\n")
	abstracts := make([]*PubMedAbstract, 0, len(blocks))

	for i, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		pmid := ""
		if i < len(ids) {
			pmid = ids[i]
		}

		// Extract title: first non-empty line after stripping numbering
		lines := strings.Split(block, "\n")
		title := ""
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if l != "" && !strings.HasPrefix(l, "PMID:") {
				title = l
				break
			}
		}

		// The whole block is the abstract text — clean it up
		cleaned := strings.ReplaceAll(block, "\n", " ")
		cleaned = strings.Join(strings.Fields(cleaned), " ")

		if cleaned == "" {
			continue
		}

		abstracts = append(abstracts, &PubMedAbstract{
			PMID:  pmid,
			Title: title,
			Text:  cleaned,
			URL:   fmt.Sprintf("https://pubmed.ncbi.nlm.nih.gov/%s/", pmid),
		})
	}

	return abstracts
}

// PubMedToChunks converts a PubMedAbstract to embeddable chunks.
func PubMedToChunks(a *PubMedAbstract) []string {
	full := a.Text
	if a.Title != "" && !strings.Contains(full, a.Title) {
		full = "Title: " + a.Title + "\n\n" + full
	}
	return Chunk(full, 200, 30)
}
