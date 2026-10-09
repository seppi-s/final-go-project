package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	_ "github.com/mattn/go-sqlite3"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	db     *sql.DB
	data   string
	public string
}
type Student struct {
	ID       string  `json:"id"`
	First    string  `json:"first_name"`
	Last     string  `json:"last_name"`
	Age      int     `json:"age"`
	National string  `json:"national_id"`
	Email    string  `json:"email"`
	Phone    string  `json:"phone"`
	Photo    *string `json:"photo_url"`
	Created  string  `json:"created_at"`
}
type Registration struct {
	ID       string `json:"id"`
	Student  string `json:"student_id"`
	Course   string `json:"course"`
	Semester string `json:"semester"`
	Status   string `json:"status"`
	Fee      int64  `json:"fee_cents"`
	Created  string `json:"created_at"`
	Paid     int64  `json:"paid_cents"`
}
type Payment struct {
	ID           string `json:"id"`
	Registration string `json:"registration_id"`
	Amount       int64  `json:"amount_cents"`
	Method       string `json:"method"`
	Reference    string `json:"reference"`
	Created      string `json:"created_at"`
}

func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[:4], b[4:6], b[6:8], b[8:10], b[10:])
}
func now() string { return time.Now().UTC().Format(time.RFC3339Nano) }
func openServer(data, public string) (*Server, error) {
	if e := os.MkdirAll(filepath.Join(data, "photos"), 0750); e != nil {
		return nil, e
	}
	d, e := sql.Open("sqlite3", filepath.Join(data, "students.db"))
	if e != nil {
		return nil, e
	}
	d.SetMaxOpenConns(1)
	_, e = d.Exec(`PRAGMA foreign_keys=ON; PRAGMA busy_timeout=20000; PRAGMA journal_mode=WAL;
 CREATE TABLE IF NOT EXISTS students(id TEXT PRIMARY KEY, first_name TEXT NOT NULL,last_name TEXT NOT NULL,age INTEGER NOT NULL CHECK(age BETWEEN 1 AND 120),national_id TEXT NOT NULL UNIQUE,email TEXT NOT NULL,phone TEXT NOT NULL,photo_url TEXT,created_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS registrations(id TEXT PRIMARY KEY,student_id TEXT NOT NULL REFERENCES students(id),course TEXT NOT NULL,semester TEXT NOT NULL,status TEXT NOT NULL,fee_cents INTEGER NOT NULL CHECK(fee_cents>=0),created_at TEXT NOT NULL,UNIQUE(student_id,course,semester));
 CREATE TABLE IF NOT EXISTS payments(id TEXT PRIMARY KEY,registration_id TEXT NOT NULL REFERENCES registrations(id),amount_cents INTEGER NOT NULL CHECK(amount_cents>0),method TEXT NOT NULL,reference TEXT NOT NULL UNIQUE,created_at TEXT NOT NULL);`)
	if e != nil {
		d.Close()
		return nil, e
	}
	return &Server{d, data, public}, nil
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	reply(w, status, map[string]string{"error": msg})
}
func dbError(w http.ResponseWriter, e error) {
	if strings.Contains(e.Error(), "constraint failed") {
		fail(w, 409, "Duplicate record or invalid related record.")
		return
	}
	log.Printf("database error: %v", e)
	fail(w, 500, "Internal server error.")
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024)
	d := json.NewDecoder(r.Body)
	if e := d.Decode(v); e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			fail(w, 413, "Request too large.")
		} else {
			fail(w, 400, "Send a valid JSON object with integer numeric fields.")
		}
		return false
	}
	if d.Decode(new(any)) != io.EOF {
		fail(w, 400, "Send a single JSON object.")
		return false
	}
	return true
}
func valid(s string, max int) bool { return strings.TrimSpace(s) != "" && len([]rune(s)) <= max }
func scanStudent(row interface{ Scan(...any) error }) (Student, error) {
	var s Student
	e := row.Scan(&s.ID, &s.First, &s.Last, &s.Age, &s.National, &s.Email, &s.Phone, &s.Photo, &s.Created)
	return s, e
}
func (s *Server) students(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		q := "%" + r.URL.Query().Get("q") + "%"
		rows, e := s.db.Query("SELECT * FROM students WHERE first_name LIKE ? OR last_name LIKE ? OR national_id LIKE ? ORDER BY created_at DESC LIMIT 500", q, q, q)
		if e != nil {
			dbError(w, e)
			return
		}
		defer rows.Close()
		list := []Student{}
		for rows.Next() {
			v, e := scanStudent(rows)
			if e != nil {
				dbError(w, e)
				return
			}
			list = append(list, v)
		}
		if e = rows.Err(); e != nil {
			dbError(w, e)
			return
		}
		reply(w, 200, list)
		return
	}
	var v Student
	if !decode(w, r, &v) {
		return
	}
	v.First = strings.TrimSpace(v.First)
	v.Last = strings.TrimSpace(v.Last)
	v.National = strings.TrimSpace(v.National)
	if !valid(v.First, 120) || !valid(v.Last, 120) || !valid(v.National, 40) || v.Age < 1 || v.Age > 120 || len(v.Email) > 200 || len(v.Phone) > 40 {
		fail(w, 400, "Valid first_name, last_name, national_id and age (1–120) are required.")
		return
	}
	v.ID = id()
	v.Created = now()
	v.Photo = nil
	_, e := s.db.Exec("INSERT INTO students VALUES(?,?,?,?,?,?,?,?,?)", v.ID, v.First, v.Last, v.Age, v.National, v.Email, v.Phone, nil, v.Created)
	if e != nil {
		dbError(w, e)
		return
	}
	reply(w, 201, v)
}
func (s *Server) registrationList(student string) ([]Registration, error) {
	q := `SELECT r.*,COALESCE((SELECT SUM(amount_cents) FROM payments WHERE registration_id=r.id),0) FROM registrations r`
	args := []any{}
	if student != "" {
		q += " WHERE student_id=?"
		args = append(args, student)
	}
	q += " ORDER BY created_at DESC LIMIT 500"
	rows, e := s.db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Registration{}
	for rows.Next() {
		var v Registration
		if e = rows.Scan(&v.ID, &v.Student, &v.Course, &v.Semester, &v.Status, &v.Fee, &v.Created, &v.Paid); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Server) paymentList(student string) ([]Payment, error) {
	q := "SELECT p.* FROM payments p"
	args := []any{}
	if student != "" {
		q += " JOIN registrations r ON r.id=p.registration_id WHERE r.student_id=?"
		args = append(args, student)
	}
	q += " ORDER BY p.created_at DESC LIMIT 500"
	rows, e := s.db.Query(q, args...)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []Payment{}
	for rows.Next() {
		var v Payment
		if e = rows.Scan(&v.ID, &v.Registration, &v.Amount, &v.Method, &v.Reference, &v.Created); e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Server) student(w http.ResponseWriter, r *http.Request) {
	v, e := scanStudent(s.db.QueryRow("SELECT * FROM students WHERE id=?", r.PathValue("id")))
	if e == sql.ErrNoRows {
		fail(w, 404, "Student not found.")
		return
	}
	if e != nil {
		dbError(w, e)
		return
	}
	reg, e := s.registrationList(v.ID)
	if e != nil {
		dbError(w, e)
		return
	}
	pay, e := s.paymentList(v.ID)
	if e != nil {
		dbError(w, e)
		return
	}
	reply(w, 200, struct {
		Student
		Registrations []Registration `json:"registrations"`
		Payments      []Payment      `json:"payments"`
	}{v, reg, pay})
}
func (s *Server) registrations(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := s.registrationList("")
		if e != nil {
			dbError(w, e)
			return
		}
		reply(w, 200, v)
		return
	}
	var input struct {
		Student  string `json:"student_id"`
		Course   string `json:"course"`
		Semester string `json:"semester"`
		Fee      *int64 `json:"fee_cents"`
	}
	if !decode(w, r, &input) {
		return
	}
	if !valid(input.Student, 120) || !valid(input.Course, 120) || !valid(input.Semester, 120) || input.Fee == nil || *input.Fee < 0 || *input.Fee > 100000000 {
		fail(w, 400, "Valid student_id, course, semester and fee_cents (0–100000000) are required.")
		return
	}
	v := Registration{ID: id(), Student: strings.TrimSpace(input.Student), Course: strings.TrimSpace(input.Course), Semester: strings.TrimSpace(input.Semester), Status: "active", Fee: *input.Fee, Created: now()}
	_, e := s.db.Exec("INSERT INTO registrations VALUES(?,?,?,?,?,?,?)", v.ID, v.Student, v.Course, v.Semester, v.Status, v.Fee, v.Created)
	if e != nil {
		dbError(w, e)
		return
	}
	reply(w, 201, v)
}
func (s *Server) payments(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		v, e := s.paymentList("")
		if e != nil {
			dbError(w, e)
			return
		}
		reply(w, 200, v)
		return
	}
	var v Payment
	if !decode(w, r, &v) {
		return
	}
	v.Registration = strings.TrimSpace(v.Registration)
	v.Reference = strings.TrimSpace(v.Reference)
	if !valid(v.Registration, 120) || !valid(v.Reference, 120) || v.Amount < 1 || v.Amount > 100000000 || (v.Method != "cash" && v.Method != "card" && v.Method != "bank_transfer") {
		fail(w, 400, "Valid registration_id, amount_cents, method and reference are required.")
		return
	}
	// One database connection serializes writers; all balance reads and writes share a transaction.
	tx, e := s.db.Begin()
	if e != nil {
		dbError(w, e)
		return
	}
	defer tx.Rollback()
	var fee, paid int64
	e = tx.QueryRow("SELECT fee_cents FROM registrations WHERE id=?", v.Registration).Scan(&fee)
	if e == sql.ErrNoRows {
		fail(w, 404, "Registration not found.")
		return
	}
	if e != nil {
		dbError(w, e)
		return
	}
	e = tx.QueryRow("SELECT COALESCE(SUM(amount_cents),0) FROM payments WHERE registration_id=?", v.Registration).Scan(&paid)
	if e != nil {
		dbError(w, e)
		return
	}
	if paid+v.Amount > fee {
		fail(w, 400, "Payment exceeds the outstanding balance.")
		return
	}
	v.ID = id()
	v.Created = now()
	_, e = tx.Exec("INSERT INTO payments VALUES(?,?,?,?,?,?)", v.ID, v.Registration, v.Amount, v.Method, v.Reference, v.Created)
	if e != nil {
		dbError(w, e)
		return
	}
	if e = tx.Commit(); e != nil {
		dbError(w, e)
		return
	}
	reply(w, 201, v)
}
func (s *Server) photo(w http.ResponseWriter, r *http.Request) {
	var exists int
	e := s.db.QueryRow("SELECT 1 FROM students WHERE id=?", r.PathValue("id")).Scan(&exists)
	if e == sql.ErrNoRows {
		fail(w, 404, "Student not found.")
		return
	}
	if e != nil {
		dbError(w, e)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 5*1024*1024)
	if e = r.ParseMultipartForm(5 * 1024 * 1024); e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			fail(w, 413, "Photo must be smaller than 5 MB.")
		} else {
			fail(w, 400, "Send a multipart photo file.")
		}
		return
	}
	defer r.MultipartForm.RemoveAll()
	f, _, e := r.FormFile("photo")
	if e != nil {
		fail(w, 400, "Choose a PNG or JPEG photo.")
		return
	}
	defer f.Close()
	data, e := io.ReadAll(f)
	if e != nil {
		fail(w, 400, "Could not read photo.")
		return
	}
	ext := ""
	if len(data) >= 8 && string(data[:8]) == "\x89PNG\r\n\x1a\n" {
		ext = ".png"
	} else if len(data) >= 3 && string(data[:3]) == "\xff\xd8\xff" {
		ext = ".jpg"
	}
	if ext == "" {
		fail(w, 400, "Only PNG and JPEG images are supported.")
		return
	}
	name := id() + ext
	path := filepath.Join(s.data, "photos", name)
	if e = os.WriteFile(path, data, 0640); e != nil {
		log.Printf("photo write: %v", e)
		fail(w, 500, "Could not save photo.")
		return
	}
	url := "/photos/" + name
	_, e = s.db.Exec("UPDATE students SET photo_url=? WHERE id=?", url, r.PathValue("id"))
	if e != nil {
		os.Remove(path)
		dbError(w, e)
		return
	}
	reply(w, 201, map[string]string{"photo_url": url})
}

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) { r.status = status; r.ResponseWriter.WriteHeader(status) }
func (s *Server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		if e := s.db.Ping(); e != nil {
			dbError(w, e)
			return
		}
		reply(w, 200, map[string]string{"status": "ok", "backend": "go"})
	})
	for _, m := range []string{"GET", "POST"} {
		mux.HandleFunc(m+" /api/students", s.students)
		mux.HandleFunc(m+" /api/registrations", s.registrations)
		mux.HandleFunc(m+" /api/payments", s.payments)
	}
	mux.HandleFunc("GET /api/students/{id}", s.student)
	mux.HandleFunc("POST /api/students/{id}/photo", s.photo)
	mux.HandleFunc("GET /api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API endpoint not found.") })
	mux.HandleFunc("POST /api/", func(w http.ResponseWriter, r *http.Request) { fail(w, 404, "API endpoint not found.") })
	mux.Handle("GET /photos/", http.StripPrefix("/photos/", http.FileServer(http.Dir(filepath.Join(s.data, "photos")))))
	mux.Handle("GET /", http.FileServer(http.Dir(s.public)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &recorder{w, 200}
		rw.Header().Set("X-Content-Type-Options", "nosniff")
		rw.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; script-src 'self'; img-src 'self' blob:; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		defer func() {
			if e := recover(); e != nil {
				log.Printf("panic: %v", e)
				fail(rw, 500, "Internal server error.")
			}
			log.Printf("method=%s path=%s status=%d duration=%s", r.Method, r.URL.Path, rw.status, time.Since(start))
		}()
		mux.ServeHTTP(rw, r)
	})
}
func main() {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command == "healthcheck" {
		client := http.Client{Timeout: 2 * time.Second}
		r, e := client.Get("http://127.0.0.1:8080/api/health")
		if e != nil {
			os.Exit(1)
		}
		defer r.Body.Close()
		if r.StatusCode != 200 {
			os.Exit(1)
		}
		return
	}
	data := os.Getenv("DATA_DIR")
	if data == "" {
		data = "data"
	}
	s, e := openServer(data, "public")
	if e != nil {
		log.Fatal(e)
	}
	defer s.db.Close()
	switch command {
	case "serve":
		server := &http.Server{Addr: ":8080", Handler: s.handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
		log.Print("nava-sepehr system listening on :8080")
		log.Fatal(server.ListenAndServe())
	case "summary":
		counts := map[string]int{}
		for _, t := range []string{"students", "registrations", "payments"} {
			var n int
			if e = s.db.QueryRow("SELECT COUNT(*) FROM " + t).Scan(&n); e != nil {
				log.Fatal(e)
			}
			counts[t] = n
		}
		b, _ := json.MarshalIndent(counts, "", "  ")
		fmt.Println(string(b))
	case "list":
		rows, e := s.db.Query("SELECT * FROM students ORDER BY created_at DESC")
		if e != nil {
			log.Fatal(e)
		}
		out := []Student{}
		for rows.Next() {
			v, e := scanStudent(rows)
			if e != nil {
				log.Fatal(e)
			}
			out = append(out, v)
		}
		rows.Close()
		b, _ := json.MarshalIndent(out, "", "  ")
		fmt.Println(string(b))
	default:
		fmt.Fprintln(os.Stderr, "Usage: student-console [serve|list|summary|healthcheck]")
		os.Exit(2)
	}
}
