package llm

import (
	"strings"
	"testing"
)

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

func TestValidateSummaryCoverage(t *testing.T) {
	result := &SummaryResult{
		Topics: []SummaryTopicResult{{SourceIndexes: []int{0, 2}}},
		ExcludedSources: []ExcludedSourceResult{{
			SourceIndex: 1,
			Reason:      "реклама",
		}},
	}
	if err := ValidateSummaryCoverage(result, 3); err != nil {
		t.Fatalf("ValidateSummaryCoverage() error = %v", err)
	}
}

func TestValidateSummaryCoverageRejectsMissingSource(t *testing.T) {
	result := &SummaryResult{Topics: []SummaryTopicResult{{SourceIndexes: []int{0}}}}
	err := ValidateSummaryCoverage(result, 2)
	if err == nil || !strings.Contains(err.Error(), "[1]") {
		t.Fatalf("ValidateSummaryCoverage() error = %v, want missing index 1", err)
	}
}

func TestValidateSummaryCoverageRejectsUsedAndExcludedSource(t *testing.T) {
	result := &SummaryResult{
		Topics:          []SummaryTopicResult{{SourceIndexes: []int{0}}},
		ExcludedSources: []ExcludedSourceResult{{SourceIndex: 0, Reason: "реклама"}},
	}
	err := ValidateSummaryCoverage(result, 1)
	if err == nil || !strings.Contains(err.Error(), "both used and excluded") {
		t.Fatalf("ValidateSummaryCoverage() error = %v, want overlap error", err)
	}
}
