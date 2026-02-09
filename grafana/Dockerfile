FROM grafana/grafana:latest

USER root

# Cache buster - update this to force rebuild: v5
ARG CACHE_BUST=5

# Copy provisioning configs and dashboards source
COPY grafana/provisioning /etc/grafana/provisioning
COPY grafana/dashboards /dashboards-source

# Copy entrypoint script
COPY grafana-entrypoint.sh /grafana-entrypoint.sh
RUN chmod +x /grafana-entrypoint.sh

# Set permissions
RUN chown -R grafana:root /etc/grafana/provisioning /dashboards-source

USER grafana

# Environment variables
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
ENV GF_SECURITY_ADMIN_USER=admin
ENV GF_SECURITY_ADMIN_PASSWORD=admin
ENV GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH=/var/lib/grafana/dashboards/head_lag.json

EXPOSE 3000

# Use custom entrypoint
ENTRYPOINT ["/bin/sh", "/grafana-entrypoint.sh"]
