# Dockerfile for croj-sandbox api-server

# Stage 1: Build the application
FROM golang:1.24.6-bookworm AS builder

# Install necessary build dependencies, including libseccomp for seccomp support
# Cgroup v2 support is primarily kernel-level; the target node must support it.
RUN apt-get update && apt-get install -y --no-install-recommends \
    libseccomp-dev \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy go module files and download dependencies first to leverage Docker cache
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the application source code
COPY . .

# Build the api-server binary
# CGO is required by libseccomp-golang. Strip debug information to reduce size.
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /api-server ./cmd/api-server \
    && CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /sandbox-exec ./cmd/sandbox-exec

# Stage 2: Create the final minimal image
FROM debian:bookworm-slim

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
