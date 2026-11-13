package httpapi

import (
	"time"

	"github.com/hemantksingh/go-starter/internal/engine"
	"github.com/hemantksingh/go-starter/internal/healthrun"
)

// Response shapes live in the transport layer. The domain types stay free of
// JSON tags, and this is where a time.Duration (int64 nanoseconds on the wire,
// useless to a client) becomes milliseconds.

type runResponse struct {
	ID         string           `json:"id"`
	Tenant     string           `json:"tenant"`
	Status     healthrun.Status `json:"status"`
	URLCount   int              `json:"url_count"`
	CreatedAt  time.Time        `json:"created_at"`
	StartedAt  *time.Time       `json:"started_at,omitempty"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`
	Summary    *summaryResponse `json:"summary,omitempty"`
	Error      string           `json:"error,omitempty"`
}

type summaryResponse struct {
	Total       int64           `json:"total"`
	Success     int64           `json:"success"`
	Failed      int64           `json:"failed"`
	StatusCodes map[int]int     `json:"status_codes"` // encoding/json renders int keys as strings
	Errors      map[string]int  `json:"errors"`
	LatencyMS   latencyResponse `json:"latency_ms"`
}

type latencyResponse struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Mean float64 `json:"mean"`
	P50  float64 `json:"p50"`
	P95  float64 `json:"p95"`
	P99  float64 `json:"p99"`
}

func toRunResponse(r healthrun.Run) runResponse {
	out := runResponse{
		ID:         r.ID,
		Tenant:     r.TenantID,
		Status:     r.Status,
		URLCount:   r.URLCount,
		CreatedAt:  r.CreatedAt,
		StartedAt:  r.StartedAt,
		FinishedAt: r.FinishedAt,
		Error:      r.Error,
	}
	if r.Summary != nil {
		out.Summary = toSummaryResponse(*r.Summary)
	}
	return out
}

func toSummaryResponse(s engine.Summary) *summaryResponse {
	return &summaryResponse{
		Total:       s.Total,
		Success:     s.Success,
		Failed:      s.Failed,
		StatusCodes: s.StatusHist,
		Errors:      s.ErrHist,
		LatencyMS: latencyResponse{
			Min:  ms(s.Min),
			Max:  ms(s.Max),
			Mean: ms(s.Mean),
			P50:  ms(s.P50),
			P95:  ms(s.P95),
			P99:  ms(s.P99),
		},
	}
}

func ms(d time.Duration) float64 { return float64(d) / float64(time.Millisecond) }
