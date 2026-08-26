package association

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"math"
	"time"
)

func midpoint(arc domain.ObservationArc) time.Time {
	if len(arc.CanonicalSamples) == 0 {
		return time.Time{}
	}
	first := arc.CanonicalSamples[0].ObservedAt
	last := arc.CanonicalSamples[len(arc.CanonicalSamples)-1].ObservedAt
	return first.Add(last.Sub(first) / 2)
}

func angularResidual(a, b domain.ObservationArc) float64 {
	if len(a.CanonicalSamples) == 0 || len(b.CanonicalSamples) == 0 {
		return 999
	}
	aa := meanAngles(a)
	bb := meanAngles(b)
	az := math.Abs(aa[0] - bb[0])
	if az > 180 {
		az = 360 - az
	}
	el := math.Abs(aa[1] - bb[1])
	return math.Round(math.Hypot(az, el)*1_000_000) / 1_000_000
}

func meanAngles(a domain.ObservationArc) [2]float64 {
	var az, el float64
	for _, s := range a.CanonicalSamples {
		az += s.AzimuthDeg
		el += s.ElevationDeg
	}
	n := float64(len(a.CanonicalSamples))
	return [2]float64{az / n, el / n}
}

func stationGeometryPenalty(st *persistence.State, a, b string) float64 {
	if a == b {
		return 0
	}
	sa, oka := st.Stations[a]
	sb, okb := st.Stations[b]
	if !oka || !okb {
		return 5
	}
	dlat := sa.LatitudeDeg - sb.LatitudeDeg
	dlon := sa.LongitudeDeg - sb.LongitudeDeg
	return math.Round(math.Hypot(dlat, dlon)/50*1_000_000) / 1_000_000
}
