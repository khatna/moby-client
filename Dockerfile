FROM golang:1.20.1-bullseye

WORKDIR /app

COPY . ./

RUN go mod tidy
RUN go build

CMD [ "./moby-client" ]