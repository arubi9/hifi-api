## Stage 1: Build Go TUI binary
FROM golang:1.23-alpine AS go-builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -o /hifi-tui ./cmd/hifi-tui

## Stage 2: Python API + Go TUI
FROM python:3.13.10-slim

RUN apt-get update && apt-get install -y --no-install-recommends ffmpeg && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --upgrade pip && \
    pip install --no-cache-dir -r requirements.txt

RUN addgroup --system app && adduser --system --ingroup app app

COPY main.py .
COPY entrypoint.sh /entrypoint.sh
COPY --from=go-builder /hifi-tui /usr/local/bin/hifi-tui

RUN chmod +x /entrypoint.sh /usr/local/bin/hifi-tui && chown -R app:app /app

USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["python", "-c", "import urllib.request; urllib.request.urlopen('http://localhost:8000/health')"]

ENTRYPOINT ["/entrypoint.sh"]
CMD ["tui"]
