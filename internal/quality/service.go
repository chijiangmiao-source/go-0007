package quality

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"math"
)

type Service struct {
	store      *persistence.Store
	thresholds domain.ThresholdSnapshot
}

func New(store *persistence.Store) *Service {
	return &Service{store: store, thresholds: domain.ThresholdSnapshot{PublishRMSDeg: 0.08, RejectRMSDeg: 0.25, ConfigRevision: 1}}
}

func (s *Service) RecordEngineResiduals(solutionID string, residuals []domain.ResidualPoint) (domain.ResidualReview, error) {
	var review domain.ResidualReview
	err := s.store.Update(func(st *persistence.State) error {
		if _, ok := st.Solutions[solutionID]; !ok {
			return domain.Errorf(domain.CodeNotFound, "solution %s not found", solutionID)
		}
		if existing, ok := st.Reviews[solutionID]; ok {
			review = existing
			return nil
		}
		summary := summarize(residuals)
		status := domain.ReviewPending
		if summary.RMSDeg <= s.thresholds.PublishRMSDeg {
			status = domain.ReviewApproved
		} else if summary.RMSDeg > s.thresholds.RejectRMSDeg {
			status = domain.ReviewRejected
		}
		review = domain.ResidualReview{SolutionID: solutionID, Residuals: residuals, Summary: summary, Thresholds: s.thresholds, Status: status, CreatedAt: domain.NowUTC()}
		hash, err := domain.HashAny(review)
		if err != nil {
			return err
		}
		ev := persistence.AppendEvent(st, persistence.EventResidualReviewed, solutionID, hash, "auto-quality")
		if status != domain.ReviewPending {
			review.DecisionEventSeq = ev.Seq
		}
		st.Reviews[solutionID] = review
		return nil
	})
	return review, err
}

func (s *Service) EvaluateSolution(solutionID string) (domain.ResidualReview, error) {
	var residuals []domain.ResidualPoint
	err := s.store.View(func(st *persistence.State) error {
		solution, ok := st.Solutions[solutionID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "solution %s not found", solutionID)
		}
		residuals = append([]domain.ResidualPoint(nil), solution.Residuals...)
		return nil
	})
	if err != nil {
		return domain.ResidualReview{}, err
	}
	return s.RecordEngineResiduals(solutionID, residuals)
}

func (s *Service) Get(solutionID string) (domain.ResidualReview, error) {
	var review domain.ResidualReview
	err := s.store.View(func(st *persistence.State) error {
		found, ok := st.Reviews[solutionID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "review for solution %s not found", solutionID)
		}
		review = found
		return nil
	})
	return review, err
}

func summarize(points []domain.ResidualPoint) domain.ResidualSummary {
	var sumSq, max float64
	for _, p := range points {
		sumSq += p.MagnitudeDeg * p.MagnitudeDeg
		if p.MagnitudeDeg > max {
			max = p.MagnitudeDeg
		}
	}
	if len(points) == 0 {
		return domain.ResidualSummary{}
	}
	return domain.ResidualSummary{RMSDeg: math.Round(math.Sqrt(sumSq/float64(len(points)))*1_000_000) / 1_000_000, MaxDeg: max, SampleCount: len(points)}
}
