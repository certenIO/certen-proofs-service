package server

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/certen/proofs-service/pkg/database"
)

// RB12-OPS-C item 4: the /v2 evidence bound is the spec bound (two stored levels of at most 64 MiB each), and a document of the
// largest size is written from the rows, not built: the evidence is held once.

// sinkWriter is a ResponseWriter that keeps no body: it counts what is written and records the write deadline it was given.
type sinkWriter struct {
	hdr       http.Header
	code      int
	n         int64
	failAfter int64 // a write that would pass this many bytes fails (0: never)
	deadline  time.Time
}

func newSink() *sinkWriter { return &sinkWriter{hdr: http.Header{}} }

func (s *sinkWriter) Header() http.Header { return s.hdr }
func (s *sinkWriter) WriteHeader(c int)   { s.code = c }
func (s *sinkWriter) Write(p []byte) (int, error) {
	if s.code == 0 {
		s.code = http.StatusOK
	}
	if s.failAfter > 0 && s.n+int64(len(p)) > s.failAfter {
		return 0, errors.New("the client went away")
	}
	s.n += int64(len(p))
	return len(p), nil
}
func (s *sinkWriter) SetWriteDeadline(t time.Time) error { s.deadline = t; return nil }

func serveV2(h *PortableHandlers, proof uuid.UUID, w http.ResponseWriter, hdr map[string]string) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/proofs/"+proof.String()+"/v2", nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	h.HandleGetProofV2(w, req)
}

// maxRows are two addenda whose evidence adds up to exactly MaxFinalityEvidenceBytes (each starts with a well-formed CFE1 header).
func maxRows(proof uuid.UUID) []database.FinalityAddendum {
	each := int(MaxFinalityEvidenceBytes / 2)
	mk := func(seq int64, level byte, kind string) database.FinalityAddendum {
		ev := make([]byte, each)
		copy(ev, cfe1(kind, 84532, level, 47368146, 0x33))
		return row(proof, seq, int16(level), 84532, kind, ev)
	}
	return []database.FinalityAddendum{mk(1, 2, "base-sepolia/l1_posted/v1"), mk(2, 3, "base-sepolia/finalized/v1")}
}

// The bytes written are the bytes json.Marshal of the composed document is, for every vector set and with or without the govRoot inputs;
// compressed or not.
func TestTheStreamedBodyIsByteForByteTheComposedDocument(t *testing.T) {
	vectors := loadVectors(t)
	proof := vectorProof(t)
	for _, name := range []string{"valid_sepolia_finalized", "valid_base_l1_posted_then_finalized", "valid_base_finalized_then_l1_posted"} {
		rows := rowsOf(t, vectors[name])
		for docName, p := range map[string]*database.PortableV2{"with govRoot inputs": stored2(), "without": stored()} {
			section, err := ComposeFinalitySection(proof, rows)
			if err != nil {
				t.Fatal(err)
			}
			want, err := ComposePortableWithFinality(p, section)
			if err != nil {
				t.Fatal(err)
			}
			h := NewPortableHandlers(&fakePortable{p: p}, nil).WithFinality(&fakeFinality{rows: rows})

			plain := httptest.NewRecorder()
			serveV2(h, proof, plain, nil)
			if plain.Code != http.StatusOK || plain.Body.String() != string(want) {
				t.Fatalf("%s / %s: the streamed body is not the composed document\n got %.200s\nwant %.200s", name, docName, plain.Body, want)
			}
			if cl := plain.Header().Get("Content-Length"); cl != strconv.Itoa(len(want)) {
				t.Fatalf("%s / %s: Content-Length %q for a body of %d", name, docName, cl, len(want))
			}

			zipped := httptest.NewRecorder()
			serveV2(h, proof, zipped, map[string]string{"Accept-Encoding": "gzip"})
			zr, err := gzip.NewReader(zipped.Body)
			if err != nil {
				t.Fatalf("%s / %s: %v", name, docName, err)
			}
			got, err := io.ReadAll(zr)
			if err != nil || !bytes.Equal(got, want) || zipped.Header().Get("Content-Encoding") != "gzip" || zipped.Header().Get("Content-Length") != "" {
				t.Fatalf("%s / %s: the gzip body is not the composed document (err %v, encoding %q, length %q)", name, docName, err, zipped.Header().Get("Content-Encoding"), zipped.Header().Get("Content-Length"))
			}
		}
	}
}

