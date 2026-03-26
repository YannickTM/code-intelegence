package handler

import (
	"strings"
	"testing"
	"time"
)

func TestBuildCommitListQuery_NoOptionalFilters(t *testing.T) {
	p := commitQueryParams{
		ProjectID: testUUID(1),
		Limit:     20,
		Offset:    0,
	}
	dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(p)

	if !strings.Contains(dataSQL, "project_id = $1") {
		t.Error("data query missing project_id filter")
	}
	if strings.Contains(dataSQL, "ILIKE") {
		t.Error("data query should not contain ILIKE without search")
	}
	if strings.Contains(dataSQL, "committer_date >=") {
		t.Error("data query should not contain from_date filter")
	}
	if strings.Contains(dataSQL, "committer_date <=") {
		t.Error("data query should not contain to_date filter")
	}
	if !strings.Contains(dataSQL, "LIMIT $2 OFFSET $3") {
		t.Errorf("data query missing LIMIT/OFFSET, got:\n%s", dataSQL)
	}
	if len(dataArgs) != 3 {
		t.Errorf("dataArgs len = %d, want 3", len(dataArgs))
	}
	if len(countArgs) != 1 {
		t.Errorf("countArgs len = %d, want 1", len(countArgs))
	}
	if !strings.Contains(countSQL, "project_id = $1") {
		t.Error("count query missing project_id filter")
	}
	if strings.Contains(countSQL, "LIMIT") {
		t.Error("count query should not contain LIMIT")
	}
}

func TestBuildCommitListQuery_WithSearch(t *testing.T) {
	p := commitQueryParams{
		ProjectID: testUUID(1),
		Search:    "feat",
		Limit:     20,
		Offset:    0,
	}
	dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(p)

	if !strings.Contains(dataSQL, "message ILIKE $2") {
		t.Errorf("data query missing ILIKE clause, got:\n%s", dataSQL)
	}
	if len(dataArgs) != 4 {
		t.Errorf("dataArgs len = %d, want 4", len(dataArgs))
	}
	if dataArgs[1] != "%feat%" {
		t.Errorf("dataArgs[1] = %v, want %%feat%%", dataArgs[1])
	}
	if len(countArgs) != 2 {
		t.Errorf("countArgs len = %d, want 2", len(countArgs))
	}
	if !strings.Contains(countSQL, "message ILIKE $2") {
		t.Error("count query missing ILIKE clause")
	}
}

func TestBuildCommitListQuery_WithFromDate(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	p := commitQueryParams{
		ProjectID: testUUID(1),
		FromDate:  from,
		Limit:     20,
		Offset:    0,
	}
	dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(p)

	if !strings.Contains(dataSQL, "committer_date >= $2") {
		t.Errorf("data query missing from_date clause, got:\n%s", dataSQL)
	}
	if len(dataArgs) != 4 {
		t.Errorf("dataArgs len = %d, want 4", len(dataArgs))
	}
	if dataArgs[1] != from {
		t.Errorf("dataArgs[1] = %v, want %v", dataArgs[1], from)
	}
	if len(countArgs) != 2 {
		t.Errorf("countArgs len = %d, want 2", len(countArgs))
	}
	if !strings.Contains(countSQL, "committer_date >= $2") {
		t.Error("count query missing from_date clause")
	}
}

func TestBuildCommitListQuery_WithToDate(t *testing.T) {
	to := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
	p := commitQueryParams{
		ProjectID: testUUID(1),
		ToDate:    to,
		Limit:     20,
		Offset:    0,
	}
	dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(p)

	if !strings.Contains(dataSQL, "committer_date <= $2") {
		t.Errorf("data query missing to_date clause, got:\n%s", dataSQL)
	}
	if len(dataArgs) != 4 {
		t.Errorf("dataArgs len = %d, want 4", len(dataArgs))
	}
	if dataArgs[1] != to {
		t.Errorf("dataArgs[1] = %v, want %v", dataArgs[1], to)
	}
	if len(countArgs) != 2 {
		t.Errorf("countArgs len = %d, want 2", len(countArgs))
	}
	if !strings.Contains(countSQL, "committer_date <= $2") {
		t.Error("count query missing to_date clause")
	}
}

func TestBuildCommitListQuery_AllFiltersCombined(t *testing.T) {
	from := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2024, 6, 30, 23, 59, 59, 0, time.UTC)
	p := commitQueryParams{
		ProjectID: testUUID(1),
		Search:    "fix",
		FromDate:  from,
		ToDate:    to,
		Limit:     50,
		Offset:    10,
	}
	dataSQL, countSQL, dataArgs, countArgs := buildCommitListQuery(p)

	// WHERE: project_id=$1, ILIKE $2, >= $3, <= $4, LIMIT $5 OFFSET $6
	if !strings.Contains(dataSQL, "project_id = $1") {
		t.Error("missing project_id")
	}
	if !strings.Contains(dataSQL, "message ILIKE $2") {
		t.Error("missing ILIKE")
	}
	if !strings.Contains(dataSQL, "committer_date >= $3") {
		t.Error("missing from_date")
	}
	if !strings.Contains(dataSQL, "committer_date <= $4") {
		t.Error("missing to_date")
	}
	if !strings.Contains(dataSQL, "LIMIT $5 OFFSET $6") {
		t.Errorf("missing LIMIT/OFFSET, got:\n%s", dataSQL)
	}
	if len(dataArgs) != 6 {
		t.Errorf("dataArgs len = %d, want 6", len(dataArgs))
	}
	if len(countArgs) != 4 {
		t.Errorf("countArgs len = %d, want 4", len(countArgs))
	}

	// Count query should have same WHERE but no LIMIT/OFFSET
	if !strings.Contains(countSQL, "project_id = $1") {
		t.Error("count missing project_id")
	}
	if !strings.Contains(countSQL, "message ILIKE $2") {
		t.Error("count missing ILIKE")
	}
	if !strings.Contains(countSQL, "committer_date >= $3") {
		t.Error("count missing from_date")
	}
	if !strings.Contains(countSQL, "committer_date <= $4") {
		t.Error("count missing to_date")
	}
	if strings.Contains(countSQL, "LIMIT") {
		t.Error("count query should not contain LIMIT")
	}
}

func TestBuildCommitListQuery_CountMatchesDataWhere(t *testing.T) {
	p := commitQueryParams{
		ProjectID: testUUID(1),
		Search:    "refactor",
		FromDate:  time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		Limit:     20,
		Offset:    0,
	}
	dataSQL, countSQL, _, _ := buildCommitListQuery(p)

	// Extract WHERE clause from data query (between "WHERE " and "\nORDER BY")
	dataWhere := dataSQL[strings.Index(dataSQL, "WHERE ")+6 : strings.Index(dataSQL, "\nORDER BY")]
	countWhere := countSQL[strings.LastIndex(countSQL, "WHERE ")+6:]

	if strings.TrimSpace(dataWhere) != strings.TrimSpace(countWhere) {
		t.Errorf("WHERE clauses differ:\ndata:  %q\ncount: %q", dataWhere, countWhere)
	}
}
