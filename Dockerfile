FROM golang:1.10.1-alpine as build
WORKDIR /go/src/github.com/verticallabs/gsm-gateway
COPY . .
RUN GOOS=linux GOARCH=arm GOARM=7 go build

FROM scratch
WORKDIR /app
COPY --from=build /go/src/github.com/verticallabs/gsm-gateway/gsm-gateway gsm-gateway

ENTRYPOINT ["/app/gsm-gateway"]