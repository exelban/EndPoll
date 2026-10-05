# EndPoll

<a href="https://github.com/exelban/EndPoll"><p align="center"><img src="https://github.com/exelban/EndPoll/raw/master/templates/static/icon.png" width="120"></p></a>

[![EndPoll](https://serhiy.s3.eu-central-1.amazonaws.com/Github_repo/JAM/cover.png)](https://github.com/exelban/EndPoll)

EndPoll is a lightweight, self-hosted status page and monitoring tool. It periodically checks the health of your services and displays their status on a clean, minimalistic web dashboard with 90 days of history.

## Features

- **Multiprotocol monitoring** — HTTP/HTTPS, TCP, DNS, ICMP (ping), MongoDB, Redis, PostgreSQL and MySQL
- **90-day history** with automatic daily aggregation
- **Incident tracking** — records downtime events with duration and status codes
- **Host grouping** — organize hosts into named groups with optional hidden members
- **Notifications** — Slack, Telegram, SMTP (email), Discord, Microsoft Teams, Mattermost/Rocket.Chat, Pushover, ntfy, Twilio SMS, BotComm and generic webhooks
- **Hot reload** — config file changes are picked up automatically without restart
- **Response time charts** — per-host response time graph rendered as PNG
- **JSON API and Prometheus metrics** — integrate with dashboards, Grafana and alerting
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

The `--smtp.*` flags are a fallback: they are used only when the configuration file does not define `notifications.smtp`.

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
  hideURL: false            # hide host URLs from the dashboard (hosts without a name are shown by their id)
  basicAuth:                # protect the dashboard with basic authentication (optional)
    username: "admin"
    password: "secret"
```

When `basicAuth` is set, all pages require credentials. The `/ping` health check endpoint remains unauthenticated.

#### Connectivity check

For self-hosted setups (e.g. running at home), a drop in your own internet connection would otherwise make every external host appear DOWN at once. Before counting a failed check as an outage, EndPoll verifies that the monitor itself is online by opening a TCP connection to well-known public endpoints. If the monitor has no connectivity, the failed check is skipped: the host keeps its last known status, no failure is counted and no alert is sent. Normal checking resumes automatically once connectivity is restored.

```yaml
connectivity:
  disabled: false                       # disable the connectivity check (enabled by default)
  targets:                              # endpoints probed over TCP (default: 1.1.1.1:53, 8.8.8.8:53)
    - "1.1.1.1:53"
    - "8.8.8.8:53"
  interval: 5s                          # how long a connectivity result is cached (default: 5s)
  timeout: 2s                           # timeout for a single probe (default: 2s)
```

#### Notifications

```yaml
notifications:
  # Send a message when EndPoll starts (default: true)
  initializationMessage: true
  # Send a message when EndPoll shuts down (default: false)
  shutdownMessage: false
  # Minimal time between two identical notifications (same host, same status,
  # same channel). Prevents a flapping host from spamming the channels.
  # Default: 5m, set to 0 to disable.
  cooldown: 5m

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
    insecureSkipVerify: false  # skip TLS certificate verification (default: false)

  # generic HTTP webhook: receives a JSON document per event
  webhook:
    url: "https://example.com/hooks/endpoll"
    method: POST                  # optional, default: POST
    headers:                      # optional
      Authorization: "Bearer token"

  discord:
    webhookURL: "https://discord.com/api/webhooks/..."
    username: "EndPoll"           # optional

  teams:
    webhookURL: "https://prod-00.westus.logic.azure.com:443/workflows/..."

  # Slack-compatible incoming webhook (Mattermost, Rocket.Chat, ...)
  mattermost:
    webhookURL: "https://mattermost.example.com/hooks/..."
    channel: "monitoring"         # optional
    username: "EndPoll"           # optional

  pushover:
    token: "application token"
    user: "user or group key"
    priority: 0                   # optional, -2..2 (2 = emergency, repeats until acknowledged)

  ntfy:
    url: "https://ntfy.sh"        # optional, self-hosted server url
    topic: "endpoll"
    token: "tk_..."               # optional, access token
    priority: 3                   # optional, 1..5

  twilio:
    accountSID: "AC..."
    authToken: "..."
    from: "+15005550006"          # sender number or messaging service sid
    to:
      - "+15005550001"

  botcomm:
    clientID: "your-bot-client-id"
    clientSecret: "your-bot-client-secret"
    sessionIDs:                   # optional; omit or use [] to broadcast to everyone connected to the bot
      - "YOUR_SESSION_ID"
```

Channel names for the per-host `alerts` list: `slack`, `telegram`, `smtp`, `webhook`, `discord`, `teams`, `mattermost`, `pushover`, `ntfy`, `twilio`, `botcomm`.

BotComm sends messages to `https://api.botcomm.app/message` using HTTP Basic authentication. Create a bot in BotComm to get its client ID and client secret, and copy recipient Session IDs from Chat Settings on web or iOS. Keep the secret in your server-side configuration. Empty or whitespace-only Session IDs are rejected. With no Session IDs, messages are broadcast to everyone connected to the bot, including the owner; a `202` response means the broadcast was queued, not delivered. Broadcasts require a backend supporting Basic-auth broadcasts and are limited to one per minute and 24 per day per bot. Sending stops at the first error, so earlier recipients may already have received the message; EndPoll does not automatically retry BotComm requests.

The webhook payload of a status change:

```json
{
  "id": "j_3vve.down.1725000000000000000",
  "type": "host",
  "subject": "❌ Google is DOWN",
  "status": "down",
  "host": {"id": "j_3vve", "name": "Google", "url": "https://www.google.com", "group": "Search"},
  "timestamp": "2025-08-30T10:00:00Z"
}
```

The startup/shutdown messages have `"type": "system"` and a `"message"` field instead of `status`/`host`.

Notifications are sent when a host changes its status (after the configured thresholds). A notification that failed to be delivered to one channel does not prevent the delivery to the other channels. The startup/shutdown messages are sent once per process, a configuration reload does not repeat them.

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
| TCP | `tcp://host:port` | `tcp://db.internal:5432` |
| DNS | `dns://name` | `dns://example.com` |
| Redis | `redis://` or `rediss://` (TLS) | `redis://:password@cache.internal:6379/0` |
| PostgreSQL | `postgres://` or `postgresql://` | `postgres://user:pass@db.internal:5432/app?sslmode=disable` |
| MySQL / MariaDB | `mysql://` | `mysql://user:pass@db.internal:3306/app` |

TCP opens a connection and closes it. Redis sends `AUTH` (when credentials are given), `SELECT` (when a database is given) and `PING`. PostgreSQL and MySQL connect and run `SELECT 1`. Passwords are masked everywhere they are displayed (UI, API, logs, notifications).

DNS resolves the name and, optionally, checks that the expected records are present:

```yaml
hosts:
  - name: "Website DNS"
    url: "dns://example.com"
    dns:
      type: A                       # A, AAAA, CNAME, MX, NS, TXT (default: A)
      expect: ["93.184.216.34"]     # every listed value must be in the answer (optional)
      resolver: "1.1.1.1:53"        # dns server to query (default: system resolver)
```

ICMP checks on Linux use raw sockets, which require the `CAP_NET_RAW` capability (granted by default to Docker containers). On macOS no special permissions are needed. `mongodb+srv://` URLs are detected as MongoDB as well.

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
| `GET` | `/api/hosts` | JSON: overall status and the state of every host |
| `GET` | `/api/hosts/{id}` | JSON: host details, 30-day stats, history and incidents |
| `GET` | `/api/hosts/{id}/response-time` | JSON: average response time per day |
| `GET` | `/api/incidents` | JSON: recent incidents of every host, newest first |
| `GET` | `/metrics` | Prometheus metrics |
| `GET` | `/ping` | Health check (returns `ok`) |

All endpoints except `/ping` are protected by `ui.basicAuth` when it is configured.

### JSON API

```bash
curl -s http://localhost:8822/api/hosts | jq
```

```json
{
  "status": "up",
  "hosts": [
    {
      "id": "j_3vve", "name": "Google", "url": "https://www.google.com", "group": "Search",
      "type": "http", "status": "up", "uptime": 100,
      "lastCheck": "2025-08-30T10:00:00Z",
      "last": {"code": 200, "responseTimeMs": 87, "sslExpiry": "2025-11-01T00:00:00Z", "sslIssuer": "Google Trust Services", "tlsVersion": "TLS 1.3"}
    }
  ]
}
```

`/api/hosts/{id}` adds `details` (30-day uptime and response time, SSL, last outage), `history` (the last 90 checks) and `incidents`.

### Prometheus metrics

`/metrics` exposes one sample per host with the labels `id`, `name`, `group` and `type`:

| Metric | Description |
|--------|-------------|
| `endpoll_host_up` | `1` up, `0` down/degraded (absent while unknown) |
| `endpoll_host_status{status="up\|degraded\|down\|unknown"}` | `1` for the current status |
| `endpoll_host_response_seconds` | duration of the last check |
| `endpoll_host_response_code` | status code of the last check (HTTP code or `521`/`522`/`523`) |
| `endpoll_host_last_check_timestamp_seconds` | unix time of the last check |
| `endpoll_host_ssl_expiry_timestamp_seconds` | unix time of the certificate expiration (HTTPS hosts) |
| `endpoll_host_uptime_ratio` | uptime of the last 90 days (0..1) |
| `endpoll_build_info{version}` | build information |

Example alert rule:

```yaml
- alert: HostDown
  expr: endpoll_host_up == 0
  for: 5m
  labels: {severity: critical}
  annotations: {summary: "{{ $labels.name }} is down"}
- alert: CertificateExpiresSoon
  expr: (endpoll_host_ssl_expiry_timestamp_seconds - time()) / 86400 < 14
  labels: {severity: warning}
```

When `basicAuth` is enabled, configure `basic_auth` in the Prometheus scrape job.

## License

[MIT License](https://github.com/exelban/EndPoll/blob/master/LICENSE)
