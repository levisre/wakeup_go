FROM golang:alpine AS builder

RUN apk add make binutils

WORKDIR /build
COPY *.go /build/

COPY Makefile /build/

COPY go.mod /build/
COPY go.sum /build/
RUN go mod download
RUN make release
RUN strip -s --remove-section=.gosymtab --remove-section=.go.buildinfo /build/wakebot_go
FROM scratch AS export-stage
COPY --from=builder /build/wakebot_go ./
