FROM prom/prometheus:v2.49.1

USER root

# Copy config from the prometheus folder
COPY prometheus/prometheus.yml /etc/prometheus/prometheus.yml

EXPOSE 9090

# Run as root to avoid permission issues with Railway volumes
CMD ["--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus", "--web.enable-lifecycle"]
