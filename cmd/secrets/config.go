package main

import (
	"time"

	"github.com/udhos/boilerplate/envconfig"
)

type config struct {
	debugLog                       bool
	listenAddr                     string
	httpClientTimeout              time.Duration
	cacheTTL                       time.Duration
	cacheErrorTTL                  time.Duration
	healthAddr                     string
	healthPath                     string
	metricsAddr                    string
	metricsPath                    string
	metricsNamespace               string
	metricsBucketsLatencyHTTP      []float64
	groupcachePort                 string
	groupcacheSizeBytes            int64
	kubegroupMetricsNamespace      string
	kubegroupDebug                 bool
	kubegroupLabelSelector         string
	kubegroupForceNamespaceDefault bool
}

func newConfig(roleSessionName string) config {

	env := envconfig.NewSimple(roleSessionName)

	return config{
		debugLog:          env.Bool("DEBUG_LOG", true),
		listenAddr:        env.String("LISTEN_ADDR", ":8080"),
		httpClientTimeout: env.Duration("HTTP_CLIENT_TIMEOUT", 300*time.Second),
		cacheTTL:          env.Duration("CACHE_TTL", 600*time.Second),
		cacheErrorTTL:     env.Duration("CACHE_ERROR_TTL", 60*time.Second),
		healthAddr:        env.String("HEALTH_ADDR", ":8888"),
		healthPath:        env.String("HEALTH_PATH", "/health"),
		metricsAddr:       env.String("METRICS_ADDR", ":3000"),
		metricsPath:       env.String("METRICS_PATH", "/metrics"),
		metricsNamespace:  env.String("METRICS_NAMESPACE", ""),
		metricsBucketsLatencyHTTP: env.Float64Slice("METRICS_BUCKETS_LATENCY_HTTP",
			[]float64{0.00005, 0.001, 0.0025, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5, 10}),
		groupcachePort:                 env.String("GROUPCACHE_PORT", ":5000"),
		groupcacheSizeBytes:            env.Int64("GROUPCACHE_SIZE_BYTES", 100_000_000),
		kubegroupMetricsNamespace:      env.String("KUBEGROUP_METRICS_NAMESPACE", ""),
		kubegroupDebug:                 env.Bool("KUBEGROUP_DEBUG", true),
		kubegroupLabelSelector:         env.String("KUBEGROUP_LABEL_SELECTOR", "app=secrets"),
		kubegroupForceNamespaceDefault: env.Bool("KUBEGROUP_FORCE_NAMESPACE_DEFAULT", false),
	}
}
