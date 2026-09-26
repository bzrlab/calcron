FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /calcron ./cmd/calcron

FROM alpine:3.22
RUN apk add --no-cache tzdata
COPY --from=build /calcron /calcron
EXPOSE 8080
ENTRYPOINT ["/calcron"]
