package metrics

import (
	"bytes"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	corev1 "k8s.io/api/core/v1"
)

// Proxy holds metrics
type Proxy struct {
	// WaitingForPods indicates if the proxy is holding requests waiting for pods to be avialable
	WaitingForPods bool `json:"waitingForPods"`
	// LastRequest seconds since the last request
	LastRequest int `json:"lastRequest"`
	// PendingRequests number of requests pending to be processed by the proxy
	PendingRequests int `json:"pendingRequest"`
}

const (
	metricPort = 19999

	httpConnections              = "http_connections"
	httpRequestsSecondsAgo       = "http_requests_seconds_ago"
	httpRequestsWaitingEndpoints = "http_requests_waiting_endpoint"
)

// Pull extracts proxy metrics from a running proxy pod
func Pull(pod *corev1.Pod) (*Proxy, error) {
	// extract IP
	// make request
	// parse metrics
	// return
	return nil, nil
}

func parse(data []byte) (*Proxy, error) {
	textParser := expfmt.TextParser{}

	r := bytes.NewReader(data)

	dtos, err := textParser.TextToMetricFamilies(r)
	if err != nil {
		return nil, err
	}

	out := &Proxy{}

	if metric, ok := dtos[httpConnections]; ok {
		out.PendingRequests = findMetricValueWithLabel(metric, "state", "writing")
	}

	if metric, ok := dtos[httpRequestsSecondsAgo]; ok {
		out.LastRequest = extractValue(metric)
	}

	if metric, ok := dtos[httpRequestsWaitingEndpoints]; ok {
		mv := extractValue(metric)
		if mv == 1 {
			out.WaitingForPods = true
		}
	}

	return out, nil
}

func extractValue(metricFamily *dto.MetricFamily) int {
	m := *metricFamily.Metric[0]
	if m.Gauge != nil {
		return int(m.Gauge.GetValue())
	}
	if m.Counter != nil {
		return int(m.Counter.GetValue())
	}
	if m.Untyped != nil {
		return int(m.Untyped.GetValue())
	}

	return 0
}

func findMetricValueWithLabel(mf *dto.MetricFamily, label, value string) int {
	for _, m := range mf.Metric {
		for _, l := range m.Label {
			if label == l.GetName() && value == l.GetValue() {
				if m.Gauge != nil {
					return int(m.Gauge.GetValue())
				}
				if m.Counter != nil {
					return int(m.Counter.GetValue())
				}
				if m.Untyped != nil {
					return int(m.Untyped.GetValue())
				}

				return 0
			}
		}
	}

	return 0
}
