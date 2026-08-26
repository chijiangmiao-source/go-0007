package api

import (
	"leo-debris-orbit-loop/internal/association"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/orbit"
	"leo-debris-orbit-loop/internal/persistence"
	"leo-debris-orbit-loop/internal/quality"
	"leo-debris-orbit-loop/internal/recovery"
	"leo-debris-orbit-loop/internal/versioning"
)

type App struct {
	Store       *persistence.Store
	Intake      *intake.Service
	Association *association.Service
	Scheduler   *orbit.Scheduler
	Quality     *quality.Service
	Versioning  *versioning.Service
	Recovery    *recovery.Service
}

func NewApp(storePath string, engine orbit.Engine) *App {
	store := persistence.NewStore(storePath)
	return &App{
		Store:       store,
		Intake:      intake.New(store),
		Association: association.New(store),
		Scheduler:   orbit.NewScheduler(store, engine),
		Quality:     quality.New(store),
		Versioning:  versioning.New(store),
		Recovery:    recovery.New(store),
	}
}
