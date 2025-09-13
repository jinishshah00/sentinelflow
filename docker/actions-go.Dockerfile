# syntax=docker/dockerfile:1
FROM golang:1.24-alpine AS build
WORKDIR /src
ENV CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOTOOLCHAIN=auto
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -ldflags="-s -w" -o /out/server ./services/actions-go/cmd/server

FROM gcr.io/distroless/base-debian12:nonroot
ENV PORT=8080
COPY --from=build /out/server /server
USER nonroot:nonroot
ENTRYPOINT ["/server"]
