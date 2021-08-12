FROM golang:1.16-stretch as builder


ENV CGO_ENABLED=0

RUN go install -a -v runtime

WORKDIR /bosun

# Populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .
RUN go build -a -ldflags '-w -s' .

# Runtime image
FROM alpine AS base


RUN apk update && apk add ca-certificates unzip && rm -rf /var/cache/apk/*

ADD https://releases.hashicorp.com/vault/1.8.1/vault_1.8.1_linux_amd64.zip .

RUN unzip vault_1.8.1_linux_amd64.zip -d /usr/bin

COPY --from=builder /bosun/bosun /bin/bosun

COPY examples/bosun.yaml /root/.bosun/bosun.yaml

ENTRYPOINT ["/bin/bosun"]