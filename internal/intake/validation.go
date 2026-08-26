package intake

import (
	"leo-debris-orbit-loop/internal/domain"
	"math"
	"sort"
)

type canonicalPayload struct {
	StationID  string                  `json:"station_id"`
	ArcID      string                  `json:"arc_id"`
	Confidence float64                 `json:"confidence"`
	Samples    []canonicalPayloadPoint `json:"samples"`
}

type canonicalPayloadPoint struct {
	TimeUnixNano int64   `json:"time_unix_nano"`
	RawTime      string  `json:"raw_time"`
	AzimuthDeg   float64 `json:"azimuth_deg"`
	ElevationDeg float64 `json:"elevation_deg"`
}

func normalize(req SubmitArcRequest) ([]domain.AngleSample, string, error) {
	if req.StationID == "" {
		return nil, "", domain.NewError(domain.CodeValidation, "station_id is required")
	}
	if req.ArcID == "" {
		return nil, "", domain.NewError(domain.CodeValidation, "arc_id is required")
	}
	if len(req.Samples) < 3 {
		return nil, "", domain.NewError(domain.CodeValidation, "at least three angle samples are required")
	}
	if !finite(req.Confidence) || req.Confidence < 0 || req.Confidence > 1 {
		return nil, "", domain.NewError(domain.CodeValidation, "confidence must be finite and between zero and one")
	}
	samples := make([]domain.AngleSample, 0, len(req.Samples))
	for i, p := range req.Samples {
		if !finite(p.AzimuthDeg) || p.AzimuthDeg < 0 || p.AzimuthDeg >= 360 {
			return nil, "", domain.Errorf(domain.CodeValidation, "sample %d azimuth out of range", i)
		}
		if !finite(p.ElevationDeg) || p.ElevationDeg < -90 || p.ElevationDeg > 90 {
			return nil, "", domain.Errorf(domain.CodeValidation, "sample %d elevation out of range", i)
		}
		t, err := domain.ParseCCSDSTime(p.Time)
		if err != nil {
			return nil, "", err
		}
		samples = append(samples, domain.AngleSample{RawTime: p.Time, ObservedAt: t, AzimuthDeg: p.AzimuthDeg, ElevationDeg: p.ElevationDeg})
	}
	sort.SliceStable(samples, func(i, j int) bool {
		return samples[i].ObservedAt.Before(samples[j].ObservedAt)
	})
	for i := 1; i < len(samples); i++ {
		if !samples[i].ObservedAt.After(samples[i-1].ObservedAt) {
			return nil, "", domain.NewError(domain.CodeValidation, "sample times must form a strictly increasing arc after sorting")
		}
	}
	points := make([]canonicalPayloadPoint, 0, len(samples))
	for _, s := range samples {
		points = append(points, canonicalPayloadPoint{TimeUnixNano: s.ObservedAt.UnixNano(), RawTime: s.RawTime, AzimuthDeg: round6(s.AzimuthDeg), ElevationDeg: round6(s.ElevationDeg)})
	}
	hash, err := domain.HashAny(canonicalPayload{StationID: req.StationID, ArcID: req.ArcID, Confidence: round6(req.Confidence), Samples: points})
	return samples, hash, err
}

func finite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func round6(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}
