FROM golang:1.27 AS builder
WORKDIR /app
COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o api_service main.go

FROM scratch
WORKDIR /

COPY --from=builder /app/api_service /api_service
EXPOSE 8080
CMD [ "/api_service" ]