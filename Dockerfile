FROM golang:alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Build the Go app
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/dbx-ray cmd/server/main.go

FROM alpine:latest
WORKDIR /app
COPY --from=builder /app/dbx-ray .

# Expose port 8080 for the API
EXPOSE 8080

CMD ["./dbx-ray"]
