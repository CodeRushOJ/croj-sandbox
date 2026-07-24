# Dockerfile for croj-sandbox api-server

# Stage 1: Build the application
FROM golang:1.26.5-bookworm@sha256:1ecb7edf62a0408027bd5729dfd6b1b8766e578e8df93995b225dfd0944eb651 AS builder

ARG GOPROXY=https://proxy.golang.org,direct

# Install necessary build dependencies, including libseccomp for seccomp support
# Cgroup v2 support is primarily kernel-level; the target node must support it.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libseccomp-dev \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go module files and download dependencies first to leverage Docker cache
COPY go.mod go.sum ./
RUN GOPROXY="${GOPROXY}" go mod download

# Copy the rest of the application source code
COPY . .

# Build the api-server binary
# CGO is required by libseccomp-golang. Strip debug information to reduce size.
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /api-server ./cmd/api-server \
    && CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /sandbox-exec ./cmd/sandbox-exec

# Stage 2: Create the final minimal image
FROM debian:bookworm-slim@sha256:7b140f374b289a7c2befc338f42ebe6441b7ea838a042bbd5acbfca6ec875818

# Install runtime libraries and the C++, Python, Java and JavaScript toolchains.
# The Go toolchain is copied from the pinned builder stage below.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libseccomp2 \
    ca-certificates \
    gcc \
    g++ \
    python3 \
    openjdk-17-jdk-headless \
    nodejs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /api-server /app/api-server
COPY --from=builder /sandbox-exec /app/sandbox-exec
COPY --from=builder /usr/local/go /usr/local/go

ENV PATH="/usr/local/go/bin:${PATH}"

# Expose the default gRPC port
EXPOSE 50051

# Set the entrypoint to run the api-server
ENTRYPOINT ["/app/api-server"]

# Default command arguments (can be overridden at runtime)
CMD []
