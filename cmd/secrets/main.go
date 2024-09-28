// Package main implements the secrets service.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	_ "github.com/KimMachineGun/automemlimit"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/udhos/boilerplate/boilerplate"
	"github.com/udhos/otelconfig/oteltrace"
	_ "go.uber.org/automaxprocs"
)

func main() {

	//
	// command-line
	//
	var showVersion bool
	flag.BoolVar(&showVersion, "version", showVersion, "show version")
	flag.Parse()

	me := filepath.Base(os.Args[0])

	//
	// version
	//
	{
		v := boilerplate.LongVersion(me + " version=" + version)
		if showVersion {
			fmt.Print(v)
			fmt.Println()
			return
		}
		log.Info().Msg(v)
	}

	//
	// application
	//

	app := newApplication(me)

	if app.cfg.debugLog {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	//
	// initialize tracing
	//

	{
		options := oteltrace.TraceOptions{
			DefaultService:     me,
			NoopTracerProvider: false,
			Debug:              true,
		}

		tracer, cancel, errTracer := oteltrace.TraceStart(options)
		if errTracer != nil {
			log.Fatal().Msgf("tracer: %v", errTracer)
		}

		defer cancel()

		app.tracer = tracer
	}

	//
	// start application server
	//

	go app.run()

	//
	// start health server
	//

	{
		log.Info().Msgf("registering health route: %s %s",
			app.cfg.healthAddr, app.cfg.healthPath)

		mux := http.NewServeMux()
		app.serverHealth = &http.Server{Addr: app.cfg.healthAddr, Handler: mux}
		mux.HandleFunc(app.cfg.healthPath, func(w http.ResponseWriter,
			_ *http.Request) {
			fmt.Fprintln(w, "health ok")
		})

		go func() {
			log.Info().Msgf("health server: listening on %s %s",
				app.cfg.healthAddr, app.cfg.healthPath)
			err := app.serverHealth.ListenAndServe()
			log.Info().Msgf("health server: exited: %v", err)
		}()
	}

	//
	// start metrics server
	//

	{
		log.Info().Msgf("registering metrics route: %s %s",
			app.cfg.metricsAddr, app.cfg.metricsPath)

		mux := http.NewServeMux()
		app.serverMetrics = &http.Server{Addr: app.cfg.metricsAddr, Handler: mux}
		mux.Handle(app.cfg.metricsPath, app.metricsHandler())

		go func() {
			log.Info().Msgf("metrics server: listening on %s %s",
				app.cfg.metricsAddr, app.cfg.metricsPath)
			err := app.serverMetrics.ListenAndServe()
			log.Error().Msgf("metrics server: exited: %v", err)
		}()
	}

	gracefulShutdown(app)
}

func gracefulShutdown(app *application) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit

	log.Info().Msgf("received signal '%v', initiating shutdown", sig)

	app.stop()

	log.Info().Msgf("exiting")
}
