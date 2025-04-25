# Dockerfile for croj-sandbox api-server

# Stage 1: Build the application
FROM golang:1.21-bookworm AS builder

# Install necessary build dependencies, including libseccomp for seccomp support
# Cgroup v2 support is primarily kernel-level, ensure host supports it.
# Debian Bookworm (base for golang:1.21-bookworm) generally works well with cgroup v2.
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
# Use CGO_ENABLED=1 to link against libseccomp if needed by dependencies (though direct Go seccomp bindings might not require it)
# Add -ldflags "-s -w" to strip debug information and reduce binary size
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o /api-server ./cmd/api-server

# Stage 2: Create the final minimal image
FROM debian:bookworm-slim

# Install runtime dependencies, including libseccomp2
# Also install ca-certificates for potential HTTPS/TLS communication (e.g., with Zookeeper)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libseccomp2 \
    ca-certificates \
    # Add any other runtime dependencies needed by the sandbox languages (e.g., python3, openjdk-17-jre-headless, nodejs) 
    # Example: Add compilers/interpreters needed by the sandbox itself
    gcc g++ python3 openjdk-17-jre-headless nodejs \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy the built binary from the builder stage
COPY --from=builder /api-server /app/api-server

# Copy necessary configuration or runtime files if any (currently seems self-contained)
# COPY config.yaml /app/config.yaml

# Expose the default gRPC port
EXPOSE 50051

# Set the entrypoint to run the api-server
# Default command can be overridden, e.g., to specify Zookeeper address
ENTRYPOINT ["/app/api-server"]

# Default command arguments (can be overridden at runtime)
# Example: CMD ["--zk=zookeeper:2181"] 
CMD []