// A document with the maximum evidence is served by writing it from the rows: the evidence is not copied. The rows are held by the test
// (as the database read holds them in production); serving adds a small constant, not a multiple of the evidence. The old path marshalled
// the section (the hex), then the document around it, then a compressed copy: about eight times the evidence.
func TestServingTheLargestLegalDocumentCopiesNoEvidence(t *testing.T) {
	proof := uuid.New()
	rows := maxRows(proof)
	var held int64
	for _, r := range rows {
		held += int64(len(r.Evidence))
	}
	if held != MaxFinalityEvidenceBytes {
		t.Fatalf("the rows hold %d bytes, the bound is %d", held, MaxFinalityEvidenceBytes)
	}
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows})
	sink := newSink()

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	var peak uint64
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		var m runtime.MemStats
		for {
			runtime.ReadMemStats(&m)
			if m.HeapAlloc > peak {
				peak = m.HeapAlloc
			}
			select {
			case <-stop:
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	start := time.Now()
	serveV2(h, proof, sink, nil)
	close(stop)
	wg.Wait()
	runtime.ReadMemStats(&after)

	if sink.code != http.StatusOK {
		t.Fatalf("status %d", sink.code)
	}
	limit := uint64(MaxFinalityEvidenceBytes / 16)
	allocated := after.TotalAlloc - before.TotalAlloc
	grew := uint64(0)
	if peak > before.HeapAlloc {
		grew = peak - before.HeapAlloc
	}
	t.Logf("body %d bytes; allocated while serving %d bytes; peak heap above the held rows %d bytes (limit %d)", sink.n, allocated, grew, limit)
	if allocated > limit {
		t.Errorf("serving allocated %d bytes for %d bytes of evidence (limit %d): the evidence is being copied", allocated, MaxFinalityEvidenceBytes, limit)
	}
	if grew > limit {
		t.Errorf("the heap grew by %d bytes while serving (limit %d)", grew, limit)
	}
	cl, err := strconv.ParseInt(sink.hdr.Get("Content-Length"), 10, 64)
	if err != nil || cl != sink.n {
		t.Fatalf("Content-Length %q, bytes written %d", sink.hdr.Get("Content-Length"), sink.n)
	}
	if cl < 2*MaxFinalityEvidenceBytes {
		t.Fatalf("a body of %d bytes cannot carry the hex of %d bytes of evidence", cl, MaxFinalityEvidenceBytes)
	}
	// the response was given the time its size needs: a base window plus a second per MiB
	want := 30*time.Second + time.Duration(cl>>20)*time.Second
	if got := sink.deadline.Sub(start); got < want-5*time.Second || got > want+5*time.Second {
		t.Errorf("the write deadline is %v after the request, want about %v", got, want)
	}
}

func TestAnSmallDocumentIsGivenTheBaseWriteWindow(t *testing.T) {
	proof, rows := twoRows(t)
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows})
	sink := newSink()
	start := time.Now()
	serveV2(h, proof, sink, nil)
	if got := sink.deadline.Sub(start); sink.deadline.IsZero() || got < 25*time.Second || got > 35*time.Second {
		t.Fatalf("the write deadline of a small document is %v after the request", got)
	}
}

// The status is sent before the body: a write that fails then aborts the response (the connection is closed, the reader sees a body
// shorter than its Content-Length), it is never ended as if it were the whole document.
func TestAWriteFailureMidBodyAbortsTheResponse(t *testing.T) {
	proof, rows := twoRows(t)
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows})
	sink := newSink()
	sink.failAfter = 200
	var recovered any
	func() {
		defer func() { recovered = recover() }()
		serveV2(h, proof, sink, nil)
	}()
	if recovered != http.ErrAbortHandler {
		t.Fatalf("a failed write ended with %v, want http.ErrAbortHandler", recovered)
	}
	if sink.n > sink.failAfter {
		t.Fatalf("%d bytes were delivered past the failure point %d", sink.n, sink.failAfter)
	}
}

// A stored document that spells its finality member in any case is refused as the stored finality member: the Go consumers of the
// document match member names case-insensitively.
func TestAStoredFinalityMemberInAnyCaseIsRefused(t *testing.T) {
	proof := uuid.New()
	for _, spelling := range []string{"finality", "Finality", "FINALITY", "fInAlItY"} {
		p := stored2()
		p.Document = strings.Replace(p.Document, "{", `{"`+spelling+`":{"format":"x"},`, 1)
		rr := httptest.NewRecorder()
		serveV2(NewPortableHandlers(&fakePortable{p: p}, nil), proof, rr, nil)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "finality_member_stored_in_document") {
			t.Errorf("%s: %d %s", spelling, rr.Code, rr.Body)
		}
	}
}

