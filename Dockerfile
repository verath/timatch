FROM golang:1.8-alpine

COPY . /go/src/github.com/verath/timatch


RUN go install github.com/verath/timatch

RUN rm -rf /go/src

STOPSIGNAL SIGINT

ENTRYPOINT ["timatch"]
