# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gofilebeam ./cmd/gofilebeam

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/gofilebeam .
COPY --from=builder /app/static ./static

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