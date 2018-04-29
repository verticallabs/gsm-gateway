FROM golang:1.10.1-alpine as build

# install tzdata and root cas
# https://github.com/jeremyhuiskamp/golang-docker-scratch/blob/master/Dockerfile
RUN apk --no-cache add tzdata zip ca-certificates
WORKDIR /usr/share/zoneinfo
# -0 means no compression.  Needed because go's
# tz loader doesn't handle compressed data.
RUN zip -r -0 /zoneinfo.zip .

# build app
WORKDIR /go/src/github.com/verticallabs/gsm-gateway
COPY . .
RUN GOOS=linux GOARCH=arm GOARM=7 go build

FROM armhf/alpine
WORKDIR /app

COPY --from=build /zoneinfo.zip /
ENV ZONEINFO /zoneinfo.zip

COPY --from=build /go/src/github.com/verticallabs/gsm-gateway/gsm-gateway gsm-gateway

ENTRYPOINT ["/app/gsm-gateway"]