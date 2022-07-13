FROM golang:alpine
RUN apk add build-base

RUN mkdir /app

ENV GIN_MODE=release

WORKDIR /app

COPY ./ /app/

RUN apk add --no-cache curl jq bash

RUN go build

EXPOSE 8080

CMD ["./start_with_token.sh"]
