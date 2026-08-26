package api

import (
	"encoding/json"
	"errors"
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/quality"
	"leo-debris-orbit-loop/internal/versioning"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", a.health)
	mux.HandleFunc("/v1/system/state", a.systemState)
	mux.HandleFunc("/v1/system/events", a.eventJournal)
	mux.HandleFunc("/v1/stations", a.stationCollection)
	mux.HandleFunc("/v1/stations/", a.stationRoutes)
	mux.HandleFunc("/v1/observation-arcs", a.observationCollection)
	mux.HandleFunc("/v1/observation-arcs/", a.observationMember)
	mux.HandleFunc("/v1/targets/", a.targetRoutes)
	mux.HandleFunc("/v1/solve-jobs/", a.solveJobRoutes)
	mux.HandleFunc("/v1/solutions/", a.solutionRoutes)
	mux.Handle("/", http.FileServer(http.Dir(frontendDir())))
	return mux
}

func frontendDir() string {
	for _, root := range frontendRoots() {
		dist := filepath.Join(root, "web", "dist")
		if _, err := os.Stat(filepath.Join(dist, "index.html")); err == nil {
			return dist
		}
		src := filepath.Join(root, "web", "src")
		if _, err := os.Stat(filepath.Join(src, "index.html")); err == nil {
			return src
		}
	}
	return "web/src"
}

func frontendRoots() []string {
	wd, err := os.Getwd()
	if err != nil {
		return []string{"."}
	}
	roots := []string{wd}
	for i := 0; i < 4; i++ {
		next := filepath.Dir(roots[len(roots)-1])
		if next == roots[len(roots)-1] {
			break
		}
		roots = append(roots, next)
	}
	return roots
}

func (a *App) health(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (a *App) systemState(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	state, err := a.Store.Load()
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stations": len(state.Stations), "arcs": len(state.ObservationArcs), "targets": len(state.Targets), "jobs": len(state.SolveJobs), "solutions": len(state.Solutions), "versions": len(state.FrozenVersions), "checkpoint": state.Checkpoint})
}

func (a *App) observationCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	var req intake.SubmitArcRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
		return
	}
	result, err := a.Intake.SubmitArc(req)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := a.Association.ProcessPending(); err != nil {
		writeError(w, err)
		return
	}
	arc, _ := a.Intake.GetArc(result.ArcKey)
	result.AssociatedTargetID = arc.AssociatedTargetID
	status := http.StatusAccepted
	if result.Duplicate {
		status = http.StatusOK
	}
	writeJSON(w, status, result)
}

func (a *App) observationMember(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/observation-arcs/")
	arc, err := a.Intake.GetArc(id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, arc)
}

func (a *App) targetRoutes(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/v1/targets/"))
	if len(parts) == 1 && r.Method == http.MethodGet {
		a.getTarget(w, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "solve-jobs" && r.Method == http.MethodPost {
		a.createSolveJob(w, r, parts[0])
		return
	}
	if len(parts) == 3 && parts[1] == "versions" && parts[2] == "freeze" && r.Method == http.MethodPost {
		a.freezeVersion(w, r, parts[0])
		return
	}
	if len(parts) == 2 && parts[1] == "versions" && r.Method == http.MethodGet {
		versions, err := a.Versioning.List(parts[0])
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, versions)
		return
	}
	if len(parts) == 3 && parts[1] == "versions" && r.Method == http.MethodGet {
		n, err := strconv.Atoi(parts[2])
		if err != nil {
			writeError(w, domain.NewError(domain.CodeValidation, "version must be numeric"))
			return
		}
		version, err := a.Versioning.Get(parts[0], n)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, version)
		return
	}
	writeError(w, domain.NewError(domain.CodeNotFound, "target route not found"))
}

func (a *App) getTarget(w http.ResponseWriter, targetID string) {
	target, arcs, job, review, err := a.Association.Target(targetID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"target": target, "arc_count": len(arcs), "latest_job": job, "latest_review": review})
}

func (a *App) createSolveJob(w http.ResponseWriter, r *http.Request, targetID string) {
	var req struct {
		ExpectedAssociationRevision int64 `json:"expected_association_revision"`
		RunNow                      bool  `json:"run_now"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
		return
	}
	job, err := a.Scheduler.CreateJob(targetID, req.ExpectedAssociationRevision)
	if err != nil {
		writeError(w, err)
		return
	}
	if req.RunNow {
		// Bind the synchronous solve to the client's request context so that a
		// client timeout or disconnect cancels the in-flight computation
		// promptly. The scheduler classifies context.Canceled as a canceled
		// terminal state and persists it, keeping status and failure class
		// queryable via GET /v1/solve-jobs/{job_id}.
		if err := a.Scheduler.RunJob(r.Context(), job.ID); err != nil {
			writeError(w, err)
			return
		}
		job, _ = a.Scheduler.GetJob(job.ID)
		if job.ResultSolutionID != "" {
			_, _ = a.Quality.EvaluateSolution(job.ResultSolutionID)
		}
	}
	writeJSON(w, http.StatusAccepted, job)
}

func (a *App) solveJobRoutes(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/v1/solve-jobs/"))
	if len(parts) == 1 && r.Method == http.MethodGet {
		job, err := a.Scheduler.GetJob(parts[0])
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, job)
		return
	}
	if len(parts) == 2 && parts[1] == "cancel" && r.Method == http.MethodPost {
		result, err := a.Scheduler.Cancel(parts[0], "api cancel")
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	writeError(w, domain.NewError(domain.CodeNotFound, "solve job route not found"))
}

func (a *App) solutionRoutes(w http.ResponseWriter, r *http.Request) {
	parts := splitPath(strings.TrimPrefix(r.URL.Path, "/v1/solutions/"))
	if len(parts) == 2 && parts[1] == "residuals" && r.Method == http.MethodGet {
		review, err := a.Quality.Get(parts[0])
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, review)
		return
	}
	if len(parts) == 2 && parts[1] == "review-decisions" && r.Method == http.MethodPost {
		var req quality.DecisionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
			return
		}
		review, err := a.Quality.Decide(parts[0], req)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, review)
		return
	}
	writeError(w, domain.NewError(domain.CodeNotFound, "solution route not found"))
}

func (a *App) freezeVersion(w http.ResponseWriter, r *http.Request, targetID string) {
	var req versioning.FreezeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, domain.NewError(domain.CodeValidation, "invalid json request"))
		return
	}
	version, err := a.Versioning.Freeze(targetID, req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, version)
}

func splitPath(s string) []string {
	raw := strings.Split(strings.Trim(s, "/"), "/")
	parts := raw[:0]
	for _, p := range raw {
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	var app *domain.AppError
	if errors.As(err, &app) {
		switch app.Code {
		case domain.CodeValidation:
			status = http.StatusUnprocessableEntity
		case domain.CodeNotFound:
			status = http.StatusNotFound
		case domain.CodeConflict, domain.CodeFrozenImmutable, domain.CodePreconditionFail:
			status = http.StatusConflict
		case domain.CodeIllegalState:
			status = http.StatusConflict
		case domain.CodeCanceled:
			status = http.StatusRequestTimeout
		}
		writeJSON(w, status, app)
		return
	}
	writeJSON(w, status, map[string]any{"code": "internal", "message": err.Error()})
}
