FROM golang:1.10.1-alpine3.7 AS go-build


ADD ./main.go /src/main.go

RUN cd /src && go build -o /src/prom-proxy

FROM alpine:3.7

RUN apk add --no-cache ca-certificates

COPY --from=go-build /src/prom-proxy /app/

EXPOSE 8080

ENTRYPOINT ["/app/prom-proxy"]
