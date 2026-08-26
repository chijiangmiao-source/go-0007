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
		// 重启恢复前必须先校验事件日志的校验链完整性；一旦发现序列缺口或
		// 校验和不符，立即停止恢复并报告损坏边界，绝不在坏链上追加取消
		// 记录或改写状态。Update 在回调返回错误时不会落盘，从而保证损坏
		// 的存储不被继续重写。
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
