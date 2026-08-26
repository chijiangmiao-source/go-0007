package intake

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"math"
	"sort"
)

type StationDriftUpdate struct {
	CurrentDriftMS int64  `json:"current_drift_ms"`
	Reason         string `json:"reason"`
}

type StationStatusUpdate struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason"`
}

func (s *Service) ListStations() ([]domain.Station, error) {
	var stations []domain.Station
	err := s.store.View(func(st *persistence.State) error {
		for _, station := range st.Stations {
			stations = append(stations, station)
		}
		sort.Slice(stations, func(i, j int) bool { return stations[i].ID < stations[j].ID })
		return nil
	})
	return stations, err
}

func (s *Service) UpdateStationDrift(stationID string, req StationDriftUpdate) (domain.Station, error) {
	if req.Reason == "" {
		return domain.Station{}, domain.NewError(domain.CodeValidation, "drift update reason is required")
	}
	var station domain.Station
	err := s.store.Update(func(st *persistence.State) error {
		current, ok := st.Stations[stationID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "station %s not found", stationID)
		}
		current.CurrentDriftMS = req.CurrentDriftMS
		current.ConfigRevision++
		current.UpdatedAt = domain.NowUTC()
		hash, err := domain.HashAny(current)
		if err != nil {
			return err
		}
		persistence.AppendEvent(st, "station.clock_drift_updated", stationID, hash, req.Reason)
		st.Stations[stationID] = current
		station = current
		return nil
	})
	return station, err
}

func (s *Service) UpdateStationStatus(stationID string, req StationStatusUpdate) (domain.Station, error) {
	if req.Reason == "" {
		return domain.Station{}, domain.NewError(domain.CodeValidation, "station status reason is required")
	}
	var station domain.Station
	err := s.store.Update(func(st *persistence.State) error {
		current, ok := st.Stations[stationID]
		if !ok {
			return domain.Errorf(domain.CodeNotFound, "station %s not found", stationID)
		}
		current.Enabled = req.Enabled
		current.ConfigRevision++
		current.UpdatedAt = domain.NowUTC()
		hash, err := domain.HashAny(current)
		if err != nil {
			return err
		}
		persistence.AppendEvent(st, "station.status_updated", stationID, hash, req.Reason)
		st.Stations[stationID] = current
		station = current
		return nil
	})
	return station, err
}

func StationAcceptsAssociation(station domain.Station) bool {
	return station.Enabled && math.Abs(float64(station.CurrentDriftMS)) <= float64(station.AllowedClockDriftMS)
}
