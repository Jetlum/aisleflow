FROM golang:1.26.1-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/analyzer ./backend/analytics/cmd/analyzer && \
    CGO_ENABLED=0 go build -o /out/worker ./backend/temporal/cmd/worker && \
    CGO_ENABLED=0 go build -o /out/demo ./cmd/demo && \
    CGO_ENABLED=0 go build -o /out/migrate ./cmd/migrate

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/ /app/
COPY config/baselines.json /app/config/baselines.json
ENTRYPOINT ["/app/analyzer"]
