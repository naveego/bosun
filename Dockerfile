FROM golang:1.11-stretch as builder

WORKDIR /bosun

# Populate the module cache based on the go.{mod,sum} files.
COPY go.mod .
COPY go.sum .

RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -ldflags '-w -s' .

# Runtime image
FROM alpine AS base
RUN apk update && apk add ca-certificates && rm -rf /var/cache/apk/*

COPY --from=builder /bosun/bosun /bin/bosun
CMD ["/bin/bosun"]