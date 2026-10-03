FROM golang:1.27.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY db ./db
RUN mkdir /out && for app in api worker media-worker main-bot group-bot downloader-bot customer-bot-runtime migrate; do CGO_ENABLED=0 go build -trimpath -o /out/$app ./cmd/$app; done
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg && rm -rf /var/lib/apt/lists/* && useradd --uid 10001 --create-home app
COPY --from=build /out /app
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /app/zoneinfo.zip
ENV ZONEINFO=/app/zoneinfo.zip
RUN test -s /app/zoneinfo.zip
USER 10001:10001
WORKDIR /app
EXPOSE 8080
CMD ["/app/api"]
