FROM mcr.microsoft.com/devcontainers/base:bookworm

# exiftool backs the tag-import enricher (XMP dc:subject / IPTC Keywords).
RUN apt-get update \
    && apt-get install -y --no-install-recommends libimage-exiftool-perl \
    && rm -rf /var/lib/apt/lists/*

# Same version as the Makefile and CI; build context is the repo root.
COPY .golangci-lint-version /tmp/
RUN curl -sSfL https://golangci-lint.run/install.sh \
    | sh -s -- -b /usr/local/bin "$(cat /tmp/.golangci-lint-version)" \
    && rm /tmp/.golangci-lint-version
