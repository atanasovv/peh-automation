FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /mail-mcp .

FROM alpine:3.23
RUN apk add --no-cache ca-certificates
COPY --from=build /mail-mcp /usr/local/bin/mail-mcp
USER 65532:65532
ENV MCP_TRANSPORT=http MCP_LISTEN_ADDR=:8080
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/mail-mcp"]
