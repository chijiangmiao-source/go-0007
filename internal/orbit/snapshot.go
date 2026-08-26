package orbit

import (
	"leo-debris-orbit-loop/internal/domain"
	"sort"
	"time"
)

type InputSnapshot struct {
	TargetID            string        `json:"target_id"`
	AssociationRevision int64         `json:"association_revision"`
	CalculatorVersion   string        `json:"calculator_version"`
	ThresholdRevision   int64         `json:"threshold_revision"`
	CreatedAt           time.Time     `json:"created_at"`
	Arcs                []SnapshotArc `json:"arcs"`
}

type SnapshotArc struct {
	ArcID       string               `json:"arc_id"`
	StationID   string               `json:"station_id"`
	Confidence  float64              `json:"confidence"`
	PayloadHash string               `json:"payload_hash"`
	Samples     []domain.AngleSample `json:"samples"`
}

func BuildSnapshot(target domain.CatalogTarget, arcs []domain.ObservationArc, calculatorVersion string) (InputSnapshot, string, error) {
	sort.Slice(arcs, func(i, j int) bool {
		if arcs[i].ReceivedSeq == arcs[j].ReceivedSeq {
			return arcs[i].ID < arcs[j].ID
		}
		return arcs[i].ReceivedSeq < arcs[j].ReceivedSeq
	})
	snap := InputSnapshot{TargetID: target.ID, AssociationRevision: target.AssociationRevision, CalculatorVersion: calculatorVersion, ThresholdRevision: 1, CreatedAt: target.UpdatedAt}
	for _, arc := range arcs {
		snap.Arcs = append(snap.Arcs, SnapshotArc{ArcID: arc.ID, StationID: arc.StationID, Confidence: arc.Confidence, PayloadHash: arc.PayloadHash, Samples: append([]domain.AngleSample(nil), arc.CanonicalSamples...)})
	}
	hash, err := domain.HashAny(snap)
	return snap, hash, err
}
