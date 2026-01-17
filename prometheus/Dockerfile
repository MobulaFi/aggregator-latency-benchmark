FROM prom/prometheus:latest

USER root

# Copy config from the prometheus folder
COPY prometheus/prometheus.yml /etc/prometheus/prometheus.yml

# Fix permissions for the data directory (for Railway volumes)
RUN mkdir -p /prometheus && chown -R nobody:nobody /prometheus

USER nobody

EXPOSE 9090

CMD ["--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus", "--web.enable-lifecycle"]
