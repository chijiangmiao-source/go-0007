package api

import (
	"leo-debris-orbit-loop/internal/domain"
	"leo-debris-orbit-loop/internal/persistence"
	"net/http"
	"strconv"
)

func (a *App) eventJournal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, domain.NewError(domain.CodeValidation, "method not allowed"))
		return
	}
	after, _ := strconv.ParseInt(r.URL.Query().Get("after_seq"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	var page persistence.EventPage
	err := a.Store.View(func(st *persistence.State) error {
		page = persistence.QueryEvents(st, persistence.EventQuery{AfterSeq: after, Limit: limit})
		return nil
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
