package persistence

import (
	"leo-debris-orbit-loop/internal/domain"
	"sort"
)

type EventQuery struct {
	AfterSeq int64
	Limit    int
}

type EventPage struct {
	Events  []domain.EventRecord `json:"events"`
	NextSeq int64                `json:"next_seq"`
}

func QueryEvents(st *State, q EventQuery) EventPage {
	limit := q.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	events := make([]domain.EventRecord, 0, limit)
	for _, ev := range st.Events {
		if ev.Seq <= q.AfterSeq {
			continue
		}
		events = append(events, ev)
		if len(events) == limit {
			break
		}
	}
	next := q.AfterSeq
	if len(events) > 0 {
		next = events[len(events)-1].Seq
	}
	return EventPage{Events: events, NextSeq: next}
}

func TargetIDs(st *State) []string {
	ids := make([]string, 0, len(st.Targets))
	for id := range st.Targets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func StateDigest(st *State) (string, error) {
	type targetDigest struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
		Version  int    `json:"version"`
	}
	targets := make([]targetDigest, 0, len(st.Targets))
	for _, id := range TargetIDs(st) {
		t := st.Targets[id]
		targets = append(targets, targetDigest{ID: id, Revision: t.AssociationRevision, Version: t.CurrentFrozenVersion})
	}
	return domain.HashAny(struct {
		ArcCount     int               `json:"arc_count"`
		TargetDigest []targetDigest    `json:"target_digest"`
		EventCount   int               `json:"event_count"`
		Checkpoint   domain.Checkpoint `json:"checkpoint"`
	}{ArcCount: len(st.ObservationArcs), TargetDigest: targets, EventCount: len(st.Events), Checkpoint: st.Checkpoint})
}
