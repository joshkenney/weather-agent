FROM golang:1.22-alpine AS builder

# Install git and necessary build tools
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod ./
# COPY go.sum ./    # Uncomment if you have a go.sum file

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o weather-agent .

# Final stage: Create a minimal image
FROM alpine:latest

# Add ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy the binary from the builder stage
COPY --from=builder /app/weather-agent .

# Copy templates and static files
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static

# Create a non-root user to run the application
RUN adduser -D -g '' appuser
RUN chown -R appuser:appuser /app
USER appuser

# Expose port 3000 (as specified in your docker-compose.yml)
EXPOSE 3000

# Command to run the application
CMD ["./weather-agent"]
