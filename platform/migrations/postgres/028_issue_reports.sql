-- 028_issue_reports.sql
--
-- OfficeDex desktop "user-initiated issue report" ingestion endpoint.
-- See .omc/plans/2026-05-24-officedex-issue-reporting-receiver.md
--
-- Tables:
--   issue_reports                desktop-submitted problem reports
--   issue_report_request_ids     bridge to usage_events.request_id (hosted-mode evidence chain)
--
-- Forward-only and idempotent: all DDL uses IF NOT EXISTS guards.

CREATE TABLE IF NOT EXISTS issue_reports (
  id BIGSERIAL PRIMARY KEY,
  ticket_id VARCHAR(40) NOT NULL UNIQUE,
  user_id BIGINT NULL,
  client_event_id VARCHAR(128) NOT NULL UNIQUE,
  schema_version INTEGER NOT NULL,
  runtime_mode VARCHAR(16) NOT NULL,
  category VARCHAR(64) NOT NULL DEFAULT '',
  severity VARCHAR(16) NOT NULL DEFAULT '',
  summary VARCHAR(500) NOT NULL,
  detail VARCHAR(2000) NOT NULL DEFAULT '',
  contact_email VARCHAR(254) NULL,
  task_id VARCHAR(128) NOT NULL DEFAULT '',
  error_code VARCHAR(64) NOT NULL DEFAULT '',
  error_message VARCHAR(500) NOT NULL DEFAULT '',
  via VARCHAR(16) NOT NULL DEFAULT 'http',
  client_meta JSONB NOT NULL DEFAULT '{}'::jsonb,
  client_ts_ms BIGINT NOT NULL,
  server_now_ms BIGINT NOT NULL,
  clock_skew_fallback BOOLEAN NOT NULL DEFAULT FALSE,
  client_ip VARCHAR(64) NOT NULL DEFAULT '',
  user_agent VARCHAR(512) NOT NULL DEFAULT '',
  status VARCHAR(16) NOT NULL DEFAULT 'new' CHECK (status IN ('new', 'triaged', 'resolved', 'dropped')),
  triaged_at TIMESTAMPTZ NULL,
  triaged_by VARCHAR(191) NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_issue_reports_user_id_created_at ON issue_reports(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_issue_reports_status_created_at ON issue_reports(status, created_at);
CREATE INDEX IF NOT EXISTS idx_issue_reports_runtime_mode ON issue_reports(runtime_mode);
CREATE INDEX IF NOT EXISTS idx_issue_reports_created_at ON issue_reports(created_at);

CREATE TABLE IF NOT EXISTS issue_report_request_ids (
  id BIGSERIAL PRIMARY KEY,
  report_id BIGINT NOT NULL REFERENCES issue_reports(id) ON DELETE CASCADE,
  request_id VARCHAR(128) NOT NULL,
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_issue_report_request_ids_report_id ON issue_report_request_ids(report_id);
CREATE INDEX IF NOT EXISTS idx_issue_report_request_ids_request_id ON issue_report_request_ids(request_id);
CREATE UNIQUE INDEX IF NOT EXISTS uq_issue_report_request_ids_report_request ON issue_report_request_ids(report_id, request_id);
