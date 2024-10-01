#!/bin/bash

export KUBEGROUP_FORCE_NAMESPACE_DEFAULT=true
export OTELCONFIG_EXPORTER=jaeger
export OTEL_TRACES_EXPORTER=jaeger
export OTEL_PROPAGATORS=b3multi
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14268

secrets