package relay

import (
	"github.com/blocknative/dreamboat/metrics"
	"github.com/prometheus/client_golang/prometheus"
)

type RelayMetrics struct {
	Timing *prometheus.HistogramVec

	MissHeaderCount *prometheus.CounterVec
}

func (r *Relay) initMetrics() {
	r.m.Timing = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "dreamboat",
		Subsystem: "relay",
		Name:      "timing",
		Help:      "Duration of requests per function",
	}, []string{"function", "type"})

	r.m.MissHeaderCount = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "dreamboat",
		Subsystem: "relayprocess",
		Name:      "missHeader",
		Help:      "Number of missed headers by reason (oldSlot, noSubmission)",
	}, []string{"reason"})
}

func (r *Relay) AttachMetrics(m *metrics.Metrics) {
	m.Register(r.m.Timing)
	m.Register(r.m.MissHeaderCount)
}
