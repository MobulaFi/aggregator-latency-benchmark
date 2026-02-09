#!/bin/sh

# Copy dashboards to runtime directory
mkdir -p /tmp/dashboards
cp /var/lib/grafana/dashboards/*.json /tmp/dashboards/

# Hide Quote API Latency Benchmark dashboard on Railway
if [ ! -z "$RAILWAY_ENVIRONMENT" ] || [ ! -z "$RAILWAY_STATIC_URL" ]; then
  echo "Railway environment detected - hiding Quote API Latency Benchmark dashboard"
  rm -f /tmp/dashboards/quote_api_latency.json
fi

# Replace mounted dashboards with filtered ones
rm -rf /var/lib/grafana/dashboards/*
cp /tmp/dashboards/*.json /var/lib/grafana/dashboards/

# Start Grafana with default entrypoint
exec /run.sh
