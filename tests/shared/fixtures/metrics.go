package fixtures

import (
	"github.com/siti-nabila/grpc-auth/internal/telemetry"
	"math"
)

func MaxTelemetrySnapshot() telemetry.Snapshot {
	var v telemetry.Snapshot
	for i := range v.RPC {
		for j := range v.RPC[i].Codes {
			v.RPC[i].Codes[j] = math.MaxUint64
		}
		for j := range v.RPC[i].Auth {
			v.RPC[i].Auth[j] = math.MaxUint64
		}
		for j := range v.RPC[i].Histogram {
			v.RPC[i].Histogram[j] = math.MaxUint64
		}
		v.RPC[i].DurationNS = math.MaxUint64
	}
	for i := range v.Audit {
		v.Audit[i] = math.MaxUint64
	}
	for i := range v.Runtime.Pool {
		v.Runtime.Pool[i] = math.MaxUint64
	}
	for i := range v.Runtime.Rate {
		for j := range v.Runtime.Rate[i] {
			v.Runtime.Rate[i][j] = math.MaxUint64
		}
	}
	v.Runtime.Status = telemetry.Stale
	v.Runtime.SampledAt = math.MaxInt64
	v.Runtime.Pending = math.MaxInt64
	v.Runtime.OldestAgeNS = math.MaxUint64
	v.Runtime.LimiterEntries = int(^uint(0) >> 1)
	v.CollectionFailures = math.MaxUint64
	v.ExportFailures = math.MaxUint64
	return v
}
