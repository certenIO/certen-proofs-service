package main

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RB12-OPS-C review D3: the portable-document handler aborts a response it cannot finish by panicking with http.ErrAbortHandler (the
// status is already sent, so the only honest ending is a closed connection). The recover middleware that wraps every handler of the
// service must let that panic through to net/http: it used to log "[panic]", write a 500 status line (ignored) and 49 bytes of JSON
// INTO the middle of the body.

func serveThrough(t *testing.T, logs *bytes.Buffer, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(recoverMiddleware(log.New(logs, "", 0))(h))
	t.Cleanup(srv.Close)
	return srv
}

func TestAnAbortedResponseIsAbortedNotCompletedWithAnErrorBody(t *testing.T) {
	var logs bytes.Buffer
	srv := serveThrough(t, &logs, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "1000000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("x"), 500000))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("the body ended with %v, want an unexpected EOF (the connection aborted)", err)
	}
	if len(body) != 500000 || strings.Trim(string(body), "x") != "" {
		t.Fatalf("the client read %d bytes of which %d are not the document's (junk written into the body)", len(body), len(body)-bytes.Count(body, []byte("x")))
	}
	if strings.Contains(logs.String(), "[panic]") {
		t.Fatalf("an abort is logged as a panic: %q", logs.String())
	}
}

func TestAnAbortedChunkedResponseIsAbortedToo(t *testing.T) {
	var logs bytes.Buffer
	srv := serveThrough(t, &logs, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("y"), 100000))
		w.(http.Flusher).Flush()
		panic(http.ErrAbortHandler)
	})
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err == nil || bytes.Contains(body, []byte("Internal server error")) {
		t.Fatalf("a chunked body that was aborted ended cleanly or carries the error JSON: err %v, %d bytes", err, len(body))
	}
}

// Any other panic still becomes the 500 the middleware exists for.
func TestAnOrdinaryPanicIsStillAnInternalServerError(t *testing.T) {
	var logs bytes.Buffer
	srv := serveThrough(t, &logs, func(w http.ResponseWriter, r *http.Request) { panic("boom") })
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "Internal server error") || !strings.Contains(logs.String(), "[panic] GET /: boom") {
		t.Fatalf("%d %s / log %q", resp.StatusCode, body, logs.String())
	}
}
