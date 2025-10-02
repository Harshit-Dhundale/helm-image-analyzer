FROM golang:1.23 AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/helm-image-analyzer ./main.go

FROM alpine:3.20
RUN apk add --no-cache ca-certificates bash curl

ARG HELM_VERSION=v3.14.4
RUN curl -fsSL https://get.helm.sh/helm-${HELM_VERSION}-linux-amd64.tar.gz \
  | tar -zx && mv linux-amd64/helm /usr/local/bin/helm && rm -rf linux-amd64
COPY --from=build /out/helm-image-analyzer /usr/local/bin/app
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/app"]
