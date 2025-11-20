# ==== Stage 1: Builder ====
FROM golang:alpine AS builder

# Install git (required if you have dependencies from git repositories)
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum first to leverage Docker cache
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# CGO_ENABLED=0: Build a statically linked binary (no C libraries dependency)
# -ldflags="-s -w": Strip debug information to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o server ./cmd/server/main.go

# ==== Stage 2: Runner ====
FROM alpine:latest

# Install certificates (for HTTPS) and timezone data
RUN apk --no-cache add ca-certificates tzdata

# Set working directory
WORKDIR /app

# Create a non-root user for security
RUN adduser -D -g '' appuser

# Copy the binary from the builder stage
COPY --from=builder /app/server .

# Copy documentation if your code reads it at runtime (optional based on your structure)
# COPY --from=builder /app/docs ./docs

# Use the non-root user
USER appuser

# Expose the port application runs on (default is usually 8080)
EXPOSE 8080

# Command to run the executable
CMD ["./server"]
