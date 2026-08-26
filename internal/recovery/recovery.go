package recovery

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
)

type Service struct {
	store *persistence.Store
}

func New(store *persistence.Store) *Service {
	return &Service{store: store}
}

func (s *Service) Recover() error {
	return s.store.Update(func(st *persistence.State) error {
		if err := persistence.ValidateEventChain(st.Events); err != nil {
			return err
		}
		for id, job := range st.SolveJobs {
			if job.Status == domain.JobRunning || job.Status == domain.JobQueued {
				job.Status = domain.JobCanceled
				job.CancelReason = "restart recovery canceled unfinished task"
				ev := persistence.AppendEvent(st, persistence.EventSolveCanceled, id, job.InputSnapshotHash, "recovery")
				job.CancelEventSeq = ev.Seq
				job.UpdatedAt = ev.RecordedAt
				st.SolveJobs[id] = job
			}
		}
		return nil
	})
}
