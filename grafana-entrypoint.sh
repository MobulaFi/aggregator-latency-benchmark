#!/bin/sh

# Create dashboards directory
mkdir -p /var/lib/grafana/dashboards

# Copy dashboards from source
if [ ! -z "$RAILWAY_ENVIRONMENT" ] || [ ! -z "$RAILWAY_STATIC_URL" ]; then
  echo "Railway environment detected - hiding Quote API Latency Benchmark dashboard"
  cp /dashboards-source/head_lag.json /var/lib/grafana/dashboards/
else
  echo "Local environment - copying all dashboards"
  cp /dashboards-source/*.json /var/lib/grafana/dashboards/
fi

# Start Grafana with default entrypoint
exec /run.sh
