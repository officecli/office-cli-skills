package sqlstore

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/officecli/officecli-internal/platform/internal/model"
)

func newIssueReportTestStore(t *testing.T, name string) *Store {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+name+"?mode=memory&cache=shared"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.IssueReport{}, &model.IssueReportRequestID{}))
	return NewWithDB(db)
}

func sampleIssueReport(clientEventID, ticketID string) *model.IssueReport {
	email := "user@example.com"
	return &model.IssueReport{
		TicketID:      ticketID,
		ClientEventID: clientEventID,
		SchemaVersion: 1,
		RuntimeMode:   "hosted",
		Category:      "conversion",
		Severity:      "high",
		Summary:       "PDF export fails",
		Detail:        "Error when exporting to PDF",
		ContactEmail:  &email,
		Via:           "http",
		ClientMeta:    []byte("{}"),
		ClientTsMs:    1716500000000,
		ServerNowMs:   1716500001000,
		Status:        "new",
	}
}

func TestCreateIssueReport(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_create")
	ctx := context.Background()

	report := sampleIssueReport("evt-001", "TKT-001")
	requestIDs := []string{"req-aaa", "req-bbb"}

	created, isReplay, err := store.CreateIssueReport(ctx, report, requestIDs)
	require.NoError(t, err)
	require.False(t, isReplay)
	require.NotZero(t, created.ID)
	require.Equal(t, "TKT-001", created.TicketID)

	rows, err := store.ListIssueReportRequestIDs(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	ids := []string{rows[0].RequestID, rows[1].RequestID}
	require.ElementsMatch(t, []string{"req-aaa", "req-bbb"}, ids)
}

func TestCreateIssueReportIdempotent(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_idempotent")
	ctx := context.Background()

	report1 := sampleIssueReport("evt-dup", "TKT-DUP1")
	created1, isReplay1, err := store.CreateIssueReport(ctx, report1, []string{"req-1"})
	require.NoError(t, err)
	require.False(t, isReplay1)

	report2 := sampleIssueReport("evt-dup", "TKT-DUP2")
	created2, isReplay2, err := store.CreateIssueReport(ctx, report2, []string{"req-2"})
	require.NoError(t, err)
	require.True(t, isReplay2)
	require.Equal(t, created1.ID, created2.ID)
	require.Equal(t, "TKT-DUP1", created2.TicketID)

	// Replay must still append new request_ids; existing pairs are skipped via
	// the unique (report_id, request_id) index.
	rows, err := store.ListIssueReportRequestIDs(ctx, created1.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	gotReqIDs := []string{rows[0].RequestID, rows[1].RequestID}
	require.ElementsMatch(t, []string{"req-1", "req-2"}, gotReqIDs)

	// A third replay with an already-seen request_id must not duplicate.
	report3 := sampleIssueReport("evt-dup", "TKT-DUP3")
	_, isReplay3, err := store.CreateIssueReport(ctx, report3, []string{"req-1"})
	require.NoError(t, err)
	require.True(t, isReplay3)
	rows, err = store.ListIssueReportRequestIDs(ctx, created1.ID)
	require.NoError(t, err)
	require.Len(t, rows, 2)
}

func TestFindIssueReportByClientEventID(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_find_cev")
	ctx := context.Background()

	found, err := store.FindIssueReportByClientEventID(ctx, "evt-missing")
	require.NoError(t, err)
	require.Nil(t, found)

	report := sampleIssueReport("evt-find", "TKT-FIND")
	_, _, err = store.CreateIssueReport(ctx, report, nil)
	require.NoError(t, err)

	found, err = store.FindIssueReportByClientEventID(ctx, "evt-find")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "TKT-FIND", found.TicketID)
}

func TestFindIssueReportByTicketID(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_find_tid")
	ctx := context.Background()

	found, err := store.FindIssueReportByTicketID(ctx, "TKT-MISS")
	require.NoError(t, err)
	require.Nil(t, found)

	report := sampleIssueReport("evt-tid", "TKT-HIT")
	_, _, err = store.CreateIssueReport(ctx, report, nil)
	require.NoError(t, err)

	found, err = store.FindIssueReportByTicketID(ctx, "TKT-HIT")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, "evt-tid", found.ClientEventID)
}

func TestListIssueReportRequestIDs(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_list_rids")
	ctx := context.Background()

	report := sampleIssueReport("evt-list", "TKT-LIST")
	created, _, err := store.CreateIssueReport(ctx, report, []string{"r1", "r2", "r3"})
	require.NoError(t, err)

	rows, err := store.ListIssueReportRequestIDs(ctx, created.ID)
	require.NoError(t, err)
	require.Len(t, rows, 3)

	rows2, err := store.ListIssueReportRequestIDs(ctx, 99999)
	require.NoError(t, err)
	require.Empty(t, rows2)
}

func TestNullifyIssueReportContactEmail(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_nullify")
	ctx := context.Background()

	report := sampleIssueReport("evt-gdpr", "TKT-GDPR")
	require.NotNil(t, report.ContactEmail)

	created, _, err := store.CreateIssueReport(ctx, report, nil)
	require.NoError(t, err)

	err = store.NullifyIssueReportContactEmail(ctx, "TKT-GDPR")
	require.NoError(t, err)

	found, err := store.FindIssueReportByTicketID(ctx, "TKT-GDPR")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, created.ID, found.ID)
	require.Nil(t, found.ContactEmail)
}

func TestDeleteIssueReportsOlderThan(t *testing.T) {
	store := newIssueReportTestStore(t, "issue_report_delete_old")
	ctx := context.Background()

	oldReport := sampleIssueReport("evt-old", "TKT-OLD")
	oldReport.CreatedAt = time.Now().Add(-100 * 24 * time.Hour)
	_, _, err := store.CreateIssueReport(ctx, oldReport, nil)
	require.NoError(t, err)

	newReport := sampleIssueReport("evt-new", "TKT-NEW")
	newReport.CreatedAt = time.Now()
	_, _, err = store.CreateIssueReport(ctx, newReport, nil)
	require.NoError(t, err)

	cutoff := time.Now().Add(-90 * 24 * time.Hour)
	deleted, err := store.DeleteIssueReportsOlderThan(ctx, cutoff)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)

	found, err := store.FindIssueReportByTicketID(ctx, "TKT-OLD")
	require.NoError(t, err)
	require.Nil(t, found)

	found, err = store.FindIssueReportByTicketID(ctx, "TKT-NEW")
	require.NoError(t, err)
	require.NotNil(t, found)
}
