# Build stage
FROM golang:1.21 AS builder
WORKDIR /app
COPY . .
RUN go mod tidy
RUN go build -o main .

# Runtime stage
FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/main .
EXPOSE 8081
CMD ["./main"]

