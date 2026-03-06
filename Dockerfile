# Build stage
FROM golang:1.24-alpine AS builder

# Build arguments for version info
ARG VERSION=dev
ARG BUILD_DATE

WORKDIR /build

# Copy go.mod and go.sum for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with version embedding
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION} -X main.BuildDate=${BUILD_DATE}" \
    -o /build/openmodel ./cmd

# Runtime stage - distroless for minimal image
FROM gcr.io/distroless/static-debian12

# Copy binary from builder
COPY --from=builder /build/openmodel /bin/openmodel

# Expose default port
EXPOSE 12345

# Run the binary as non-root user for security
USER nonroot:nonroot

ENTRYPOINT ["/bin/openmodel"]