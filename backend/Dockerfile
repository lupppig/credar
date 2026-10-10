# syntax=docker/dockerfile:1.18

FROM golang:1.27.2-bookworm AS build

WORKDIR /source
COPY backend/go.mod ./
RUN go mod download
COPY backend/ ./
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/credar-api ./cmd/api

FROM scratch AS runtime

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/credar-api /credar-api

USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=3s --timeout=5s --start-period=10s --retries=30 CMD ["/credar-api", "healthcheck"]
ENTRYPOINT ["/credar-api"]
