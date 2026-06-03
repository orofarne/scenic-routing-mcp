# syntax=docker/dockerfile:1
# Base image is supplied via docker compose additional_contexts (no registry pull).
FROM valhalla-base

COPY watch.sh /scripts/
RUN chmod +x /scripts/watch.sh

ENTRYPOINT []
CMD ["/scripts/watch.sh"]
