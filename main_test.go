package main

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestAPI(t *testing.T) {
	s, e := openServer(t.TempDir(), "public")
	if e != nil {
		t.Fatal(e)
	}
	defer s.db.Close()
	h := s.handler()
	req := func(method, path, body string, status int) map[string]any {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, status, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	req("GET", "/api/health", "", 200)
	student := req("POST", "/api/students", `{"first_name":"Ada","last_name":"Lovelace","age":21,"national_id":"N123"}`, 201)
	sid := student["id"].(string)
	req("POST", "/api/students", `{"first_name":"A","last_name":"B","age":true,"national_id":"N2"}`, 400)
	req("POST", "/api/students", `{"first_name":"A","last_name":"B","age":22,"national_id":"N123"}`, 409)
	req("GET", "/api/students/missing", "", 404)
	regBody := `{"student_id":"` + sid + `","course":"CS","semester":"Fall 2026","fee_cents":10000}`
	reg := req("POST", "/api/registrations", regBody, 201)
	rid := reg["id"].(string)
	req("POST", "/api/registrations", regBody, 409)
	req("POST", "/api/registrations", `{"student_id":"missing","course":"CS","semester":"Fall","fee_cents":100}`, 409)
	pay := func(amount, ref string) string {
		return `{"registration_id":"` + rid + `","amount_cents":` + amount + `,"method":"cash","reference":"` + ref + `"}`
	}
	req("POST", "/api/payments", pay("6000", "R1"), 201)
	req("POST", "/api/payments", pay("100", "R1"), 409)
	req("POST", "/api/payments", pay("5000", "R2"), 400)
	req("POST", "/api/payments", pay("4000", "R2"), 201)
	detail := req("GET", "/api/students/"+sid, "", 200)
	registrations := detail["registrations"].([]any)
	if registrations[0].(map[string]any)["paid_cents"] != float64(10000) {
		t.Fatal("incorrect balance")
	}
	for _, path := range []string{"/api/students?q=Ada", "/api/registrations", "/api/payments"} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		var list []any
		if e = json.Unmarshal(w.Body.Bytes(), &list); e != nil || len(list) == 0 {
			t.Fatalf("collection %s: %s", path, w.Body.String())
		}
	}
	upload := func(data []byte, status int) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		f, _ := mw.CreateFormFile("photo", "../../photo.png")
		f.Write(data)
		mw.Close()
		r := httptest.NewRequest("POST", "/api/students/"+sid+"/photo", &buf)
		r.Header.Set("Content-Type", mw.FormDataContentType())
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("photo got %d: %s", w.Code, w.Body.String())
		}
		var out map[string]any
		json.Unmarshal(w.Body.Bytes(), &out)
		return out
	}
	png := []byte("\x89PNG\r\n\x1a\nfixture")
	photo := upload(png, 201)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", photo["photo_url"].(string), nil))
	if !bytes.Equal(w.Body.Bytes(), png) {
		t.Fatal("photo retrieval")
	}
	upload([]byte("<script>"), 400)
	upload(bytes.Repeat([]byte("x"), 5300000), 413)
	s.db.Close()
	s2, e := openServer(s.data, "public")
	if e != nil {
		t.Fatal(e)
	}
	defer s2.db.Close()
	var count int
	s2.db.QueryRow("SELECT COUNT(*) FROM students").Scan(&count)
	if count != 1 {
		t.Fatal("persistence")
	}
}
func TestConcurrentPayments(t *testing.T) {
	s, e := openServer(t.TempDir(), "public")
	if e != nil {
		t.Fatal(e)
	}
	defer s.db.Close()
	s.db.Exec("INSERT INTO students VALUES('s','A','B',20,'N','','',NULL,'now')")
	s.db.Exec("INSERT INTO registrations VALUES('r','s','CS','Fall','active',100,'now')")
	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for _, ref := range []string{"P1", "P2"} {
		wg.Add(1)
		go func(ref string) {
			defer wg.Done()
			w := httptest.NewRecorder()
			r := httptest.NewRequest("POST", "/api/payments", strings.NewReader(`{"registration_id":"r","amount_cents":100,"method":"cash","reference":"`+ref+`"}`))
			s.handler().ServeHTTP(w, r)
			statuses <- w.Code
		}(ref)
	}
	wg.Wait()
	close(statuses)
	counts := map[int]int{}
	for status := range statuses {
		counts[status]++
	}
	if counts[201] != 1 || counts[400] != 1 {
		t.Fatalf("concurrent statuses %v", counts)
	}
}
