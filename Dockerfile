FROM golang:1.12 as builder
ENV GO111MODULE=on
WORKDIR /app
# Resolve dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download
# Build app
COPY . .
RUN GO111MODULE=on CGO_ENABLED=0 GOOS=linux go build -a

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/timatch .
STOPSIGNAL SIGINT
ENTRYPOINT ["./timatch"]
