FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod main.go index.html ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /iptvproxy .

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /iptvproxy /iptvproxy
EXPOSE 8080
USER 65534:65534
ENTRYPOINT ["/iptvproxy"]
