FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder
USER root
WORKDIR /build# Build context must be the repo root (gecko/).
# The replace directives in controllers/go.mod reference ../platform-api and ../orlop.
COPY orlop/ ./orlop/
COPY platform-api/ ./platform-api/
COPY controllers/ ./controllers/
WORKDIR /build/controllers
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/gecko-controllers .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/gecko-controllers /gecko-controllers
ENTRYPOINT ["/gecko-controllers"]
