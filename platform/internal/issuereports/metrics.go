package issuereports

import (
	"context"
	"log/slog"
)

// Metrics emits structured slog events for issue-report ingestion observability.
//
// Platform does not currently use prometheus client; per plan §7 the six metric
// names are emitted as `metric` slog events with structured fields so an external
// log-to-metric pipeline can aggregate them. When prometheus is introduced
// platform-wide, this type can be swapped to use promauto counters/gauges with
// the same call sites.
type Metrics struct {
	logger *slog.Logger
}

func NewMetrics(logger *slog.Logger) *Metrics {
	if logger == nil {
		logger = slog.Default()
	}
	return &Metrics{logger: logger.With("subsystem", "issue_reports")}
}

func (m *Metrics) ReceivedTotal(ctx context.Context, runtimeMode, category, severity string) {
	m.logger.LogAttrs(ctx, slog.LevelInfo, "metric",
		slog.String("name", "issue_reports_received_total"),
		slog.String("runtime_mode", runtimeMode),
		slog.String("category", category),
		slog.String("severity", severity),
	)
}

func (m *Metrics) DroppedTotal(ctx context.Context, reason string) {
	m.logger.LogAttrs(ctx, slog.LevelInfo, "metric",
		slog.String("name", "issue_reports_dropped_total"),
		slog.String("reason", reason),
	)
}

// AnonymousRatio is reported as a per-request boolean event; downstream pipeline
// can compute ratio over a rolling window. Cheaper than tracking gauges in-process.
func (m *Metrics) AnonymousObserved(ctx context.Context, anonymous bool) {
	m.logger.LogAttrs(ctx, slog.LevelInfo, "metric",
		slog.String("name", "issue_reports_anonymous_ratio"),
		slog.Bool("anonymous", anonymous),
	)
}

func (m *Metrics) ClockSkewRejected(ctx context.Context, reason string) {
	m.logger.LogAttrs(ctx, slog.LevelWarn, "metric",
		slog.String("name", "clock_skew_rejected_total"),
		slog.String("reason", reason),
	)
}

func (m *Metrics) ContentTruncated(ctx context.Context, field string) {
	m.logger.LogAttrs(ctx, slog.LevelInfo, "metric",
		slog.String("name", "content_truncated_total"),
		slog.String("field", field),
	)
}
