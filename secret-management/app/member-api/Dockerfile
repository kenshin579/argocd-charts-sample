FROM golang:1.26-alpine3.24 AS builder
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -o /out/member-api ./cmd/member-api

FROM alpine:3.24
RUN adduser -D -u 10001 app
COPY --from=builder /out/member-api /app/member-api
USER 10001
EXPOSE 8080
ENTRYPOINT ["/app/member-api"]
CMD ["serve"]
