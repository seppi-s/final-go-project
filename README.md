# nava-sepehr system

A light liquid-glass HTML/CSS/JavaScript interface with a Go REST API, SQLite persistence, and a command-line console. No frontend build tool required.

## Run with Docker

Requires Docker Engine and Docker Compose v2.

```sh
./scripts/start.sh
# Open http://localhost:8080
./scripts/console.sh summary
./scripts/console.sh list
docker compose logs -f web
./scripts/stop.sh
```

The multi-stage Dockerfile builds dependencies in a separate stage and runs a static Go binary as a non-root user. A named volume persists the database and photos when the container is rebuilt or stopped. `docker compose down -v` deletes this data. The Compose port is bound to localhost by default.

## Run without Docker

Requires Go 1.24 or later and a C compiler (GCC/Clang) for the SQLite driver. Docker installs its build tools automatically.

```sh
go mod download
./scripts/test.sh
go build -o student-console .
./student-console serve
```

Open http://localhost:8080. `./student-console list` prints the student collection; `./student-console summary` prints record counts. Set `DATA_DIR` to choose persistent storage (default `./data`). The web server uses Go net/http with request timeouts.

## API

| Method | Path | Purpose |
|---|---|---|
| GET | /api/health | Database health check |
| GET | /api/students?q=Ada | Student collection, optional name/ID filter |
| POST | /api/students | Create student |
| GET | /api/students/{id} | Student with registrations and payments |
| POST | /api/students/{id}/photo | Upload multipart file named `photo` |
| GET / POST | /api/registrations | List / create registrations |
| GET / POST | /api/payments | List / record payments |

Examples:

```sh
curl -X POST http://localhost:8080/api/students \
  -H 'Content-Type: application/json' \
  -d '{"first_name":"Ada","last_name":"Lovelace","age":21,"national_id":"N123","email":"ada@example.com","phone":"123456"}'

curl http://localhost:8080/api/students

curl -X POST http://localhost:8080/api/students/STUDENT_ID/photo \
  -F photo=@portrait.jpg

curl -X POST http://localhost:8080/api/registrations \
  -H 'Content-Type: application/json' \
  -d '{"student_id":"STUDENT_ID","course":"Computer Science","semester":"Fall 2026","fee_cents":10000}'

curl -X POST http://localhost:8080/api/payments \
  -H 'Content-Type: application/json' \
  -d '{"registration_id":"REGISTRATION_ID","amount_cents":5000,"method":"bank_transfer","reference":"BANK-001"}'
```

Replace the IDs with values returned by creation requests. Amounts use integer cents (the interface displays USD). Supported methods: `cash`, `bank_transfer`, `card`. Payments are bookkeeping records, not actual card charges.

## Models and collections

- Student: UUID, name, age, unique national ID, email, phone, photo URL, UTC creation timestamp.
- Registration: UUID, student foreign key, course, semester, status, fee in cents, timestamp. One registration per student/course/semester.
- Payment: UUID, registration foreign key, positive amount in cents, method, unique reference, timestamp.

SQL foreign keys and constraints enforce relationships. Collections are returned as JSON arrays; Go slices and structs assemble nested profiles. GET collections currently return at most 500 records. Payment writes share a transaction and a single database connection to prevent concurrent overpayment within one process. SQLite WAL supports concurrent readers. Structured fields avoid floating-point currency errors and database queries use parameters.

Requests and exceptions are logged to stdout with timestamps, methods, paths and status codes. Request bodies and personal fields are not logged. Photo requests are limited to 5 MiB including multipart overhead; PNG/JPEG signatures are checked and filenames are random. Signature checks do not perform complete image decoding.

## Verification

```sh
./scripts/test.sh
```

The Docker builder stage runs Go tests before compiling:

```sh
docker build --target builder -t nava-sepehr-tests .
```

Tests cover student persistence and validation, duplicates, missing foreign keys, registration collections, payments and balances, uploads, size/type rejection, health, and security headers. See VERIFICATION.md for results from the build environment.

## Deployment scope

This is a local demonstration project without authentication or authorization. Before serving real student personal data remotely, add access control, HTTPS, a backup policy and appropriate privacy safeguards. Photo files are served by URL. There is no payment gateway, refunds, registration cancellation, or background job queue. Use a single persistent volume on one host; multi-host scaling needs a shared database and object storage.

## API test console

The browser includes an API test section with GET/POST methods, endpoint and JSON-body editors, example requests, HTTP status, response timing and formatted JSON output. Requests are limited to same-origin `/api/` paths. POST requests create real records and refresh the dashboard. Replace `STUDENT_ID` or `REGISTRATION_ID` in examples using IDs from GET responses. Photo uploads use the profile form.

## Go backend

The server uses `net/http`, `database/sql`, Go structs and slices, and the pinned `github.com/mattn/go-sqlite3` driver. The SQLite driver uses CGO; only the Docker build stage contains C build tools. The final image contains a compiled executable and static UI assets. Run one server process per database volume; CLI commands can read the same data. Existing databases from the earlier version use the same schema and do not need conversion. `/api/health` identifies the backend as `go`.
