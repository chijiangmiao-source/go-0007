package persistence

import (
	"leo-debris-orbit-loop/internal/domain"
	"time"
)

type State struct {
	Stations        map[string]domain.Station             `json:"stations"`
	ObservationArcs map[string]domain.ObservationArc      `json:"observation_arcs"`
	ArcIndex        map[string]string                     `json:"arc_index"`
	Targets         map[string]domain.CatalogTarget       `json:"targets"`
	Associations    map[string]domain.AssociationDecision `json:"associations"`
	SolveJobs       map[string]domain.SolveJob            `json:"solve_jobs"`
	Solutions       map[string]domain.OrbitSolution       `json:"solutions"`
	Reviews         map[string]domain.ResidualReview      `json:"reviews"`
	FrozenVersions  map[string]domain.FrozenVersion       `json:"frozen_versions"`
	Events          []domain.EventRecord                  `json:"events"`
	Checkpoint      domain.Checkpoint                     `json:"checkpoint"`
	NextTarget      int64                                 `json:"next_target"`
	NextJob         int64                                 `json:"next_job"`
	NextSolution    int64                                 `json:"next_solution"`
	NextEvent       int64                                 `json:"next_event"`
}

func NewState() *State {
	now := domain.NowUTC()
	s := &State{
		Stations:        map[string]domain.Station{},
		ObservationArcs: map[string]domain.ObservationArc{},
		ArcIndex:        map[string]string{},
		Targets:         map[string]domain.CatalogTarget{},
		Associations:    map[string]domain.AssociationDecision{},
		SolveJobs:       map[string]domain.SolveJob{},
		Solutions:       map[string]domain.OrbitSolution{},
		Reviews:         map[string]domain.ResidualReview{},
		FrozenVersions:  map[string]domain.FrozenVersion{},
		NextTarget:      1,
		NextJob:         1,
		NextSolution:    1,
		NextEvent:       1,
	}
	s.Stations["STA-ALPHA"] = domain.Station{ID: "STA-ALPHA", Name: "Alpha Ridge Optical", LatitudeDeg: 35.247, LongitudeDeg: -116.793, AltitudeM: 940, Enabled: true, AllowedClockDriftMS: 500, ConfigRevision: 1, UpdatedAt: now}
	s.Stations["STA-BRAVO"] = domain.Station{ID: "STA-BRAVO", Name: "Bravo Plateau Optical", LatitudeDeg: 19.826, LongitudeDeg: -155.47, AltitudeM: 3400, Enabled: true, AllowedClockDriftMS: 500, ConfigRevision: 1, UpdatedAt: now}
	s.Stations["STA-DRIFT"] = domain.Station{ID: "STA-DRIFT", Name: "Drift Isolation Station", LatitudeDeg: 40.0, LongitudeDeg: 70.0, AltitudeM: 1200, Enabled: true, AllowedClockDriftMS: 50, CurrentDriftMS: 125, ConfigRevision: 1, UpdatedAt: now}
	return s
}

func (s *State) Ensure() {
	if s.Stations == nil {
		s.Stations = map[string]domain.Station{}
	}
	if s.ObservationArcs == nil {
		s.ObservationArcs = map[string]domain.ObservationArc{}
	}
	if s.ArcIndex == nil {
		s.ArcIndex = map[string]string{}
	}
	if s.Targets == nil {
		s.Targets = map[string]domain.CatalogTarget{}
	}
	if s.Associations == nil {
		s.Associations = map[string]domain.AssociationDecision{}
	}
	if s.SolveJobs == nil {
		s.SolveJobs = map[string]domain.SolveJob{}
	}
	if s.Solutions == nil {
		s.Solutions = map[string]domain.OrbitSolution{}
	}
	if s.Reviews == nil {
		s.Reviews = map[string]domain.ResidualReview{}
	}
	if s.FrozenVersions == nil {
		s.FrozenVersions = map[string]domain.FrozenVersion{}
	}
	if s.NextTarget == 0 {
		s.NextTarget = 1
	}
	if s.NextJob == 0 {
		s.NextJob = 1
	}
	if s.NextSolution == 0 {
		s.NextSolution = 1
	}
	if s.NextEvent == 0 {
		s.NextEvent = int64(len(s.Events)) + 1
		if s.NextEvent == 0 {
			s.NextEvent = 1
		}
	}
}

func (s *State) TouchCheckpoint() error {
	hash, err := domain.HashAny(struct {
		Stations  int   `json:"stations"`
		Arcs      int   `json:"arcs"`
		Targets   int   `json:"targets"`
		Jobs      int   `json:"jobs"`
		Solutions int   `json:"solutions"`
		Reviews   int   `json:"reviews"`
		Versions  int   `json:"versions"`
		LastEvent int64 `json:"last_event"`
	}{
		Stations: len(s.Stations), Arcs: len(s.ObservationArcs), Targets: len(s.Targets),
		Jobs: len(s.SolveJobs), Solutions: len(s.Solutions), Reviews: len(s.Reviews),
		Versions: len(s.FrozenVersions), LastEvent: s.NextEvent - 1,
	})
	if err != nil {
		return err
	}
	s.Checkpoint = domain.Checkpoint{LastAppliedSeq: s.NextEvent - 1, StateHash: hash, UpdatedAt: time.Now().UTC()}
	return nil
}
