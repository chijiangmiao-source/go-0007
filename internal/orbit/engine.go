package orbit

import (
	"context"
	"leo-debris-orbit-loop/internal/domain"
	"math"
	"time"
)

type Engine interface {
	Version() string
	Compute(ctx context.Context, snapshot InputSnapshot) (EngineResult, error)
}

type EngineResult struct {
	Epoch      time.Time
	Params     domain.OrbitParameters
	Iteration  domain.IterationSummary
	Residuals  []domain.ResidualPoint
	OutputHash string
}

type DeterministicEngine struct {
	version string
	mode    string
}

func NewDeterministicEngine(mode string) *DeterministicEngine {
	if mode == "" {
		mode = "normal"
	}
	return &DeterministicEngine{version: "deterministic-orbit-v1", mode: mode}
}

func (e *DeterministicEngine) Version() string {
	return e.version + ":" + e.mode
}

func (e *DeterministicEngine) Compute(ctx context.Context, snapshot InputSnapshot) (EngineResult, error) {
	switch e.mode {
	case "timeout":
		timer := time.NewTimer(2 * time.Second)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return EngineResult{}, ctx.Err()
		case <-timer.C:
		}
	case "retry":
		return EngineResult{}, domain.NewError(domain.CodeUnavailable, "deterministic retry fault")
	case "nonconverge":
		return EngineResult{}, domain.NewError(domain.CodeNonConverged, "deterministic non convergence")
	}
	select {
	case <-ctx.Done():
		return EngineResult{}, ctx.Err()
	default:
	}
	if len(snapshot.Arcs) == 0 {
		return EngineResult{}, domain.NewError(domain.CodeNonConverged, "no associated arcs in snapshot")
	}
	var az, el, conf float64
	var count int
	var epoch time.Time
	for _, arc := range snapshot.Arcs {
		conf += arc.Confidence
		for _, sample := range arc.Samples {
			az += sample.AzimuthDeg
			el += sample.ElevationDeg
			count++
			if epoch.IsZero() || sample.ObservedAt.Before(epoch) {
				epoch = sample.ObservedAt
			}
		}
	}
	meanAz := az / float64(count)
	meanEl := el / float64(count)
	meanConf := conf / float64(len(snapshot.Arcs))
	params := domain.OrbitParameters{
		SemiMajorAxisKM: round3(6800 + meanEl*8 + meanConf*30),
		Eccentricity:    round6(math.Abs(math.Sin(meanAz*math.Pi/180)) * 0.015),
		InclinationDeg:  round3(35 + math.Mod(meanAz, 55)),
		RAANDeg:         round3(math.Mod(meanAz*1.7+float64(snapshot.AssociationRevision), 360)),
		ArgPerigeeDeg:   round3(math.Mod(meanEl*2+180, 360)),
		MeanAnomalyDeg:  round3(math.Mod(float64(count)*17+meanAz, 360)),
	}
	residuals := make([]domain.ResidualPoint, 0, count)
	for _, arc := range snapshot.Arcs {
		for i, sample := range arc.Samples {
			azErr := math.Sin((sample.AzimuthDeg+float64(i))*math.Pi/180) * residualScale(e.mode)
			elErr := math.Cos((sample.ElevationDeg+float64(i))*math.Pi/180) * residualScale(e.mode) * 0.8
			residuals = append(residuals, domain.ResidualPoint{ArcID: arc.ArcID, SampleTime: sample.ObservedAt, AzimuthError: round6(azErr), ElevationError: round6(elErr), MagnitudeDeg: round6(math.Hypot(azErr, elErr))})
		}
	}
	out := EngineResult{Epoch: epoch, Params: params, Iteration: domain.IterationSummary{Iterations: 6 + len(snapshot.Arcs), LastDelta: 0.00042, Converged: true}, Residuals: residuals}
	hash, err := domain.HashAny(struct {
		SnapshotHash string                 `json:"snapshot_hash"`
		Params       domain.OrbitParameters `json:"params"`
		Residuals    []domain.ResidualPoint `json:"residuals"`
	}{SnapshotHash: domain.MustHashAny(snapshot), Params: params, Residuals: residuals})
	if err != nil {
		return EngineResult{}, err
	}
	out.OutputHash = hash
	return out, nil
}

func residualScale(mode string) float64 {
	switch mode {
	case "review":
		return 0.16
	case "reject":
		return 0.55
	default:
		return 0.025
	}
}

func round3(v float64) float64 {
	return math.Round(v*1000) / 1000
}

func round6(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}
