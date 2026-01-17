FROM prom/prometheus:latest

USER root

# Copy config from the prometheus folder
COPY prometheus/prometheus.yml /etc/prometheus/prometheus.yml

# Create entrypoint script that fixes permissions at runtime
RUN echo '#!/bin/sh' > /entrypoint.sh && \
    echo 'chown -R nobody:nobody /prometheus 2>/dev/null || true' >> /entrypoint.sh && \
    echo 'exec su-exec nobody /bin/prometheus "$@"' >> /entrypoint.sh && \
    chmod +x /entrypoint.sh

EXPOSE 9090

ENTRYPOINT ["/entrypoint.sh"]
CMD ["--config.file=/etc/prometheus/prometheus.yml", "--storage.tsdb.path=/prometheus", "--web.enable-lifecycle"]
