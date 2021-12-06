FROM golang:1.17-buster AS build
WORKDIR /build
COPY ../ .
RUN go build -o raft ./cmd

FROM alpine:latest
COPY --from=build /build/raft /usr/bin/raft
ENTRYPOINT [ "/usr/bin/raft" ]