# Tyk-proxy service

## Getting started
 - Docker and docker compose are required
 - Service uses Redis as a storage and distributed rate limiter
 - service whoami is used for testing as target backend
 - the target backend has to be specified in config

## Makefile commands
 - `make build` - build service
 - `make gen` - generate token and put it to redis
 - `make test` - run tests
 - `make up` - run service, redis and whoami in docker

## Service
Works on 8080 port. If token is valid then service proxies request to target backend. If not is return error status code.
Logs have levels of severity. Debug, Info, Warn, Error, Fatal.
Logs have output format. json or console.
Logs have colored output. Which could be disabled.

Response example:
```shell
Hostname: 23e7dd4157d6
IP: 127.0.0.1
IP: ::1
IP: 192.168.148.3
RemoteAddr: 192.168.148.4:35342
GET /api/v1/test HTTP/1.1
Host: backend-whoami:80
User-Agent: curl/8.7.1
Accept: */*
Accept-Encoding: gzip
Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcGlfa2V5IjoianJUZDhMVHJZZnI5UVJYYThuS3Y0USIsInJhdGVfbGltaXQiOjEwLCJleHBpcmVzX2F0IjoiMjAyNi0wMi0wOVQxNzozODo0NVoiLCJhbGxvd2VkX3JvdXRlcyI6WyIvYXBpL3YxL3Rlc3QiLCIvYXBpL3YxL3Rlc3QyIl0sImV4cCI6MTc3MDY1ODcyNSwiaWF0IjoxNzcwNTcyMzI1fQ.oG5V_YFctAwCA5lwolfItdf9znh9c_dVF6pKFgHEZ1U
X-Forwarded-For: 192.168.148.1
```

## Healthcheck
Service has healthcheck endpoint on `:8080/health` Ok if service is up.
Service has readiness endpoint on `:8080/ready` Ok if service is up and connected to redis.
## Metrics
Service exposes prometheus metrics on `:9090/metrics` endpoint. Prometheus metrics format is used.

## Usage of service
After build you can run service with command (ot just use Make up-b to start all services):
```
Usage of ./tyk_proxy:
  -config string
    	path to config file
  -env
    	override json config values by ENV vars
  -version
    	show version
```

## Config
I prefer to use json config file. But you can use ENV vars. To use then you need to specify flag `-env` on service start.
To use them in Docker you need to update Dockerfile and add ENV vars to docker-compose.yml. Or use .env.example file.

Default configs provided in config.json and .env.example

```json
{
  "application": {
    "target_host": "http://backend-whoami:80",
    "port": 8080,
    "token": {
      "jwt_secret": "II+NZDtODCTp0eAGX0/3HNdaExOf+M1uesFHdN+IFcTD774aaeJrJIOMS4aYhi+l",
      "algorithm": "HS256"
    }
  },
  "server_timeouts": {
    "readHeaderTimeout": "5s",
    "readTimeout": "30s",
    "writeTimeout": "30s",
    "idleTimeout": "60s"
  },
  "redis": {
    "addr": "tyk-redis:6379"
  },
  "log": {
    "level": "Debug",
    "format": "console",
    "colored": true
  },
  "monitoring": {
    "ip": "0.0.0.0",
    "scheme": "http",
    "port": 9090
  }
}

```

```dotenv
export TYK_PROX_LOG__LEVEL=Debug
export TYK_PROX_LOG__FORMAT=console
export TYK_PROX_LOG__COLORED=true

export TYK_PROX_APPLICATION__TARGET_HOST=http://backend-whoami:80
export TYK_PROX_APPLICATION__PORT=8080
export TYK_PROX_APPLICATION__TOKEN__JWT_SECRET=II+NZDtODCTp0eAGX0/3HNdaExOf+M1uesFHdN+IFcTD774aaeJrJIOMS4aYhi+l
export TYK_PROX_APPLICATION__TOKEN__ALGORITHM=HS256

export TYK_PROX_REDIS__ADDR=localhost:6379

export TYK_PROX_SERVER_TIMEOUTS__READHEADERTIMEOUT=5s
export TYK_PROX_SERVER_TIMEOUTS__READTIMEOUT=30s
export TYK_PROX_SERVER_TIMEOUTS__WRITETIMEOUT=30s
export TYK_PROX_SERVER_TIMEOUTS__IDLETIMEOUT=60s

export TYK_PROX_MONITORING__IP=0.0.0.0
export TYK_PROX_MONITORING__SCHEME=http
export TYK_PROX_MONITORING__PORT=9090

```

