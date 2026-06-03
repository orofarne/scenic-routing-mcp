# syntax=docker/dockerfile:1
# Base image is supplied via docker compose additional_contexts (no registry pull).
FROM valhalla-base

COPY serve.sh /scripts/
RUN chmod +x /scripts/serve.sh

EXPOSE 8002
ENTRYPOINT []
CMD ["/scripts/serve.sh"]
