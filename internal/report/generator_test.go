package report

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateMarkdownLabelsGrossSavingsWhenControllerTokensUnknown(t *testing.T) {
	now := time.Now()
	text, err := NewGenerator(t.TempDir()).GenerateMarkdown(&ReportData{
		TaskID:                "token-test",
		Status:                "completed",
		TotalTokens:           1000,
		EstimatedDirectTokens: 3000,
		EstimatedGrossSavings: 2000,
		StartedAt:             now,
		FinishedAt:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Estimated gross token savings") ||
		!strings.Contains(text, "net savings are unknown") {
		t.Fatalf("report = %s", text)
	}
}

func TestGenerateMarkdownShowsNetSavingsWithObservedControllerTokens(t *testing.T) {
	now := time.Now()
	text, err := NewGenerator(t.TempDir()).GenerateMarkdown(&ReportData{
		TaskID:                "token-test",
		Status:                "completed",
		TotalTokens:           1000,
		ControllerTokens:      500,
		EstimatedDirectTokens: 3000,
		EstimatedNetSavings:   1500,
		StartedAt:             now,
		FinishedAt:            now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "Controller tokens (observed)") ||
		!strings.Contains(text, "Estimated net token savings") {
		t.Fatalf("report = %s", text)
	}
}
