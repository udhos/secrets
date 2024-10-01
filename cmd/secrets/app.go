package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/modernprogram/groupcache/v2"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/udhos/boilerplate/secret"
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
	secretClient     *secret.Secret
}

func newApplication(me string) *application {

	roleArn := os.Getenv("SECRET_ROLE_ARN")

	log.Info().Msgf("envconfig.NewSimple: SECRET_ROLE_ARN='%s'", roleArn)

	secretOptions := secret.Options{
		RoleSessionName: me,
		RoleArn:         roleArn,
		Debug:           true,
	}
	secretClient := secret.New(secretOptions)

	app := &application{
		registry:     prometheus.NewRegistry(),
		cfg:          newConfig(secretClient),
		tracer:       oteltrace.NewNoopTracer(),
		secretClient: secretClient,
	}

	initApplication(app, app.cfg.kubegroupForceNamespaceDefault)

	return app
}

func initApplication(app *application, forceNamespaceDefault bool) {

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

	log.Info().Msgf("registering route: %s %s", app.cfg.listenAddr, app.cfg.appPath)

	mux.Handle(app.cfg.appPath, otelhttp.NewHandler(app, "app.ServerHTTP"))
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
var traceElapsed = attribute.Key("elapsed")
var traceReqIP = attribute.Key("request_ip")
var traceResponseError = attribute.Key("response_error")

func ipFromPort(hostPort string) string {
	i := strings.LastIndexByte(hostPort, ':')
	if i < 0 {
		return hostPort
	}
	return hostPort[i+1:]
}

func (app *application) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	const me = "app.ServeHTTP"
	ctx, span := app.tracer.Start(r.Context(), me)
	defer span.End()

	begin := time.Now()

	uri := r.URL.String()

	method := r.Method

	reqIP := ipFromPort(r.RemoteAddr)

	resp, status, errFetch := app.handle(ctx, r)

	elap := time.Since(begin)

	isFetchError := errFetch != nil

	var errorMessage string
	if isFetchError {
		errorMessage = errFetch.Error()
	}

	outcome := outcomeFrom(status, isFetchError)

	app.metrics.recordLatency(method, strconv.Itoa(status), uri, outcome, elap)

	//
	// log query status
	//
	{
		traceID := span.SpanContext().TraceID().String()
		if !isFetchError {
			//
			// http success
			//
			log.Debug().Str("traceID", traceID).Str("request_ip", reqIP).Str("method", method).Str("uri", uri).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s url=%s elapsed=%v", traceID, method, uri, elap)
		} else {
			log.Error().Str("traceID", traceID).Str("request_ip", reqIP).Str("method", method).Str("uri", uri).Str("response_error", errFetch.Error()).Dur("elapsed", elap).Msgf("ServeHTTP: traceID=%s method=%s uri=%s elapsed=%v response_error:%v", traceID, method, uri, elap, errFetch)
		}
	}

	span.SetAttributes(
		traceMethod.String(method),
		traceURI.String(uri),
		traceElapsed.String(elap.String()),
		traceReqIP.String(reqIP),
	)
	if isFetchError {
		span.SetAttributes(traceResponseError.String(errorMessage))
	}

	if status == 0 {
		if isFetchError {
			status = 500
		} else {
			status = 200
		}
	}

	httpResponse(w, resp.SecretValue, errorMessage, status)
}

func httpResponse(w http.ResponseWriter, secretValue, errorMessage string, code int) {

	body := secretPayload{
		SecretValue: secretValue,
		Error:       errorMessage,
	}

	if code != 0 {
		body.Status = strconv.Itoa(code)
	}

	data, errJSON := json.Marshal(body)
	if errJSON != nil {
		log.Error().Msgf("httpResponse: json error: %v", errJSON)
	}

	dataStr := string(data)

	log.Info().Msgf("httpResponse: response: %s", dataStr)

	h := w.Header()
	h.Del("Content-Length")

	// There might be content type already set, but we reset it to
	// text/plain for the error message.
	h.Set("Content-Type", "application/json; charset=utf-8")
	h.Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	fmt.Fprintln(w, dataStr)
}

func (app *application) handle(c context.Context, r *http.Request) (cacheResponse, int, error) {

	const me = "app.handle"
	ctx, span := app.tracer.Start(c, me)
	defer span.End()

	var resp cacheResponse

	body, errBody := io.ReadAll(r.Body)
	if errBody != nil {
		return resp, 400, errBody
	}

	bodyStr := string(body)

	log.Info().Msgf("%s: body:%s", me, bodyStr)

	var payload secretPayload

	if errJSON := json.Unmarshal(body, &payload); errJSON != nil {
		return resp, 400, errJSON
	}

	log.Info().Msgf("%s: secret_name:%s", me, payload.SecretName)

	key := strings.TrimSpace(payload.SecretName)

	if key == "" {
		return resp, 400, fmt.Errorf("%s: invalid empty secret name", me)
	}

	resp, errFetch := app.query(ctx, key)

	log.Info().Msgf("%s: secret_name:%s secret_value:%s",
		me, payload.SecretName, resp.SecretValue)

	return resp, 0, errFetch
}

func (app *application) query(c context.Context, key string) (cacheResponse, error) {

	const me = "app.query"
	ctx, span := app.tracer.Start(c, me)
	defer span.End()

	var resp cacheResponse
	var data []byte

	if errGet := app.cache.Get(ctx, key, groupcache.AllocatingByteSliceSink(&data)); errGet != nil {
		log.Error().Msgf("key='%s' cache error:%v", key, errGet)
		return resp, errGet
	}

	if errJ := json.Unmarshal(data, &resp); errJ != nil {
		log.Error().Msgf("key='%s' json error:%v", key, errJ)
		return resp, errJ
	}

	return resp, nil
}

type secretPayload struct {
	SecretName  string `json:"secret_name,omitempty"`
	SecretValue string `json:"secret_value,omitempty"`
	Error       string `json:"error,omitempty"`
	Status      string `json:"status,omitempty"`
}
