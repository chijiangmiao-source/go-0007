package persistence

import (
	"leo-debris-orbit-loop/internal/domain"
	"sort"
)

func ArcsForTarget(st *State, targetID string) []domain.ObservationArc {
	arcs := make([]domain.ObservationArc, 0)
	for _, arc := range st.ObservationArcs {
		if arc.AssociatedTargetID == targetID {
			arcs = append(arcs, arc)
		}
	}
	sort.Slice(arcs, func(i, j int) bool {
		a := arcs[i].CanonicalSamples[0].ObservedAt
		b := arcs[j].CanonicalSamples[0].ObservedAt
		if a.Equal(b) {
			return arcs[i].ID < arcs[j].ID
		}
		return a.Before(b)
	})
	return arcs
}

func LatestJobForTarget(st *State, targetID string) *domain.SolveJob {
	var out *domain.SolveJob
	for _, job := range st.SolveJobs {
		if job.TargetID != targetID {
			continue
		}
		cp := job
		if out == nil || cp.CreatedAt.After(out.CreatedAt) || cp.ID > out.ID {
			out = &cp
		}
	}
	return out
}

func LatestReviewForTarget(st *State, targetID string) *domain.ResidualReview {
	var out *domain.ResidualReview
	for _, sol := range st.Solutions {
		if sol.TargetID != targetID {
			continue
		}
		review, ok := st.Reviews[sol.ID]
		if !ok {
			continue
		}
		cp := review
		if out == nil || cp.CreatedAt.After(out.CreatedAt) {
			out = &cp
		}
	}
	return out
}
