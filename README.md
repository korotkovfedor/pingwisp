# Pingwisp

Pingwisp is a small HTTP endpoint monitor written in Go. It periodically checks
registered URLs and exposes their latest status, HTTP response code, latency,
and errors through a JSON API.

## Requirements

- Go **1.27.1 or newer**.

## Build

From the project root:

```sh
go build -o bin/pingwisp .
```

## Run

```sh
./bin/pingwisp
./bin/pingwisp --listen 127.0.0.1:9000
./bin/pingwisp --help
```

The default address is `127.0.0.1:8080`. Use `--listen` to change the address;
`:8080` listens on all network interfaces. Stop the server with `Ctrl+C`.

To run directly from source, use `go run .` with the same flags.

## API

Create a target and read the latest results:

```sh
curl -X POST http://127.0.0.1:8080/targets \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://example.com","interval_seconds":30}'

curl http://127.0.0.1:8080/targets
```

See the [HTTP API contract](docs/api.md) for endpoints, request and response
examples, limits, and error responses.

Targets and results are stored in memory and are lost when the server restarts.

## License

[MIT](LICENSE).
