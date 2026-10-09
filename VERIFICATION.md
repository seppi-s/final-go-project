# Go backend verification — October 9, 2026

- Replaced the Python backend with Go net/http and database/sql using github.com/mattn/go-sqlite3.
- `go test -race ./...` passed: student validation, duplicate IDs, foreign keys, JSON collections, registration uniqueness, payment balance/reference validation, photo upload/retrieval and rejection, persisted records after database reopening, and concurrent payments preventing overpayment.
- Static Go executable built and served the existing SQLite database, retaining student, registration and payment data.
- Live browser: health request returned 200 with `backend: go`; create-student POST returned 201 and refreshed the directory.
- Updated multi-stage Dockerfile: Go builder runs tests and compiles a static executable; non-root Alpine runtime stores data in a persistent volume. Console and test scripts now run Go.
- Docker execution remains unverified because the environment cannot connect to the Docker daemon.

Preview data is stored outside the project and is not published to GitHub.
