package metrics

import (
	"reflect"
	"testing"
)

func testTextParse(t testing.TB) {
	var scenarios = []struct {
		in  string
		out *Proxy
	}{
		// 0: Empty
		{
			in: `
`,
			out: &Proxy{false, 0, 0},
		},
		// 1: No Metrics
		{
			in: `
`,
			out: &Proxy{false, 0, 0},
		},
		// 2: Valid
		{
			in: `
# HELP http_connections Number of HTTP connections
# TYPE http_connections gauge
http_connections{state="reading"} 0
http_connections{state="waiting"} 0
http_connections{state="writing"} 10
# HELP http_requests_duration_seconds HTTP request latency
# TYPE http_requests_duration_seconds histogram
http_requests_duration_seconds_bucket{host="_",le="00.005"} 397
http_requests_duration_seconds_bucket{host="_",le="00.010"} 397
http_requests_duration_seconds_bucket{host="_",le="00.020"} 397
http_requests_duration_seconds_bucket{host="_",le="00.030"} 397
http_requests_duration_seconds_bucket{host="_",le="00.050"} 397
http_requests_duration_seconds_bucket{host="_",le="00.075"} 397
http_requests_duration_seconds_bucket{host="_",le="00.100"} 397
http_requests_duration_seconds_bucket{host="_",le="00.200"} 397
http_requests_duration_seconds_bucket{host="_",le="00.300"} 397
http_requests_duration_seconds_bucket{host="_",le="00.400"} 397
http_requests_duration_seconds_bucket{host="_",le="00.500"} 397
http_requests_duration_seconds_bucket{host="_",le="00.750"} 397
http_requests_duration_seconds_bucket{host="_",le="01.000"} 397
http_requests_duration_seconds_bucket{host="_",le="01.500"} 397
http_requests_duration_seconds_bucket{host="_",le="02.000"} 397
http_requests_duration_seconds_bucket{host="_",le="03.000"} 397
http_requests_duration_seconds_bucket{host="_",le="04.000"} 397
http_requests_duration_seconds_bucket{host="_",le="05.000"} 397
http_requests_duration_seconds_bucket{host="_",le="10.000"} 397
http_requests_duration_seconds_bucket{host="_",le="+Inf"} 431
http_requests_duration_seconds_count{host="_"} 431
http_requests_duration_seconds_sum{host="_"} 855.36
# HELP http_requests_seconds_ago Number of seconds since the last connection
# TYPE http_requests_seconds_ago gauge
http_requests_seconds_ago 11
# HELP http_requests_total Number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{host="_",status="200"} 428
http_requests_total{host="_",status="499"} 3
# HELP http_requests_waiting_endpoint Info metric indicating if the proxy is waiting for pods
# TYPE http_requests_waiting_endpoint gauge
http_requests_waiting_endpoint 0
# HELP nginx_metric_errors_total Number of nginx-lua-prometheus errors
# TYPE nginx_metric_errors_total counter
nginx_metric_errors_total 0
`,
			out: &Proxy{false, 11, 10},
		},
		{
			in: `
# HELP http_connections Number of HTTP connections
# TYPE http_connections gauge
http_connections{state="reading"} 0
http_connections{state="waiting"} 0
http_connections{state="writing"} 1
# HELP http_requests_seconds_ago Number of seconds since the last connection
# TYPE http_requests_seconds_ago gauge
http_requests_seconds_ago 133
# HELP http_requests_total Number of HTTP requests
# TYPE http_requests_total counter
http_requests_total{host="_",status="200"} 4
http_requests_total{host="_",status="499"} 3
# HELP http_requests_waiting_endpoint Info metric indicating if the proxy is waiting for pods
# TYPE http_requests_waiting_endpoint gauge
http_requests_waiting_endpoint 1
# HELP nginx_metric_errors_total Number of nginx-lua-prometheus errors
# TYPE nginx_metric_errors_total counter
nginx_metric_errors_total 0
`,
			out: &Proxy{true, 133, 1},
		},
	}

	for i, scenario := range scenarios {
		out, err := parse([]byte(scenario.in))
		if err != nil {
			t.Errorf("%d. error: %s", i, err)
			continue
		}

		if !reflect.DeepEqual(out, scenario.out) {
			t.Errorf("%v is not equal to expected value %v", out, scenario.out)
			continue
		}
	}
}

func TestTextParse(t *testing.T) {
	testTextParse(t)
}

func BenchmarkTextParse(b *testing.B) {
	for i := 0; i < b.N; i++ {
		testTextParse(b)
	}
}
