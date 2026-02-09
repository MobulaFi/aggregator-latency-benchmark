#!/bin/sh

# Hide Quote API Latency Benchmark dashboard on Railway
if [ ! -z "$RAILWAY_ENVIRONMENT" ] || [ ! -z "$RAILWAY_STATIC_URL" ]; then
  echo "Railway environment detected - hiding Quote API Latency Benchmark dashboard"
  rm -f /var/lib/grafana/dashboards/quote_api_latency.json
fi

# Start Grafana with default entrypoint
exec /run.sh
