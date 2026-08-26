package versioning

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"sort"
)

type Service struct {
	store *persistence.Store
}

func New(store *persistence.Store) *Service {
	return &Service{store: store}
}

type FreezeRequest struct {
	SolutionID             string `json:"solution_id"`
	ExpectedCurrentVersion int    `json:"expected_current_version"`
}

func (s *Service) Freeze(targetID string, req FreezeRequest) (domain.FrozenVersion, error) {
	var version domain.FrozenVersion
	err := s.store.Update(func(st *persistence.State) error {
		target, ok := st.Targets[targetID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "target %s not found", targetID)
		}
		if target.CurrentFrozenVersion != req.ExpectedCurrentVersion {
			return domain.Errorf(domain.CodeConflict, "expected current version %d but current is %d", req.ExpectedCurrentVersion, target.CurrentFrozenVersion)
		}
		solution, ok := st.Solutions[req.SolutionID]
		if !ok || solution.TargetID != targetID {
			return domain.Errorf(domain.CodeNotFound, "solution %s not found for target", req.SolutionID)
		}
		review, ok := st.Reviews[req.SolutionID]
		if !ok {
			return domain.NewError(domain.CodePreconditionFail, "solution has no residual review")
		}
		if !domain.CanFreezeReview(review.Status) {
			return (&domain.AppError{Code: domain.CodeIllegalState, Message: "only approved solutions can freeze", State: string(review.Status)})
		}
		for _, existing := range st.FrozenVersions {
			if existing.SolutionID == req.SolutionID {
				return domain.NewError(domain.CodeFrozenImmutable, "solution is already frozen")
			}
		}
		nextVersion := target.CurrentFrozenVersion + 1
		version = domain.FrozenVersion{TargetID: targetID, Version: nextVersion, SolutionID: solution.ID, PublishedAt: domain.NowUTC(), Orbit: solution.Parameters, InputArcIDs: append([]string(nil), solution.ObservationArcIDs...), ResidualSummary: review.Summary, ResultHash: solution.OutputHash}
		hash, err := domain.HashAny(version)
		if err != nil {
			return err
		}
		version.ContentHash = hash
		ev := persistence.AppendEvent(st, persistence.EventVersionFrozen, targetID, hash, solution.ID)
		version.FreezeEventSeq = ev.Seq
		target.CurrentFrozenVersion = nextVersion
		target.LifecycleState = "published"
		target.UpdatedAt = ev.RecordedAt
		review.Status = domain.ReviewFrozen
		st.Targets[targetID] = target
		st.Reviews[req.SolutionID] = review
		st.FrozenVersions[persistence.VersionKey(targetID, nextVersion)] = version
		return nil
	})
	return version, err
}

func (s *Service) List(targetID string) ([]domain.FrozenVersion, error) {
	var versions []domain.FrozenVersion
	err := s.store.View(func(st *persistence.State) error {
		if _, ok := st.Targets[targetID]; !ok {
			return domain.Errorf(domain.CodeNotFound, "target %s not found", targetID)
		}
		for _, version := range st.FrozenVersions {
			if version.TargetID == targetID {
				versions = append(versions, version)
			}
		}
		sort.Slice(versions, func(i, j int) bool { return versions[i].Version < versions[j].Version })
		return nil
	})
	return versions, err
}

func (s *Service) Get(targetID string, number int) (domain.FrozenVersion, error) {
	var version domain.FrozenVersion
	err := s.store.View(func(st *persistence.State) error {
		found, ok := st.FrozenVersions[persistence.VersionKey(targetID, number)]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "version %d not found for target %s", number, targetID)
		}
		version = found
		return nil
	})
	return version, err
}
