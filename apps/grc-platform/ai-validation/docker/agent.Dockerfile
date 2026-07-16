# AI Validation Agent — build context is the ai-validation module root.
# ---- build ----
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /app ./cmd/agent

# ---- runtime: distroless, binary only, no shell/source ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /app /app
EXPOSE 8090
USER nonroot
ENTRYPOINT ["/app"]
