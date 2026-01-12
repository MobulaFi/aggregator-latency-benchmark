FROM grafana/grafana:latest

USER root

# Copy provisioning configs from grafana folder
COPY grafana/provisioning /etc/grafana/provisioning
COPY grafana/dashboards /var/lib/grafana/dashboards

# Set permissions
RUN chown -R grafana:root /etc/grafana/provisioning /var/lib/grafana/dashboards

USER grafana

# Environment variables
ENV GF_AUTH_ANONYMOUS_ENABLED=true
ENV GF_AUTH_ANONYMOUS_ORG_ROLE=Viewer
ENV GF_SECURITY_ADMIN_USER=admin
ENV GF_SECURITY_ADMIN_PASSWORD=admin
ENV GF_DASHBOARDS_DEFAULT_HOME_DASHBOARD_PATH=/var/lib/grafana/dashboards/pool_discovery_latency.json

EXPOSE 3000
