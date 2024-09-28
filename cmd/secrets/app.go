package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/udhos/otelconfig/oteltrace"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type application struct {
	cfg              config
	tracer           trace.Tracer
	registry         *prometheus.Registry
	metrics          *prometheusMetrics
	serverMain       *http.Server
	serverHealth     *http.Server
	serverMetrics    *http.Server
	serverGroupCache *http.Server
	cache            *groupcache.Group
	groupcacheClose  func()
	httpClient       *http.Client
}

func newApplication(me string) *application {
	app := &application{
		registry: prometheus.NewRegistry(),
		cfg:      newConfig(me),
		tracer:   oteltrace.NewNoopTracer(),
	}

	initApplication(app, app.cfg.kubegroupForceNamespaceDefault)

	return app
}

func initApplication(app *application, forceNamespaceDefault bool) {

	app.httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   app.cfg.httpClientTimeout,
	}

	//
	// add basic/default instrumentation
	//
	app.registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	app.registry.MustRegister(prometheus.NewGoCollector())

	app.metrics = newMetrics(app.registry, app.cfg.metricsNamespace,
		app.cfg.metricsBucketsLatencyHTTP)

	//
	// start group cache
	//
	app.groupcacheClose = startGroupcache(app, forceNamespaceDefault)

	//
	// register application route
	//

	mux := http.NewServeMux()
	app.serverMain = &http.Server{Addr: app.cfg.listenAddr, Handler: mux}

	const route = "/"

	log.Info().Msgf("registering route: %s %s", app.cfg.listenAddr, route)

	mux.Handle(route, otelhttp.NewHandler(app, "app.ServerHTTP"))
}

func (app *application) run() {
	log.Info().Msgf("application server: listening on %s", app.cfg.listenAddr)
	err := app.serverMain.ListenAndServe()
	log.Error().Msgf("application server: exited: %v", err)
}

func (app *application) stop() {
	app.groupcacheClose()
	const timeout = 5 * time.Second
	httpShutdown(app.serverHealth, "health", timeout)
	httpShutdown(app.serverMain, "main", timeout)
	httpShutdown(app.serverGroupCache, "groupcache", timeout)
	httpShutdown(app.serverMetrics, "metrics", timeout)
}

func httpShutdown(s *http.Server, label string, timeout time.Duration) {
	if s == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		log.Error().Msgf("http shutdown error: %s: %v", label, err)
	}
}

var traceMethod = attribute.Key("method")
var traceURI = attribute.Key("uri")
var traceResponseStatus = attribute.Key("response_status")
var traceResponseError = attribute.Key("response_error")
var traceElapsed = attribute.Key("elapsed")
var traceUseCache = attribute.Key("use_cache")
var traceReqIP = attribute.Key("request_ip")

func (app *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	const me = "app.ServeHTTP"
	ctx, span := app.tracer.Start(r.Context(), me)
	defer span.End()

	begin := time.Now()

	uri := r.URL.String()

	method := r.Method

	key := method + " " + uri

	//useCache := mustCache(method, r.URL.RequestURI(), app.restrictMethod, app.restrictRouteRegexp)

	reqIP, _, _ := strings.Cut(r.RemoteAddr, ":")

	resp, errFetch := app.query(ctx, key, reqIP)

	isFetchError := errFetch != nil

	elap := time.Since(begin)

	outcome := outcomeFrom(resp.Status, isFetchError)

	app.metrics.recordLatency(r.Method, strconv.Itoa(resp.Status), uri, outcome, elap)

	//
	// log query status
	//
	{
		traceID := span.SpanContext().TraceID().String()
		status := resp.Status
		if !isFetchError {
			if isHTTPError(status) {
				//
				// http error
				//
				bodyStr := string(resp.Body)
				log.Error().Str("traceID", traceID).Str("request_ip", reqIP).Str("method", method).Str("uri", uri).Int("response_status", status).Dur("elapsed", elap).Str("response_body", bodyStr).Msgf("ServeHTTP: traceID=%s method=%s url=%s response_status=%d elapsed=%v response_body:%s", traceID, method, uri, status, elap, bodyStr)
			} else {
				//
				// http success
				//
				log.Debug().Str("traceID", traceID).Str("request_ip", reqIP).Str("method", method).Str("uri", uri).Int("response_status", status).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s url=%s response_status=%d elapsed=%v", traceID, method, uri, status, elap)
			}
		} else {
			log.Error().Str("traceID", traceID).Str("request_ip", reqIP).Str("method", method).Str("uri", uri).Int("response_status", status).Str("response_error", errFetch.Error()).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s uri=%s response_status=%d elapsed=%v response_error:%v", traceID, method, uri, status, elap, errFetch)
		}
	}

	span.SetAttributes(
		traceMethod.String(method),
		traceURI.String(uri),
		traceResponseStatus.Int(resp.Status),
		traceElapsed.String(elap.String()),
		traceReqIP.String(reqIP),
	)
	if isFetchError {
		span.SetAttributes(traceResponseError.String(errFetch.Error()))
	}

	//
	// send response headers (1/3)
	//
	//w.Header().Add("a", "b")

	//
	// send response status (2/3)
	//
	if !isFetchError {
		w.WriteHeader(resp.Status)
	} else {
		w.WriteHeader(500)
	}

	//
	// send response body (3/3)
	//
	if !isFetchError {
		w.Write(resp.Body)
	} else {
		//
		// error
		//
		if len(resp.Body) > 0 {
			//
			// prefer received body
			//
			w.Write(resp.Body)
		} else {
			fmt.Fprint(w, errFetch.Error())
		}
	}
}

func (app *application) query(c context.Context, key, _ /*reqIP*/ string) (response, error) {

	const me = "app.query"
	ctx, span := app.tracer.Start(c, me)
	defer span.End()

	var resp response
	var data []byte

	if errGet := app.cache.Get(ctx, key, groupcache.AllocatingByteSliceSink(&data)); errGet != nil {
		log.Error().Msgf("key='%s' cache error:%v", key, errGet)
		resp.Status = 500
		return resp, errGet
	}

	if errJ := json.Unmarshal(data, &resp); errJ != nil {
		log.Error().Msgf("key='%s' json error:%v", key, errJ)
		resp.Status = 500
		return resp, errJ
	}

	return resp, nil
}
