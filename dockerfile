FROM golang:1.21.3

WORKDIR /app

COPY go.mod .
COPY main.go .
COPY go.sum .
RUN go build -o quoteRet .

ENV SCRAPE_OPS_KEY="7a23f97c-aada-4964-9e1c-cf16b3dfc762"
ENV ADDRESS="0.0.0.0"
ENTRYPOINT [ "/app/quoteRet"]
