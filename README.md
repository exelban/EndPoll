# EndPoll

<a href="https://github.com/exelban/EndPoll"><p align="center"><img src="https://github.com/exelban/EndPoll/raw/master/templates/static/icon.png" width="120"></p></a>

[![EndPoll](https://serhiy.s3.eu-central-1.amazonaws.com/Github_repo/JAM/cover.png)](https://github.com/exelban/EndPoll)

EndPoll is a lightweight, self-hosted status page and monitoring tool. It periodically checks the health of your services and displays their status on a clean, minimalistic web dashboard with 90 days of history.

## Features

- **Multi-protocol monitoring** — HTTP/HTTPS, MongoDB, and ICMP (ping)
- **90-day history** with automatic daily aggregation
- **Incident tracking** — records downtime events with duration and status codes
- **Host grouping** — organize hosts into named groups with optional hidden members
- **Notifications** — Slack, Telegram, and SMTP (email)
- **Hot reload** — config file changes are picked up automatically without restart
- **Response time charts** — per-host response time graph rendered as PNG
- **SSL certificate monitoring** — tracks expiry dates for HTTPS hosts
- **Detailed timing** — DNS, TLS handshake, connect, and TTFB breakdown for HTTP checks
- **Threshold-based status** — configurable success/failure thresholds to prevent flapping
- **Lightweight storage** — embedded BoltDB (default) or in-memory

## Installation

### Docker

```bash
docker run -d \
  -p 8822:8822 \
  -v ./config.yaml:/app/config.yaml \
  -v ./data:/app/data \
  exelban/endpoll:latest
```

Images are available from:
- Docker Hub: [exelban/endpoll](https://hub.docker.com/r/exelban/endpoll)
- GitHub Container Registry: [ghcr.io/exelban/endpoll](https://github.com/users/exelban/packages/container/package/endpoll)

### Docker Compose

```yaml
services:
  endpoll:
    image: exelban/endpoll:latest
    container_name: endpoll
    restart: unless-stopped
    ports:
      - "8822:8822"
    volumes:
      - ./config.yaml:/app/config.yaml
      - ./data:/app/data
    logging:
      driver: "json-file"
      options:
        max-size: "10m"
        max-file: "3"
    healthcheck:
      test: "curl -f http://localhost:8822/ping || exit 1"
      interval: 10s
      timeout: 10s
      retries: 3
      start_period: 3s
```

### Build from source

Requires [Go 1.26+](https://go.dev/doc/install).

```bash
git clone https://github.com/exelban/EndPoll.git
cd EndPoll
go build -o endpoll
./endpoll
```

## Quick start

Create a `config.yaml` file:

```yaml
hosts:
  - name: Google
    url: https://www.google.com
  - name: GitHub
    url: https://github.com
```

Run EndPoll and open [http://localhost:8822](http://localhost:8822).

## Configuration

EndPoll is configured via a YAML or JSON file. The file is watched for changes and reloaded automatically.

### Command-line flags / environment variables

| Flag | Env variable | Default | Description |
|------|-------------|---------|-------------|
| `--config-path` | `CONFIG_PATH` | `./config.yaml` | Path to configuration file |
| `--storage.type` | `STORAGE_TYPE` | `bolt` | Storage backend (`bolt` or `memory`) |
| `--storage.path` | `STORAGE_PATH` | `./data` | Directory for BoltDB storage |
| `--port` | `PORT` | `8822` | HTTP server port |
| `--debug` | `DEBUG` | `false` | Enable debug logging |
| `--smtp.host` | `SMTP_HOST` | | SMTP server host |
| `--smtp.port` | `SMTP_PORT` | `25` | SMTP server port |
| `--smtp.username` | `SMTP_USERNAME` | | SMTP username |
| `--smtp.password` | `SMTP_PASSWORD` | | SMTP password |
| `--smtp.from` | `SMTP_FROM` | | Sender email address |
| `--smtp.to` | `SMTP_TO` | | Recipient email address(es) |

### Config file reference

#### Global settings

```yaml
# Maximum concurrent dial connections (default: 128)
maxConn: 128

# Default check interval for all hosts (default: 30s)
interval: 30s

# Default timeout for all hosts (default: 60s)
timeout: 60s

# Delay before the first check after startup (optional)
initialDelay: 5s

# Consecutive successful checks required to mark a host as UP (default: 1)
successThreshold: 1

# Consecutive failed checks required to mark a host as DOWN (default: 2)
failureThreshold: 2

# Default success conditions applied to all hosts
success:
  code: [200, 201, 202, 203, 204, 205, 206, 207, 208]
  body: "OK"  # optional: expected response body

# Default headers sent with every HTTP request
headers:
  Authorization: "Bearer token"
  User-Agent: "EndPoll"
```

#### UI settings

```yaml
ui:
  title: "My Status Page"  # browser tab title
  hideURL: false            # hide host URLs from the dashboard
```

#### Notifications

```yaml
notifications:
  # Send a message when EndPoll starts (default: true)
  initializationMessage: true
  # Send a message when EndPoll shuts down (default: false)
  shutdownMessage: false

  slack:
    token: "xoxb-your-token"
    channel: "#monitoring"

  telegram:
    token: "123456:ABC-DEF"
    chatIDs:
      - "111111111"
      - "222222222"

  smtp:
    host: "smtp.example.com"
    port: 587
    username: "user@example.com"
    password: "password"
    from: "endpoll@example.com"
    to:
      - "admin@example.com"
```

#### Hosts

```yaml
hosts:
  - name: "Google"                    # display name (optional)
    description: "Search engine"      # shown on the detail page (optional)
    url: "https://www.google.com"     # required
    group: "Search"                   # group name (optional)
    hidden: false                     # hide from group view, only affects grouped hosts (optional)
    method: "GET"                     # HTTP method (default: GET)
    interval: 15s                     # override global interval (optional)
    timeout: 10s                      # override global timeout (optional)
    initialDelay: 2s                  # override global initial delay (optional)
    successThreshold: 1               # override global threshold (optional)
    failureThreshold: 3               # override global threshold (optional)
    conditions:                       # override global success conditions (optional)
      code: [200]
      body: "OK"
    headers:                          # override/extend global headers (optional)
      X-Custom: "value"
    alerts:                           # restrict notifications to specific channels (optional)
      - "slack"
      - "telegram"
```

### Host types

The host type is auto-detected from the URL:

| Type | URL pattern | Example |
|------|------------|---------|
| HTTP/HTTPS | URLs starting with `http://` or `https://` | `https://example.com` |
| MongoDB | URLs starting with `mongodb://` | `mongodb://user:pass@host:27017` |
| ICMP | IPv4 addresses without a scheme | `192.168.1.1` |

You can also set the type explicitly:

```yaml
hosts:
  - url: "192.168.1.1"
    type: icmp
```

## Use cases

### Simple website monitoring

Monitor a set of websites and get notified via Slack when any of them goes down:

```yaml
interval: 60s
failureThreshold: 3

notifications:
  slack:
    token: "xoxb-your-token"
    channel: "#alerts"

hosts:
  - name: "Homepage"
    url: "https://example.com"
  - name: "API"
    url: "https://api.example.com/health"
    conditions:
      code: [200]
      body: "ok"
```

### Grouped microservices

Organize hosts by service group. Hidden hosts contribute to the group's overall status without cluttering the dashboard:

```yaml
hosts:
  - name: "Auth API"
    url: "https://auth.internal/health"
    group: "Auth Service"
  - name: "Auth DB"
    url: "mongodb://user:pass@auth-db:27017"
    group: "Auth Service"
    hidden: true
  - name: "Payments API"
    url: "https://payments.internal/health"
    group: "Payments"
```

### Infrastructure monitoring with ICMP

Monitor network devices alongside web services:

```yaml
hosts:
  - name: "Gateway"
    url: "10.0.0.1"
  - name: "DNS Server"
    url: "10.0.0.53"
  - name: "Dashboard"
    url: "https://grafana.internal"
```

### Multi-channel notifications

Route specific hosts to specific notification channels:

```yaml
notifications:
  slack:
    token: "xoxb-..."
    channel: "#ops"
  telegram:
    token: "123:ABC"
    chatIDs: ["111"]
  smtp:
    host: smtp.example.com
    port: 587
    username: user
    password: pass
    from: alerts@example.com
    to: [oncall@example.com]

hosts:
  - name: "Critical API"
    url: "https://api.example.com"
    alerts: ["slack", "telegram", "smtp"]  # all channels
  - name: "Internal Tool"
    url: "https://tool.internal"
    alerts: ["slack"]                       # slack only
  - name: "Blog"
    url: "https://blog.example.com"
    # no alerts field = all channels
```

## API endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Status dashboard (all hosts) |
| `GET` | `/{id}` | Detail page for a single host |
| `GET` | `/response-time/{id}` | Response time chart (PNG) |
| `GET` | `/ping` | Health check (returns `ok`) |

## License

[MIT License](https://github.com/exelban/EndPoll/blob/master/LICENSE)
