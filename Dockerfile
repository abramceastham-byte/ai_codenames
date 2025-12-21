# docker build -t registryext.bsprague.com/codenames .
FROM golang:1.25.4 AS builder

RUN adduser \
  --disabled-password \
  --gecos "" \
  --home "/nonexistent" \
  --shell "/sbin/nologin" \
  --no-create-home \
  --uid 65532 \
  noroot

WORKDIR /build

# Copy dependency files first for caching
COPY go.mod go.sum ./
RUN go mod download && go mod verify

# Copy the rest of the source code
COPY cmd/codenames-server/ cmd/codenames-server/
COPY aiclient/ aiclient/
COPY boardgen/ boardgen/
COPY codenames/ codenames/
COPY consensus/ consensus/
COPY cryptorand/ cryptorand/
COPY dict/ dict/
COPY game/ game/
COPY httperr/ httperr/
COPY hub/ hub/
COPY sqldb/ sqldb/
COPY web/ web/

# Build the binary (statically linked)
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/codenames-server

FROM scratch

WORKDIR /app

COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy the binary
COPY --from=builder /server /server

# Expose the port the app runs on
EXPOSE 8080

# Run the binary
ENTRYPOINT ["/server"]
