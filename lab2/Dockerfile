FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /lab2-api main.go

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /lab2-api .
EXPOSE 8080
CMD ["./lab2-api"]