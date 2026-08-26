package api

import (
	"encoding/json"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"net/http"
	"strings"
)

func (a *App) stationCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	stations, err := a.Intake.ListStations()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stations)
}

func (a *App) stationRoutes(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/v1/stations/"))
	if len(parts) == 2 && parts[1] == "clock-drift" && r.Method == http.MethodPost {
		var req intake.StationDriftUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
			return
		}
		station, err := a.Intake.UpdateStationDrift(parts[0], req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, station)
		return
	}
	if len(parts) == 2 && parts[1] == "status" && r.Method == http.MethodPost {
		var req intake.StationStatusUpdate
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
			return
		}
		station, err := a.Intake.UpdateStationStatus(parts[0], req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, station)
		return
	}
	writeError(w, domain.NewError(domain.CodeNotFound, "station route not found"))
}
