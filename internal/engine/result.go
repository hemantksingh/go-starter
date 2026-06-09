package engine

import "time"

// Result captures the outcome of checking a single URL. It is an immutable
// value that flows out of a worker into the aggregator. In Go we return small
// value structs freely — there's no separate "DTO vs entity" ceremony.
type Result struct {
	URL        string
	StatusCode int           // 0 when the request never completed (Err != nil)
	Latency    time.Duration // wall-clock time spent on the attempt
	Err        error         // non-nil on dial/timeout/transport failure
}

// OK reports whether the check counts as healthy: it completed without a
// transport error and returned a non-5xx/4xx status. (2xx and 3xx are healthy.)
func (r Result) OK() bool {
	return r.Err == nil && r.StatusCode >= 200 && r.StatusCode < 400
}
