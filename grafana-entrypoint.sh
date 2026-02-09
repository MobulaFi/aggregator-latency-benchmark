#!/bin/sh

# Create dashboards directory
mkdir -p /var/lib/grafana/dashboards

# Debug: print environment variables
echo "Checking environment..."
env | grep -i railway || echo "No RAILWAY variables found"
echo "RAILWAY_ENVIRONMENT=${RAILWAY_ENVIRONMENT}"
echo "RAILWAY_STATIC_URL=${RAILWAY_STATIC_URL}"

# Copy dashboards from source
if [ ! -z "$RAILWAY_ENVIRONMENT" ] || [ ! -z "$RAILWAY_STATIC_URL" ] || [ ! -z "$RAILWAY_SERVICE_NAME" ] || [ ! -z "$RAILWAY_PROJECT_ID" ]; then
  echo "Railway environment detected - hiding Quote API Latency Benchmark dashboard"
  cp /dashboards-source/head_lag.json /var/lib/grafana/dashboards/
else
  echo "Local environment - copying all dashboards"
  cp /dashboards-source/*.json /var/lib/grafana/dashboards/
fi

echo "Dashboards copied:"
ls -la /var/lib/grafana/dashboards/

# Start Grafana with default entrypoint
exec /run.sh
