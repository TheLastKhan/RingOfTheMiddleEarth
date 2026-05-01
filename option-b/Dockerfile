FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /rotr-engine ./cmd/server

FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

COPY --from=builder /rotr-engine /rotr-engine

EXPOSE 8080

ENTRYPOINT ["/rotr-engine"]