## Token generation
I have a script that generates tokens for testing purposes. It stores them to redis directly under the same key as the api_key.

All data for token generation is hardcoded in the script.
```
make gen

CGO_ENABLED=0 go build -tags=grpcnotrace -trimpath -ldflags="-s -w -X 'qsp_acb_broker/version.version='" -o token_gen ./cmd/token-gen
./token_gen -secret "II+NZDtODCTp0eAGX0/3HNdaExOf+M1uesFHdN+IFcTD774aaeJrJIOMS4aYhi+l"

Token created successfully!

api_key: uDDxHZBrb2vjCDHi70huEA

storage_key: token:uDDxHZBrb2vjCDHi70huEA
jwt: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcGlfa2V5IjoidUREeEhaQnJiMnZqQ0RIaTcwaHVFQSIsInJhdGVfbGltaXQiOjEwLCJleHBpcmVzX2F0IjoiMjAyNi0wMi0wOVQxNjoxNjoxN1oiLCJhbGxvd2VkX3JvdXRlcyI6WyIvYXBpL3YxL3Rlc3QiLCIvYXBpL3YxL3Rlc3QyIl0sImV4cCI6MTc3MDY1Mzc3NywiaWF0IjoxNzcwNTY3Mzc3fQ.i0JBap25p6JnAYaWrVLNzd6JcULAi--xf9jjh0bRXO0

expires_at: 2026-02-09T16:16:17Z

alowed routes: ["/api/v1/test","/api/v1/test2"]
curl example:

curl -H 'Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhcGlfa2V5IjoidUREeEhaQnJiMnZqQ0RIaTcwaHVFQSIsInJhdGVfbGltaXQiOjEwLCJleHBpcmVzX2F0IjoiMjAyNi0wMi0wOVQxNjoxNjoxN1oiLCJhbGxvd2VkX3JvdXRlcyI6WyIvYXBpL3YxL3Rlc3QiLCIvYXBpL3YxL3Rlc3QyIl0sImV4cCI6MTc3MDY1Mzc3NywiaWF0IjoxNzcwNTY3Mzc3fQ.i0JBap25p6JnAYaWrVLNzd6JcULAi--xf9jjh0bRXO0' http://localhost:8080/api/v1/test
```

Generator takes many arguments.
```
./token_gen -h
Usage of ./token_gen:
  -limit int
    	Rate limit for api_key (default 10)
  -prefix string
    	Redis key prefix (token:<api_key>) (default "token:")
  -redis string
    	Redis address (default "localhost:6379")
  -routes string
    	Comma-separated allowed routes (default "/api/v1/test,/api/v1/test2,")
  -secret string
    	JWT HS256 secret (required)
  -ttl duration
    	Token TTL (default 24h0m0s)
```

## Secret
Script generates tokens with secret. Secret is hardcoded in the script. To generate own secret use command 
```
make secret
```

## Rate limiter

I implemented a **distributed fixed-window rate limiter** backed by Redis.

* **Keying:** each token (`api_key`) gets its own counter key that is scoped to the current time window. The Redis key includes the **window start timestamp** (e.g. `rate_count:<api_key>:<windowStart>`), so all instances naturally agree on the same window.
* **Atomicity / multi-instance correctness:** each request runs a **Lua script** in Redis that does `INCR` and sets `PEXPIRE` only when the counter is created (`count == 1`). This makes the operation atomic and safe under concurrent requests across multiple gateway instances.
* **Expiration:** the counter key is given a TTL roughly equal to the window size (with a small buffer) to prevent stale keys from accumulating.
* **Decision:** after incrementing, the gateway allows the request if `count <= limit`, otherwise it returns `429 Too Many Requests`.

This approach provides a simple, production-friendly baseline for a take-home assignment while demonstrating correct cross-instance synchronization (Redis as the single source of truth for counters).

I considered a more advanced model (e.g., **sliding window**, **token bucket**, or **leaky bucket**) where capacity “refills” smoothly over time (so a request can become available a few seconds later as earlier requests age out). That design is more complex (more state, more logic in Redis/Lua, and more edge cases around clock skew and fairness). For the test assignment I intentionally chose the fixed-window solution to keep it robust, easy to reason about, and straightforward to review.
