FROM golang:1.22-alpine AS builder
WORKDIR /app
RUN apk add --no-cache wget
COPY go.mod ./
COPY . .
RUN mkdir -p static && \
    wget -q -O static/chart.min.js https://cdn.jsdelivr.net/npm/chart.js@4.4.2/dist/chart.umd.min.js
RUN go mod tidy && CGO_ENABLED=0 go build -o api .

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /app/api .
EXPOSE 8080
CMD ["./api"]
