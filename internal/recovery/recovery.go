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
		// The write cursor must point exactly past the validated log tail.
		// A non-zero cursor that lags behind (or overshoots) the events
		// indicates an abnormal exit left a detectable inconsistency: the
		// chain itself is still valid, but the next AppendEvent would reuse
		// an existing sequence number and corrupt the log. Reject the state
		// here rather than enlarging the anomaly into persistent damage.
		if expected := int64(len(st.Events)) + 1; st.NextEvent != expected {
			return domain.Errorf(domain.CodeCorruptStore, "event cursor %d does not match log tail %d", st.NextEvent, expected)
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
