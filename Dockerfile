FROM golang:alpine AS builder

WORKDIR /app

# Copy source code
COPY . .

RUN rm go.mod go.sum

RUN go mod init github.com/amaumene/gomenarr

RUN go mod tidy

RUN go get -u all

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o gomenarr ./cmd/gomenarr

# Final stage
FROM scratch

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/gomenarr .

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Create volume for config
VOLUME ["/config"]

# Set environment variable
ENV CONFIG_DIR=/config

# Expose HTTP port
EXPOSE 8080

# Run the application
CMD ["./gomenarr"]
