FROM golang:1.27.2-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db
COPY scripts/release/phase51j_guard.go ./scripts/release/phase51j_guard.go
# Evaluate the actual selected GoFiles (not just source filenames in the tree).
# Reject signed TEST V2/browser fixture code and require original trusted
# OpenEnvironment startup before any potentially redistributable app build.
RUN /bin/bash -o pipefail -c 'GOFLAGS= GOWORK=off go list -json -deps ./cmd/... | GOFLAGS= GOWORK=off go run ./scripts/release/phase51j_guard.go selected'
# Every release binary must be built with no inherited Go build tags. Validate
# compiler provenance and CGO=0 on each built ELF, not only on /out/api.
RUN mkdir /out && for app in api worker media-worker main-bot group-bot downloader-bot customer-bot-runtime migrate; do \
    GOFLAGS= GOWORK=off CGO_ENABLED=0 go build -trimpath -o /out/$app ./cmd/$app && \
    GOFLAGS= GOWORK=off go run ./scripts/release/phase51j_guard.go binary /out/$app go1.27.2 || exit 1; \
  done
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg python3 python3-venv && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home app
COPY scripts/media/requirements.txt /tmp/extractor-requirements.txt
RUN python3 -m venv /opt/extractor && /opt/extractor/bin/pip install --no-cache-dir --require-hashes --no-deps -r /tmp/extractor-requirements.txt
COPY scripts/media/extract.py /app/extract.py
COPY --from=build /out /app
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /app/zoneinfo.zip
ENV ZONEINFO=/app/zoneinfo.zip
RUN test -s /app/zoneinfo.zip
USER 10001:10001
WORKDIR /app
EXPOSE 8080
CMD ["/app/api"]
