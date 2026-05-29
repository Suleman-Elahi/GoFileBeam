# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Copy static files for embedding
RUN cp -r static internal/static/files

# Build the application with optimizations for smaller binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -ldflags="-w -s" -o gofilebeam ./cmd/gofilebeam

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy only the binary from builder (static files are embedded)
COPY --from=builder /app/gofilebeam .

# Create uploads directory
RUN mkdir -p /uploads

# Expose port
EXPOSE 8080

# Set environment variables
ENV GOFILEBEAM_PORT=8080
ENV GOFILEBEAM_HOST=0.0.0.0
ENV GOFILEBEAM_STORAGE_PATH=/uploads
ENV GOFILEBEAM_MAX_STORAGE_GB=1

# Run the application
CMD ["./gofilebeam"]