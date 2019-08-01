FROM golang:1.12 as builder
WORKDIR /app
# Resolve dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download
# Build + test app
COPY . .
ENV GO111MODULE=on
ENV GOOS=linux
ENV CGO_ENABLED=0
RUN go build -a -v
RUN go vet $(go list)
RUN CGO_ENABLED=1 go test -v -race -timeout 30s $(go list)

FROM alpine:latest
WORKDIR /root/
COPY --from=builder /app/timatch .
STOPSIGNAL SIGINT
ENTRYPOINT ["./timatch"]
