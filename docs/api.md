# HTTP API contract

This document describes the current implementation.

The default base URL is `http://127.0.0.1:8080`; the address can be changed with
`--listen`.

## Conventions

- Send JSON request bodies with `Content-Type: application/json`.
- Responses with a JSON body use `Content-Type: application/json`.
- Field names use `snake_case`.
- Target IDs are unsigned 64-bit integers, represented as JSON numbers.
  IDs start at `1` in each new server process.
- Durations are integers; field names specify seconds or milliseconds.
- Timestamps use RFC 3339 in UTC (`Z`).
- Nullable response fields are always present and contain `null` when unset.
- No query parameters are defined; query parameters are currently ignored.
- `HEAD` is supported for the GET routes, with the same status and headers but
  no response body.

## Target representation

POST and GET responses use the same target representation. List responses
contain objects with these same fields.

```json
{
  "id": 1,
  "url": "https://example.com/health",
  "interval_seconds": 30,
  "status": "up",
  "last_checked_at": "2026-10-04T02:10:32Z",
  "status_code": 200,
  "latency_ms": 143,
  "error": null,
  "next_check_at": "2026-10-04T02:11:02Z"
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | integer | Target ID. |
| `url` | string | URL submitted when the target was created. |
| `interval_seconds` | integer | Delay between completion of a check and the next check. |
| `status` | string | `pending`, `up`, or `down`. |
| `last_checked_at` | string / null | Completion time of the latest check. |
| `status_code` | integer / null | Latest HTTP response code, if a response was received. |
| `latency_ms` | integer / null | Duration of the latest check in milliseconds. |
| `error` | string / null | Latest check error, if any. |
| `next_check_at` | string / null | Scheduled time of the next check; `null` while a check is running. |

Before the first check completes, `status` is `pending` and all latest-result
fields are `null`. During later checks, the previous result remains visible.
Settings, the latest result, and the schedule are returned as a consistent
snapshot for each target.

## POST /targets

Creates an independent monitoring target. Submitting the same URL again creates
another target with a new ID.

### Request

```http
POST /targets
Content-Type: application/json
```

```json
{
  "url": "https://example.com/health",
  "interval_seconds": 30
}
```

| Field | Type | Required | Current constraints |
| --- | --- | --- | --- |
| `url` | string | Yes | Must be nonempty. Use an absolute HTTP or HTTPS URL; URL structure is not yet validated at creation. |
| `interval_seconds` | integer | Yes | Between `1` and `86400`, inclusive. |

The entire request body, including whitespace and any trailing content, must
not exceed **16 KiB (16384 bytes)**. Missing or `null` required fields fail
validation. Invalid field types are rejected.

### Responses

`201 Created` includes `Location: /targets/1` and the initial target snapshot:

```json
{
  "id": 1,
  "url": "https://example.com/health",
  "interval_seconds": 30,
  "status": "pending",
  "last_checked_at": null,
  "status_code": null,
  "latency_ms": null,
  "error": null,
  "next_check_at": "2026-10-04T02:10:00Z"
}
```

The first check is scheduled immediately. POST always returns the creation
snapshot with `pending`; a subsequent GET may already show a completed check.

| HTTP status | Response | Meaning |
| --- | --- | --- |
| `201 Created` | Target object | Target created. |
| `400 Bad Request` | `invalid_json` error | Empty, unreadable, malformed, or incompatible JSON body. |
| `400 Bad Request` | `invalid_request` error | Missing or invalid required field. |
| `413 Request Entity Too Large` | `request_too_large` error | Request body exceeds 16 KiB. |
| `500 Internal Server Error` | `internal_error` error | Response could not be encoded. |

## GET /targets

Returns all current targets, ordered by ascending ID.

### Request

```http
GET /targets
```

No request body is needed. There is no pagination or filtering.

### Responses

`200 OK` returns a `targets` array. Each element uses the
[target representation](#target-representation):

```json
{
  "targets": [
    {
      "id": 1,
      "url": "https://example.com/health",
      "interval_seconds": 30,
      "status": "up",
      "last_checked_at": "2026-10-04T02:10:32Z",
      "status_code": 200,
      "latency_ms": 143,
      "error": null,
      "next_check_at": "2026-10-04T02:11:02Z"
    }
  ]
}
```

An empty list is `[]`, not `null`:

```json
{
  "targets": []
}
```

| HTTP status | Response | Meaning |
| --- | --- | --- |
| `200 OK` | Object with a `targets` array | Current targets, including an empty list. |
| `500 Internal Server Error` | `internal_error` error | Response could not be encoded. |

## GET /targets/{id}

Returns the current state of one target.

### Request

```http
GET /targets/1
```

No request body is needed. `id` must contain decimal digits and fit in an
unsigned 64-bit integer (`0` through `18446744073709551615`). ID `0` is valid
syntax but has no corresponding target in normal use.

### Responses

`200 OK` returns a target object, as shown under
[Target representation](#target-representation). For example, a check that
received HTTP `503` produces this target snapshot:

```json
{
  "id": 1,
  "url": "https://example.com/health",
  "interval_seconds": 30,
  "status": "down",
  "last_checked_at": "2026-10-04T02:10:32Z",
  "status_code": 503,
  "latency_ms": 82,
  "error": null,
  "next_check_at": "2026-10-04T02:11:02Z"
}
```

| HTTP status | Response | Meaning |
| --- | --- | --- |
| `200 OK` | Target object | Target found. |
| `400 Bad Request` | `invalid_id` error | ID has invalid syntax or is outside the unsigned 64-bit range. |
| `404 Not Found` | `target_not_found` error | Target does not exist or has been deleted. |
| `500 Internal Server Error` | `internal_error` error | Response could not be encoded. |

A target with `status: "down"` still produces `200 OK`: the API request succeeded
even though the monitored endpoint failed its latest check.

## DELETE /targets/{id}

Removes a target and cancels its scheduled and active checks.

### Request

```http
DELETE /targets/1
```

No request body is needed. ID rules match
[GET /targets/{id}](#get-targetsid).

### Responses

```http
HTTP/1.1 204 No Content
```

The successful response has no body and no `Content-Type` header.

| HTTP status | Response | Meaning |
| --- | --- | --- |
| `204 No Content` | Empty body | Target removed. |
| `400 Bad Request` | `invalid_id` error | Invalid or out-of-range ID. |
| `404 Not Found` | `target_not_found` error | Target does not exist; repeated deletion also returns this response. |

After successful deletion, GET cannot find the target and no further checks
are scheduled. An active local check receives a cancellation signal and its
result is discarded. DELETE does not wait for that check to finish or release
its resources. Cancellation cannot guarantee that a remote server stops work
it has already started.

## Error responses

Documented API errors use this JSON envelope:

```json
{
  "error": {
    "code": "invalid_request",
    "message": "interval_seconds must be between 1 and 86400",
    "field": "interval_seconds"
  }
}
```

`field` is included only for errors associated with a particular field.
`code` is intended for programmatic handling. `message` is human-readable and
may change. Internal encoding errors return a generic message and log details.

| HTTP status | `error.code` | Meaning |
| --- | --- | --- |
| `400` | `invalid_json` | Request body cannot be read or decoded as the expected JSON object. |
| `400` | `invalid_request` | Required field is missing or has an invalid value or type. |
| `400` | `invalid_id` | Invalid target ID in the path. |
| `404` | `target_not_found` | Target does not exist. |
| `404` | `route_not_found` | Unknown API route. |
| `405` | `method_not_allowed` | Unsupported method for a known route. |
| `413` | `request_too_large` | POST body exceeds 16 KiB. |
| `500` | `internal_error` | Response could not be encoded. |

`405` responses include `Allow: GET, HEAD, POST` for `/targets`, or
`Allow: DELETE, GET, HEAD` for `/targets/{id}`.

The `error` field in a target object describes a monitoring check. The API error
envelope describes a failed API operation.

## Check behavior and limits

- Checks use HTTP GET. Checks for one target never overlap.
- Each check has a **3-second timeout**, including reading the response body.
- At most **1 MiB (1048576 bytes)** of response body is accepted. Exceeding this
  limit marks the target `down` and sets `error`.
- A completed request with a `2xx` status is `up`. Other status codes are `down`
  without an error message unless the request or body read also fails.
- Network errors, timeouts, and response read errors produce `down` with an
  error message. If no HTTP response was received, `status_code` is `null`.
  If headers were received before an error, the response code is preserved.
- Latency includes connection establishment, the request, and reading the body.
- Redirects are followed. The final response code is recorded; excessive
  redirects fail the check.
- The next check is scheduled at completion time plus `interval_seconds`.
  Scheduling does not guarantee an exact start time under load.
- Deletion and application shutdown do not create a new `down` result.

Storage is in memory in a single process. Restarting the server removes all
targets, results, and schedules; IDs may be reused after restart. No target-count
limit or global check-concurrency limit is currently enforced. The API has no
authentication, and target network addresses are not restricted.
