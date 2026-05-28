FROM golang:1.23-alpine AS dev-base

WORKDIR /app

RUN apk add --no-cache git gcc musl-dev

ENV GOPROXY=https://goproxy.cn,direct

RUN go install github.com/air-verse/air@latest
ENV PATH=$PATH:/go/bin

COPY go.mod go.sum ./
RUN go mod download && go mod tidy

COPY . .

CMD ["air"]
