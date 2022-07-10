FROM golang:alpine
RUN apk add build-base

RUN mkdir /app

ENV GIN_MODE=production

WORKDIR /app

COPY ./ /app/
RUN ls -la /app/*

RUN go build

EXPOSE 8080

CMD ["./bettit"]