// Review D6 (RB12-OPS-C): the size is read in one statement and the rows in another; a row that grew in between must not be served
// above the bound. The bound is asserted again on the rows that were read.
func TestTheBoundIsAssertedAgainOnTheRowsThatWereRead(t *testing.T) {
	proof := uuid.New()
	each := int(MaxFinalityEvidenceBytes / 2)
	mk := func(seq int64, level byte, kind string, size int) database.FinalityAddendum {
		ev := make([]byte, size)
		copy(ev, cfe1(kind, 84532, level, 47368146, 0x33))
		return row(proof, seq, int16(level), 84532, kind, ev)
	}
	for _, c := range []struct {
		name  string
		extra int
		ok    bool
	}{{"the rows add up to exactly the bound", 0, true}, {"the rows add up to one byte over the bound", 1, false}} {
		rows := []database.FinalityAddendum{mk(1, 2, "base-sepolia/l1_posted/v1", each), mk(2, 3, "base-sepolia/finalized/v1", each+c.extra)}
		small := int64(10) // what the database reported before the rows grew
		fin := &fakeFinality{rows: rows, bytes: &small}
		sink := newSink()
		serveV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin), proof, sink, nil)
		if c.ok {
			if sink.code != http.StatusOK || sink.n < 2*MaxFinalityEvidenceBytes {
				t.Errorf("%s: %d, %d bytes", c.name, sink.code, sink.n)
			}
			continue
		}
		rr := httptest.NewRecorder()
		serveV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows, bytes: &small}), proof, rr, nil)
		want := "bytes=" + itoa(MaxFinalityEvidenceBytes+1) + " limit=" + itoa(MaxFinalityEvidenceBytes)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "FINALITY_SECTION_UNUSABLE") ||
			!strings.Contains(rr.Body.String(), "finality_evidence_too_large") || !strings.Contains(rr.Body.String(), want) || strings.Contains(rr.Body.String(), "majors") {
			t.Errorf("%s: %d %.300s", c.name, rr.Code, rr.Body)
		}
	}
}

// Review (LOW): the members this service composes into the document are read by exact key; a stored member spelled in another case
// (which a case-insensitive Go consumer would read, possibly after the composed one) is refused by name.
func TestAStoredCaseVariantOfAComposedMemberIsRefused(t *testing.T) {
	proof := uuid.New()
	for _, c := range []struct{ spelling, member string }{
		{"Majors", "majors"}, {"MAJORS", "majors"}, {"majorſ", "majors"}, // U+017F folds to s
		{"govrootv3inputs", "govRootV3Inputs"}, {"GovRootV3Inputs", "govRootV3Inputs"}, {"GOVROOTV3INPUTS", "govRootV3Inputs"},
	} {
		p := stored2()
		p.Document = strings.Replace(p.Document, "{", `{"`+c.spelling+`":1,`, 1)
		rr := httptest.NewRecorder()
		serveV2(NewPortableHandlers(&fakePortable{p: p}, nil), proof, rr, nil)
		if rr.Code != http.StatusInternalServerError || !strings.Contains(rr.Body.String(), "portable_member_name_case") || !strings.Contains(rr.Body.String(), "member="+c.member) {
			t.Errorf("%s: %d %.200s", c.spelling, rr.Code, rr.Body)
		}
		if strings.Contains(rr.Body.String(), c.spelling) {
			t.Errorf("%s: the stored spelling is echoed", c.spelling)
		}
	}
	// the exact spellings are what they always were
	p := stored2()
	p.Document = strings.Replace(p.Document, "{", `{"majors":[],`, 1)
	rr := httptest.NewRecorder()
	serveV2(NewPortableHandlers(&fakePortable{p: p}, nil), proof, rr, nil)
	if rr.Code != http.StatusOK {
		t.Errorf("an exact stored majors member is composed over, as before: %d %.200s", rr.Code, rr.Body)
	}
}

// --- bounded concurrent serves of large documents (review V-OPS-C2, memory) ---------------------------------------------------------------

