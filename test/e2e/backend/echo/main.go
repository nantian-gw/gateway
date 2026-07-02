package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
)

const addr = ":8080"

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/echo/", handleEcho)
	mux.HandleFunc("/headers", handleHeaders)
	mux.HandleFunc("/status/", handleStatus)
	mux.HandleFunc("/body", handleBody)
	mux.HandleFunc("/health", handleHealth)

	log.Printf("echo-server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func headersToMap(h http.Header) map[string]string {
	m := make(map[string]string, len(h))
	for k, vals := range h {
		m[k] = strings.Join(vals, ", ")
	}
	return m
}

func readBody(r *http.Request) string {
	if r.Body == nil {
		return ""
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		return ""
	}
	return string(body)
}

// GET/POST /echo/:path — echo back path, method, headers, and body.
func handleEcho(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/echo")
	if path == "" || path == "/" {
		path = "/"
	}

	resp := map[string]any{
		"path":    path,
		"method":  r.Method,
		"headers": headersToMap(r.Header),
		"body":    readBody(r),
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /headers — return all request headers.
func handleHeaders(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"headers": headersToMap(r.Header),
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /status/:code — return HTTP :code with body {"status": code}.
// Valid codes: 200-599. Anything else returns 400.
func handleStatus(w http.ResponseWriter, r *http.Request) {
	codeStr := strings.TrimPrefix(r.URL.Path, "/status/")
	codeStr = strings.TrimRight(codeStr, "/")

	code, err := strconv.Atoi(codeStr)
	if err != nil || code < 200 || code > 599 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": fmt.Sprintf("invalid status code: %q", codeStr),
		})
		return
	}

	writeJSON(w, code, map[string]any{
		"status": code,
	})
}

// POST /body — echo back request body, length, and headers.
func handleBody(w http.ResponseWriter, r *http.Request) {
	body := readBody(r)
	resp := map[string]any{
		"body":    body,
		"length":  len(body),
		"headers": headersToMap(r.Header),
	}
	writeJSON(w, http.StatusOK, resp)
}

// GET /health — health check.
func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}
