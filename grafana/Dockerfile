FROM grafana/grafana:latest

USER root
COPY provisioning /etc/grafana/provisioning
COPY dashboards /var/lib/grafana/dashboards
RUN chown -R grafana:root /etc/grafana/provisioning /var/lib/grafana/dashboards
USER grafana