// gatedSink is a ResponseWriter whose first body write announces itself and then waits to be released, so a serve can be held open.
type gatedSink struct {
	*sinkWriter
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGated() *gatedSink {
	return &gatedSink{sinkWriter: newSink(), started: make(chan struct{}), release: make(chan struct{})}
}

func (g *gatedSink) Write(p []byte) (int, error) {
	g.once.Do(func() { close(g.started) })
	<-g.release
	return g.sinkWriter.Write(p)
}

// A serve whose evidence is above LargeServeThresholdBytes costs about twice that in memory (the driver's buffer and the scanned copy), so
// the number of them at once is bounded: the next is refused by name BEFORE any row is read, and a small document is never held back.
func TestLargeServesAreBoundedAndTheNextIsRefusedByNameBeforeAnyRowIsRead(t *testing.T) {
	proof, rows := twoRows(t)
	big := LargeServeThresholdBytes + 1
	fin := &fakeFinality{rows: rows, bytes: &big}
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin).WithLargeServeLimit(2)

	var wg sync.WaitGroup
	hold := func() *gatedSink {
		g := newGated()
		wg.Add(1)
		go func() { defer wg.Done(); serveV2(h, proof, g, nil) }()
		select {
		case <-g.started:
		case <-time.After(10 * time.Second):
			t.Fatal("a large serve did not start")
		}
		return g
	}
	g1, g2 := hold(), hold()
	readsWhileFull := fin.evidenceReads

	rr := httptest.NewRecorder()
	serveV2(h, proof, rr, nil)
	if rr.Code != http.StatusServiceUnavailable || !strings.Contains(rr.Body.String(), "PORTABLE_V2_BUSY") || !strings.Contains(rr.Body.String(), "portable_v2_busy limit=2") {
		t.Fatalf("the third large serve: %d %s", rr.Code, rr.Body)
	}
	if rr.Header().Get("Retry-After") == "" {
		t.Fatal("the refusal does not say when to retry")
	}
	if fin.evidenceReads != readsWhileFull {
		t.Fatalf("the refused request read the evidence rows (%d reads, %d before)", fin.evidenceReads, readsWhileFull)
	}

	// a document whose evidence is small is not held back by the large ones (here the same handler, another size)
	small := LargeServeThresholdBytes
	fin2 := &fakeFinality{rows: rows, bytes: &small}
	rr2 := httptest.NewRecorder()
	serveV2(NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(fin2).WithLargeServeLimit(2), proof, rr2, nil)
	if rr2.Code != http.StatusOK {
		t.Fatalf("a document at the threshold: %d", rr2.Code)
	}

	close(g1.release)
	close(g2.release)
	wg.Wait()
	if g1.code != http.StatusOK || g2.code != http.StatusOK {
		t.Fatalf("the held serves ended %d and %d", g1.code, g2.code)
	}
	rr3 := httptest.NewRecorder()
	serveV2(h, proof, rr3, nil)
	if rr3.Code != http.StatusOK {
		t.Fatalf("after the serves ended the next is served: %d %.200s", rr3.Code, rr3.Body)
	}
}

// A serve that is aborted (the status was sent, a write failed) gives its slot back: the limit does not leak.
func TestAnAbortedLargeServeGivesItsSlotBack(t *testing.T) {
	proof, rows := twoRows(t)
	big := LargeServeThresholdBytes + 1
	h := NewPortableHandlers(&fakePortable{p: stored2()}, nil).WithFinality(&fakeFinality{rows: rows, bytes: &big}).WithLargeServeLimit(1)
	for i := 0; i < 3; i++ {
		sink := newSink()
		sink.failAfter = 100
		func() {
			defer func() {
				if r := recover(); r != http.ErrAbortHandler {
					t.Fatalf("serve %d ended with %v, want http.ErrAbortHandler", i, r)
				}
			}()
			serveV2(h, proof, sink, nil)
		}()
	}
	rr := httptest.NewRecorder()
	serveV2(h, proof, rr, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("after three aborted serves with a limit of one: %d %.200s", rr.Code, rr.Body)
	}
}

func TestTheDefaultLimitOfLargeServesIsFour(t *testing.T) {
	if DefaultLargeServeLimit != 4 || LargeServeThresholdBytes != 16<<20 {
		t.Fatalf("limit %d, threshold %d", DefaultLargeServeLimit, LargeServeThresholdBytes)
	}
}
