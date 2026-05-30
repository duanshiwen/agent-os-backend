FROM golang:1.25-bookworm AS dev-base

WORKDIR /app

RUN apt-get update \
    && apt-get install -y --no-install-recommends git gcc ca-certificates \
    && rm -rf /var/lib/apt/lists/*

ENV GOPROXY=https://goproxy.cn,direct

RUN go install github.com/air-verse/air@v1.65.3
ENV PATH=$PATH:/go/bin

COPY go.mod go.sum ./
RUN go mod download && go mod tidy

COPY . .

CMD ["air"]
