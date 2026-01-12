FROM grafana/grafana:latest

USER root
COPY provisioning /etc/grafana/provisioning
COPY dashboards /var/lib/grafana/dashboards
RUN chown -R grafana:root /etc/grafana/provisioning /var/lib/grafana/dashboards
USER grafana

# Enable anonymous access (read-only)
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
ENV GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH=/var/lib/grafana/dashboards/pool_discovery_latency.json
