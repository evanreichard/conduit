# Certificates & Timezones
FROM alpine AS alpine
RUN apk update && apk add --no-cache ca-certificates tzdata

# Build Image
FROM golang:1.25 AS build

# Create Package Directory
RUN mkdir -p /opt/conduit

# Copy Source
WORKDIR /src
COPY . .

# Compile
RUN go build \
  -ldflags "-X reichard.io/conduit/config.version=`git describe --tags`" \
  -o /opt/conduit/conduit

# Create Image
FROM busybox:1.36
COPY --from=alpine /etc/ssl/certs /etc/ssl/certs
COPY --from=alpine /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /opt/conduit /opt/conduit
WORKDIR /opt/conduit
EXPOSE 8080
ENTRYPOINT ["/opt/conduit/conduit", "serve"]
