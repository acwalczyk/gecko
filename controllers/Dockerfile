FROM registry.access.redhat.com/ubi9/go-toolset:latest AS builder
USER root
WORKDIR /build
# Note: platform-api is referenced via replace directive (../platform-api).
# The build context must include both controllers/ and platform-api/ directories.
# See CI configuration for the correct build context setup.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /build/gecko-controllers .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /build/gecko-controllers /gecko-controllers
ENTRYPOINT ["/gecko-controllers"]
