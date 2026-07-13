package llm

import "testing"

func TestParseSummaryResultValidatesJSON(t *testing.T) {
	_, err := ParseSummaryResult([]byte(`{"title":"Digest","overview":"Overview","topics":[{"title":"Topic","category":"Go","short_summary":"Short","full_summary":"Full","why_important":"Important","confidence":"high","importance":9,"source_indexes":[0]}]}`))
	if err != nil {
		t.Fatalf("ParseSummaryResult() error = %v", err)
	}

	_, err = ParseSummaryResult([]byte(`{"title":"Digest","overview":"Overview","topics":[{"title":"Topic","short_summary":"Short","full_summary":"Full","confidence":"certain","importance":99}]}`))
	if err == nil {
		t.Fatal("ParseSummaryResult() error = nil, want validation error")
	}
}

func TestParseArticleResultValidatesJSON(t *testing.T) {
	_, err := ParseArticleResult([]byte(`{"title":"Guide","type":"guide","tags":["go"],"content_markdown":"# Guide"}`))
	if err != nil {
		t.Fatalf("ParseArticleResult() error = %v", err)
	}

	_, err = ParseArticleResult([]byte(`{"title":"Guide","type":"unknown","content_markdown":"# Guide"}`))
	if err == nil {
		t.Fatal("ParseArticleResult() error = nil, want validation error")
	}
}
