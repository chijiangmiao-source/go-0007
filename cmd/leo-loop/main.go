package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"leo-debris-orbit-loop/internal/api"
	"leo-debris-orbit-loop/internal/config"
	"leo-debris-orbit-loop/internal/intake"
	"leo-debris-orbit-loop/internal/orbit"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	cfg := config.Load()
	fs := flag.NewFlagSet("leo-loop", flag.ContinueOnError)
	addr := fs.String("addr", cfg.Addr, "HTTP listen address")
	storePath := fs.String("store", cfg.StorePath, "transactional JSON store path")
	engineMode := fs.String("engine-mode", cfg.EngineMode, "deterministic engine mode")
	if len(args) == 0 {
		args = []string{"serve"}
	}
	cmd := args[0]
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	app := api.NewApp(*storePath, orbit.NewDeterministicEngine(*engineMode))
	if err := app.Recovery.Recover(); err != nil {
		return err
	}
	switch cmd {
	case "serve":
		return serve(app, *addr, cfg.ShutdownTimeout)
	case "demo":
		return demo(app)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
}

func serve(app *api.App, addr string, shutdownTimeout time.Duration) error {
	srv := &http.Server{Addr: addr, Handler: app.Handler()}
	errs := make(chan error, 1)
	go func() {
		errs <- srv.ListenAndServe()
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-stop:
		ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(ctx)
		fmt.Fprintf(os.Stderr, "stopped on %s\n", sig)
		return nil
	case err := <-errs:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func demo(app *api.App) error {
	req := intake.SubmitArcRequest{
		StationID: "STA-ALPHA", ArcID: "DEMO-ARC-001", Confidence: 0.93,
		Samples: []intake.SampleRequest{
			{Time: "2026-08-25T23:59:58.000Z", AzimuthDeg: 121.1, ElevationDeg: 41.2},
			{Time: "2026-08-26T00:00:04.000Z", AzimuthDeg: 121.4, ElevationDeg: 41.0},
			{Time: "2026-08-26T00:00:09.000Z", AzimuthDeg: 121.8, ElevationDeg: 40.8},
		},
	}
	result, err := app.Intake.SubmitArc(req)
	if err != nil {
		return err
	}
	if err := app.Association.ProcessPending(); err != nil {
		return err
	}
	arc, err := app.Intake.GetArc(result.ArcKey)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{"arc": arc.ID, "target": arc.AssociatedTargetID, "duplicate": result.Duplicate})
}
