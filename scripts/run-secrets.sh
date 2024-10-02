#!/bin/bash

echo
echo Start kind: kind create cluster --name lab
echo

export TRACE=false
export DEBUG_LOG=false
export SECRET_DEBUG=false
export KUBEGROUP_FORCE_NAMESPACE_DEFAULT=true
export OTELCONFIG_EXPORTER=jaeger
export OTEL_TRACES_EXPORTER=jaeger
export OTEL_PROPAGATORS=b3multi
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:14268

secrets