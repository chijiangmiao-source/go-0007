package association

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"math"
	"sort"
)

const maxAssociationScore = 18.0

func scoreCandidates(st *persistence.State, arc domain.ObservationArc) map[string]domain.AssociationScore {
	out := map[string]domain.AssociationScore{}
	for _, target := range st.Targets {
		arcs := persistence.ArcsForTarget(st, target.ID)
		if len(arcs) == 0 {
			continue
		}
		ref := arcs[len(arcs)-1]
		score := scorePair(st, arc, ref)
		if score.Total <= maxAssociationScore {
			out[target.ID] = score
		}
	}
	return out
}

func scorePair(st *persistence.State, arc, ref domain.ObservationArc) domain.AssociationScore {
	aMid := midpoint(arc)
	rMid := midpoint(ref)
	timeDelta := math.Abs(aMid.Sub(rMid).Seconds())
	angleResidual := angularResidual(arc, ref)
	geometryPenalty := stationGeometryPenalty(st, arc.StationID, ref.StationID)
	confidenceBias := (1 - arc.Confidence) * 2
	total := math.Round((timeDelta/90+angleResidual+geometryPenalty+confidenceBias)*1_000_000) / 1_000_000
	return domain.AssociationScore{TimeDeltaSeconds: timeDelta, AngleResidualDeg: angleResidual, GeometryPenalty: geometryPenalty, ConfidenceBias: confidenceBias, Total: total}
}

func chooseTarget(st *persistence.State, scores map[string]domain.AssociationScore) (string, string) {
	if len(scores) == 0 {
		return "", "new-target"
	}
	ids := make([]string, 0, len(scores))
	for id := range scores {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := scores[ids[i]], scores[ids[j]]
		if a.Total != b.Total {
			return a.Total < b.Total
		}
		if a.AngleResidualDeg != b.AngleResidualDeg {
			return a.AngleResidualDeg < b.AngleResidualDeg
		}
		ta := st.Targets[ids[i]]
		tb := st.Targets[ids[j]]
		if ta.CreatedEventSeq != tb.CreatedEventSeq {
			return ta.CreatedEventSeq < tb.CreatedEventSeq
		}
		return ids[i] < ids[j]
	})
	return ids[0], "score-time-angle-created-target"
}
