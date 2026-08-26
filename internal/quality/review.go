package quality

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
)

type DecisionRequest struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func (s *Service) Decide(solutionID string, req DecisionRequest) (domain.ResidualReview, error) {
	var review domain.ResidualReview
	err := s.store.Update(func(st *persistence.State) error {
		current, ok := st.Reviews[solutionID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "review for solution %s not found", solutionID)
		}
		if !domain.CanReviewDecision(current.Status) {
			return (&domain.AppError{Code: domain.CodeIllegalState, Message: "manual decision only applies to pending review", State: string(current.Status)})
		}
		switch req.Decision {
		case "approve":
			current.Status = domain.ReviewApproved
		case "reject":
			current.Status = domain.ReviewRejected
		default:
			return domain.NewError(domain.CodeValidation, "decision must be approve or reject")
		}
		if req.Reason == "" {
			return domain.NewError(domain.CodeValidation, "review reason is required")
		}
		current.Decision = req.Decision
		current.Reason = req.Reason
		current.ReviewedAt = domain.NowUTC()
		hash, err := domain.HashAny(current)
		if err != nil {
			return err
		}
		ev := persistence.AppendEvent(st, persistence.EventResidualReviewed, solutionID, hash, req.Decision)
		current.DecisionEventSeq = ev.Seq
		st.Reviews[solutionID] = current
		review = current
		return nil
	})
	return review, err
}
