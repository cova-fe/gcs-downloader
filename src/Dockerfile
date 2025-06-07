# Stage 1: Build the Go application
FROM golang:1.24 AS builder

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY Makefile ./
COPY VERSION ./
COPY gcs-downloader.go ./

RUN make build-linux-amd64

# Stage 2: Create the final minimal image
FROM alpine:latest

# Install tzdata and set timezone for accurate logs
RUN apk add --no-cache tzdata
ENV TZ=Europe/Berlin

WORKDIR /app

COPY --from=builder /app/gcs-downloader-linux-amd64 ./gcs-downloader

RUN mkdir -p /app/downloads

# Set the entrypoint to run the downloader application
ENTRYPOINT ["./gcs-downloader"]

# Example of default command if no arguments are provided (can be overridden)
CMD ["--dest", "/app/downloads", "--bucket", "your-gcs-bucket-name", "--impersonate-sa", "file-downloader-sa@your-project-id.iam.gserviceaccount.com", "--interval", "30s"]
