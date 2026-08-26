package association

import (
	"fmt"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"sort"
	"time"
)

type Service struct {
	store *persistence.Store
}

func New(store *persistence.Store) *Service {
	return &Service{store: store}
}

func (s *Service) ProcessPending() error {
	return s.store.Update(func(st *persistence.State) error {
		pending := make([]domain.ObservationArc, 0)
		for _, arc := range st.ObservationArcs {
			if arc.Quarantined || arc.AssociatedTargetID != "" {
				continue
			}
			pending = append(pending, arc)
		}
		sort.Slice(pending, func(i, j int) bool {
			if pending[i].ReceivedSeq == pending[j].ReceivedSeq {
				return pending[i].ID < pending[j].ID
			}
			return pending[i].ReceivedSeq < pending[j].ReceivedSeq
		})
		for _, arc := range pending {
			scores := scoreCandidates(st, arc)
			targetID, tie := chooseTarget(st, scores)
			var target domain.CatalogTarget
			if targetID == "" {
				targetID = persistence.NextTargetID(st)
				target = domain.CatalogTarget{ID: targetID, LifecycleState: "candidate", CreatedAt: domain.NowUTC(), UpdatedAt: domain.NowUTC()}
				ev := persistence.AppendEvent(st, persistence.EventTargetAssociated, targetID, arc.PayloadHash, arc.ID)
				target.CreatedEventSeq = ev.Seq
				target.AssociationRevision = 1
				st.Targets[targetID] = target
				arc.AssociationEvent = ev.Seq
			} else {
				target = st.Targets[targetID]
				target.AssociationRevision++
				target.UpdatedAt = domain.NowUTC()
				st.Targets[targetID] = target
				ev := persistence.AppendEvent(st, persistence.EventTargetAssociated, targetID, arc.PayloadHash, arc.ID)
				arc.AssociationEvent = ev.Seq
			}
			arc.AssociatedTargetID = targetID
			st.ObservationArcs[arc.ID] = arc
			decision := domain.AssociationDecision{
				ID: fmt.Sprintf("ASC-%s", arc.ID), ArcKey: arc.ID, CandidateScores: scores,
				TieBreaker: tie, FinalTargetID: targetID, ConfigRevision: 1,
				DecisionEventSeq: arc.AssociationEvent, AssociationRevision: st.Targets[targetID].AssociationRevision,
				DecidedAt: time.Now().UTC(),
			}
			st.Associations[decision.ID] = decision
		}
		return nil
	})
}

func (s *Service) Target(id string) (domain.CatalogTarget, []domain.ObservationArc, *domain.SolveJob, *domain.ResidualReview, error) {
	var target domain.CatalogTarget
	var arcs []domain.ObservationArc
	var latestJob *domain.SolveJob
	var review *domain.ResidualReview
	err := s.store.View(func(st *persistence.State) error {
		found, ok := st.Targets[id]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "target %s not found", id)
		}
		target = found
		arcs = persistence.ArcsForTarget(st, id)
		latestJob = persistence.LatestJobForTarget(st, id)
		review = persistence.LatestReviewForTarget(st, id)
		return nil
	})
	return target, arcs, latestJob, review, err
}
