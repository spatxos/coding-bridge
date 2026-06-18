package cli

import "testing"

func TestSelectReportRejectsInvalidTaskIDBeforeLookup(t *testing.T) {
	if _, err := selectReport(nil, "bad/id"); err == nil {
		t.Fatal("selectReport() accepted a path-like task ID")
	}
}

func TestSelectReportFindsTaskReport(t *testing.T) {
	reports := []string{
		`C:\project\.coding-bridge\reports\other-20260618-report.md`,
		`C:\project\.coding-bridge\reports\task-1-20260618-report.md`,
	}
	report, err := selectReport(reports, "task-1")
	if err != nil {
		t.Fatalf("selectReport() error = %v", err)
	}
	if report != reports[1] {
		t.Fatalf("report = %q, want %q", report, reports[1])
	}
}
