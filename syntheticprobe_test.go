package syntheticprobe_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Trysac/u235-synthetic-probe"
)

type fakeDoer struct {
	resp *http.Response
	err  error
	// optional hook to advance clock inside Do
	advance func()
}

func (f *fakeDoer) Do(req *http.Request) (*http.Response, error) {
	if f.advance != nil {
		f.advance()
	}
	return f.resp, f.err
}

func TestProbeValidate(t *testing.T) {
	tests := []struct {
		name string
		p    syntheticprobe.Probe
		ok   bool
	}{
		{"valid", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.com/", ExpectStatus: []int{200}, Timeout: time.Second}, true},
		{"empty name", syntheticprobe.Probe{Method: "GET", URL: "https://example.com/"}, false},
		{"bad method", syntheticprobe.Probe{Name: "n", Method: "TRACE", URL: "https://example.com/"}, false},
		{"bad scheme", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "ftp://example.com/"}, false},
		{"no host", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https:///path"}, false},
		{"userinfo", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://u:p@example.test/"}, false},
		{"invalid expect status", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.com/", ExpectStatus: []int{5999}}, false},
		{"negative timeout", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.com/", Timeout: -time.Second}, false},
		{"defaults ok", syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.com/"}, true},
		{"all methods", syntheticprobe.Probe{Name: "n", Method: "HEAD", URL: "https://example.com/"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.p.Validate()
			if tt.ok && err != nil {
				t.Fatalf("Validate() = %v, want nil", err)
			}
			if !tt.ok {
				if err == nil {
					t.Fatal("Validate() = nil, want error")
				}
				var se *syntheticprobe.Error
				if !errors.As(err, &se) {
					t.Fatalf("Validate() error type = %T, want *syntheticprobe.Error", err)
				}
				if se.Code != "invalid_probe" {
					t.Fatalf("Validate() error code = %q, want %q", se.Code, "invalid_probe")
				}
			}
		})
	}
}

func TestRunSuccess(t *testing.T) {
	var now time.Time
	now = time.Unix(0, 0)
	r := &syntheticprobe.Runner{
		Doer: &fakeDoer{
			resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok: true"))},
			advance: func() {
				now = now.Add(120 * time.Millisecond)
			},
		},
		Now: func() time.Time { return now },
	}
	res := r.Run(context.Background(), syntheticprobe.Probe{
		Name:         "home",
		Method:       "GET",
		URL:          "https://example.test/",
		BodyContains: "ok: true",
		MaxLatency:   200 * time.Millisecond,
	})
	if res.Probe != "home" || !res.OK || res.Failure != "" || res.Status != 200 || res.Latency != 120*time.Millisecond {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestRunFailures(t *testing.T) {
	tests := []struct {
		name       string
		doer       syntheticprobe.Doer
		probe      syntheticprobe.Probe
		wantFail   string
		wantStatus int
	}{
		{
			"status",
			&fakeDoer{resp: &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(""))}},
			syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"},
			"status", 503,
		},
		{
			"body",
			&fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("no match"))}},
			syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true"},
			"body", 200,
		},
		{
			"latency",
			&fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, advance: func() { /* not used */ }},
			syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", MaxLatency: 200 * time.Millisecond},
			"latency", 200,
		},
		{
			"transport",
			&fakeDoer{err: errors.New("boom")},
			syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"},
			"transport", 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			now := time.Unix(0, 0)
			if tt.name == "latency" {
				// override doer to advance clock by 250ms
				tt.doer = &fakeDoer{
					resp:    &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))},
					advance: func() { now = now.Add(250 * time.Millisecond) },
				}
			}
			r := &syntheticprobe.Runner{Doer: tt.doer, Now: func() time.Time { return now }}
			res := r.Run(context.Background(), tt.probe)
			if res.OK || res.Failure != tt.wantFail || res.Status != tt.wantStatus {
				t.Fatalf("unexpected result: %+v", res)
			}
		})
	}
}

func TestRunBodyTooLarge(t *testing.T) {
	bigBody := bytes.Repeat([]byte("a"), 1_048_577)
	doer := &fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(bigBody))}}
	r := &syntheticprobe.Runner{Doer: doer, MaxBodyBytes: 1_048_576}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
	if res.OK || res.Failure != "body_too_large" || res.Status != 200 {
		t.Fatalf("unexpected result: %+v", res)
	}
	// exact size accepted
	exactBody := bytes.Repeat([]byte("a"), 1_048_576)
	doer = &fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(exactBody))}}
	res = r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
	if !res.OK || res.Failure != "" {
		t.Fatalf("expected success, got %+v", res)
	}
}

func TestRunCheckOrder(t *testing.T) {
	largeWithMatch := append([]byte("ok: true"), bytes.Repeat([]byte("a"), 1_048_577)...)
	tests := []struct {
		name        string
		status      int
		body        []byte
		advanceBy   time.Duration
		probe       syntheticprobe.Probe
		wantFailure string
		wantStatus  int
	}{
		{
			name:        "status wins over body_too_large and body",
			status:      503,
			body:        bytes.Repeat([]byte("a"), 1_048_577),
			probe:       syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true"},
			wantFailure: "status",
			wantStatus:  503,
		},
		{
			name:        "body_too_large wins over body",
			status:      200,
			body:        largeWithMatch,
			probe:       syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true"},
			wantFailure: "body_too_large",
			wantStatus:  200,
		},
		{
			name:        "body wins over latency",
			status:      200,
			body:        []byte("no match"),
			advanceBy:   250 * time.Millisecond,
			probe:       syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true", MaxLatency: 200 * time.Millisecond},
			wantFailure: "body",
			wantStatus:  200,
		},
		{
			name:        "latency when everything else passes",
			status:      200,
			body:        []byte("ok: true"),
			advanceBy:   250 * time.Millisecond,
			probe:       syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true", MaxLatency: 200 * time.Millisecond},
			wantFailure: "latency",
			wantStatus:  200,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var now time.Time
			doer := &fakeDoer{
				resp:    &http.Response{StatusCode: tt.status, Body: io.NopCloser(bytes.NewReader(tt.body))},
				advance: func() { now = now.Add(tt.advanceBy) },
			}
			r := &syntheticprobe.Runner{Doer: doer, Now: func() time.Time { return now }}
			res := r.Run(context.Background(), tt.probe)
			if res.OK || res.Failure != tt.wantFailure || res.Status != tt.wantStatus {
				t.Fatalf("unexpected result: %+v", res)
			}
		})
	}
}

func TestBodyClosedOnEveryPath(t *testing.T) {
	tests := []struct {
		name        string
		maxBody     int64
		probe       syntheticprobe.Probe
		advanceBy   time.Duration
		doer        func(closed *bool, advance func()) syntheticprobe.Doer
		wantFailure string
	}{
		{
			name: "success",
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{resp: &http.Response{StatusCode: 200, Body: &closeTracker{Reader: strings.NewReader("data"), closed: closed}}}
			},
		},
		{
			name: "status",
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{resp: &http.Response{StatusCode: 503, Body: &closeTracker{Reader: strings.NewReader(""), closed: closed}}}
			},
			wantFailure: "status",
		},
		{
			name:    "body_too_large",
			maxBody: 8,
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{resp: &http.Response{StatusCode: 200, Body: &closeTracker{Reader: strings.NewReader("0123456789"), closed: closed}}}
			},
			wantFailure: "body_too_large",
		},
		{
			name:  "body",
			probe: syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", BodyContains: "ok: true"},
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{resp: &http.Response{StatusCode: 200, Body: &closeTracker{Reader: strings.NewReader("no match"), closed: closed}}}
			},
			wantFailure: "body",
		},
		{
			name:      "latency",
			probe:     syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", MaxLatency: 200 * time.Millisecond},
			advanceBy: 250 * time.Millisecond,
			doer: func(closed *bool, advance func()) syntheticprobe.Doer {
				return &fakeDoer{
					resp:    &http.Response{StatusCode: 200, Body: &closeTracker{Reader: strings.NewReader(""), closed: closed}},
					advance: advance,
				}
			},
			wantFailure: "latency",
		},
		{
			name: "read error",
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{resp: &http.Response{StatusCode: 200, Body: &errCloseReader{closed: closed, err: errors.New("read boom")}}}
			},
			wantFailure: "transport",
		},
		{
			name: "doer error with response",
			doer: func(closed *bool, _ func()) syntheticprobe.Doer {
				return &fakeDoer{
					resp: &http.Response{StatusCode: 200, Body: &closeTracker{Reader: strings.NewReader(""), closed: closed}},
					err:  errors.New("boom"),
				}
			},
			wantFailure: "transport",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closed := false
			var now time.Time
			advance := func() { now = now.Add(tt.advanceBy) }
			r := &syntheticprobe.Runner{
				Doer:         tt.doer(&closed, advance),
				Now:          func() time.Time { return now },
				MaxBodyBytes: tt.maxBody,
			}
			probe := tt.probe
			if probe.Name == "" {
				probe = syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"}
			}
			res := r.Run(context.Background(), probe)
			if tt.wantFailure == "" {
				if !res.OK {
					t.Fatalf("unexpected result: %+v", res)
				}
			} else if res.Failure != tt.wantFailure {
				t.Fatalf("Failure = %q, want %q", res.Failure, tt.wantFailure)
			}
			if !closed {
				t.Fatal("response body not closed")
			}
		})
	}
}

type closeTracker struct {
	io.Reader
	closed *bool
}

func (c *closeTracker) Close() error {
	*c.closed = true
	return nil
}

type errCloseReader struct {
	closed *bool
	err    error
}

func (e *errCloseReader) Read([]byte) (int, error) { return 0, e.err }

func (e *errCloseReader) Close() error {
	*e.closed = true
	return nil
}

func TestRunTimeout(t *testing.T) {
	timeoutDoer := &blockingDoer{}
	r := &syntheticprobe.Runner{Doer: timeoutDoer}
	start := time.Now()
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/", Timeout: 20 * time.Millisecond})
	elapsed := time.Since(start)
	if res.OK || res.Failure != "timeout" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if elapsed >= time.Second {
		t.Fatalf("Run took too long: %v", elapsed)
	}
}

type blockingDoer struct{}

func (b *blockingDoer) Do(req *http.Request) (*http.Response, error) {
	<-req.Context().Done()
	return nil, req.Context().Err()
}

func TestRunInvalidProbe(t *testing.T) {
	d := &countingDoer{}
	r := &syntheticprobe.Runner{Doer: d}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "bad", Method: "GET", URL: "ftp://example.test/"})
	if res.OK || res.Failure != "invalid_probe" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if d.calls != 0 {
		t.Fatalf("doer called %d times, want 0", d.calls)
	}
}

type countingDoer struct{ calls int }

func (c *countingDoer) Do(*http.Request) (*http.Response, error) {
	c.calls++
	return nil, errors.New("must not be called")
}

func TestRunUserinfoInvalidProbe(t *testing.T) {
	d := &countingDoer{}
	r := &syntheticprobe.Runner{Doer: d}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "u", Method: "GET", URL: "https://u:p@example.test/"})
	if res.OK || res.Failure != "invalid_probe" {
		t.Fatalf("unexpected result: %+v", res)
	}
	if d.calls != 0 {
		t.Fatalf("doer called %d times, want 0", d.calls)
	}
}

func TestOnResult(t *testing.T) {
	got := []syntheticprobe.Result{}
	r := &syntheticprobe.Runner{
		Doer: &fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}},
		OnResult: func(res syntheticprobe.Result) {
			got = append(got, res)
		},
	}
	probe := syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"}
	expected := r.Run(context.Background(), probe)
	if len(got) != 1 || got[0] != expected {
		t.Fatalf("OnResult mismatch: got %+v, want %+v", got, expected)
	}
}

func TestResultDoesNotContainSensitive(t *testing.T) {
	allowed := map[string]bool{
		"Probe": true, "OK": true, "Failure": true,
		"Status": true, "Latency": true, "At": true,
	}
	typ := reflect.TypeOf(syntheticprobe.Result{})
	if typ.NumField() != len(allowed) {
		t.Fatalf("Result has %d fields, want %d", typ.NumField(), len(allowed))
	}
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		if !allowed[name] {
			t.Fatalf("unexpected Result field %q (must not carry headers, URL or body)", name)
		}
	}
}

func TestErrorString(t *testing.T) {
	e := &syntheticprobe.Error{Code: "invalid_probe", Msg: "boom"}
	if got, want := e.Error(), "syntheticprobe: invalid_probe: boom"; got != want {
		t.Fatalf("Error() = %q, want %q", got, want)
	}
}

func TestRunNilDoer(t *testing.T) {
	r := &syntheticprobe.Runner{}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
	if res.OK || res.Failure != "transport" {
		t.Fatalf("unexpected result: %+v", res)
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("read boom") }
func (errReader) Close() error             { return nil }

func TestRunReadError(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{StatusCode: 200, Body: errReader{}}}
	r := &syntheticprobe.Runner{Doer: doer}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
	if res.OK || res.Failure != "transport" || res.Status != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

type deadlineReader struct{}

func (deadlineReader) Read([]byte) (int, error) { return 0, context.DeadlineExceeded }
func (deadlineReader) Close() error             { return nil }

func TestRunReadErrorDeadline(t *testing.T) {
	doer := &fakeDoer{resp: &http.Response{StatusCode: 200, Body: deadlineReader{}}}
	r := &syntheticprobe.Runner{Doer: doer}
	res := r.Run(context.Background(), syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
	if res.OK || res.Failure != "timeout" || res.Status != 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}

func TestProbeEdgeCases(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		probe       syntheticprobe.Probe
		wantFailure string
	}{
		{
			name:   "HEAD with empty body",
			status: 200,
			probe:  syntheticprobe.Probe{Name: "head", Method: "HEAD", URL: "https://example.test/"},
		},
		{
			name:   "nil header",
			status: 200,
			probe:  syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.test/", Header: nil},
		},
		{
			name:   "multiple expect status accepted",
			status: 204,
			probe:  syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.test/", ExpectStatus: []int{200, 204}},
		},
		{
			name:        "status outside expect status",
			status:      200,
			probe:       syntheticprobe.Probe{Name: "n", Method: "GET", URL: "https://example.test/", ExpectStatus: []int{204}},
			wantFailure: "status",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &syntheticprobe.Runner{
				Doer: &fakeDoer{resp: &http.Response{StatusCode: tt.status, Body: io.NopCloser(strings.NewReader(""))}},
			}
			res := r.Run(context.Background(), tt.probe)
			if tt.wantFailure == "" {
				if !res.OK {
					t.Fatalf("unexpected result: %+v", res)
				}
			} else if res.OK || res.Failure != tt.wantFailure {
				t.Fatalf("unexpected result: %+v", res)
			}
		})
	}
}

type ctxErrorDoer struct{}

func (ctxErrorDoer) Do(req *http.Request) (*http.Response, error) {
	return nil, req.Context().Err()
}

func TestParentContext(t *testing.T) {
	t.Run("canceled parent is transport", func(t *testing.T) {
		parent, cancel := context.WithCancel(context.Background())
		cancel()
		r := &syntheticprobe.Runner{Doer: ctxErrorDoer{}}
		res := r.Run(parent, syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
		if res.OK || res.Failure != "transport" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})
	t.Run("expired parent deadline is timeout", func(t *testing.T) {
		parent, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
		defer cancel()
		r := &syntheticprobe.Runner{Doer: ctxErrorDoer{}}
		res := r.Run(parent, syntheticprobe.Probe{Name: "p", Method: "GET", URL: "https://example.test/"})
		if res.OK || res.Failure != "timeout" {
			t.Fatalf("unexpected result: %+v", res)
		}
	})
}

func TestNewTracker(t *testing.T) {
	tests := []struct {
		name    string
		target  float64
		window  int
		wantErr bool
	}{
		{"valid", 0.9, 20, false},
		{"target zero", 0, 20, true},
		{"target one", 1, 20, true},
		{"target nan", math.NaN(), 20, true},
		{"window zero", 0.9, 0, true},
		{"window too large", 0.9, 100001, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := syntheticprobe.NewTracker(tt.target, tt.window)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				var se *syntheticprobe.Error
				if !errors.As(err, &se) {
					t.Fatalf("error type = %T, want *syntheticprobe.Error", err)
				}
				if se.Code != "invalid_target" {
					t.Fatalf("error code = %q, want %q", se.Code, "invalid_target")
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestTrackerBasic(t *testing.T) {
	tr, err := syntheticprobe.NewTracker(0.9, 20)
	if err != nil {
		t.Fatal(err)
	}
	// 19 OK, 1 failure
	for i := 0; i < 19; i++ {
		tr.Add(syntheticprobe.Result{OK: true})
	}
	tr.Add(syntheticprobe.Result{OK: false})
	rep := tr.Report()
	if rep.Total != 20 || rep.Failures != 1 || rep.SuccessRatio != 0.95 || rep.Breached != false {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if diff := rep.BudgetRemaining - 0.5; diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("BudgetRemaining = %v, want 0.5", rep.BudgetRemaining)
	}
	// add 2 more failures (replace old OK results)
	tr.Add(syntheticprobe.Result{OK: false})
	tr.Add(syntheticprobe.Result{OK: false})
	rep = tr.Report()
	if rep.SuccessRatio != 0.85 || rep.Breached != true {
		t.Fatalf("unexpected report: %+v", rep)
	}
	if diff := rep.BudgetRemaining - (-0.5); diff < -1e-9 || diff > 1e-9 {
		t.Fatalf("BudgetRemaining = %v, want -0.5", rep.BudgetRemaining)
	}
}

func TestTrackerRingBuffer(t *testing.T) {
	tr, _ := syntheticprobe.NewTracker(0.9, 20)
	for i := 0; i < 25; i++ {
		tr.Add(syntheticprobe.Result{OK: i < 20}) // first 20 OK, last 5 failure
	}
	rep := tr.Report()
	if rep.Total != 20 {
		t.Fatalf("Total = %d, want 20 (ring buffer size)", rep.Total)
	}
	if rep.Failures != 5 {
		t.Fatalf("Failures = %d, want 5", rep.Failures)
	}
	// Success ratio = 15/20 = 0.75
	if rep.SuccessRatio != 0.75 {
		t.Fatalf("SuccessRatio = %v, want 0.75", rep.SuccessRatio)
	}
}

func TestTrackerEmptyReport(t *testing.T) {
	tr, _ := syntheticprobe.NewTracker(0.9, 20)
	rep := tr.Report()
	if rep.Total != 0 || rep.Failures != 0 || rep.SuccessRatio != 1.0 || rep.BudgetRemaining != 1.0 || rep.Breached {
		t.Fatalf("unexpected empty report: %+v", rep)
	}
}

func TestTrackerConcurrent(t *testing.T) {
	// A window large enough to retain every result makes the totals deterministic
	// even though the interleaving of the goroutines is not.
	tr, _ := syntheticprobe.NewTracker(0.9, 1000)
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(ok bool) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				tr.Add(syntheticprobe.Result{OK: ok})
			}
		}(i%2 == 0)
	}
	wg.Wait()
	rep := tr.Report()
	if rep.Total != 1000 {
		t.Fatalf("Total = %d, want 1000", rep.Total)
	}
	if rep.Failures != 500 {
		t.Fatalf("Failures = %d, want 500", rep.Failures)
	}
}

func ExampleProbe_Validate() {
	p := syntheticprobe.Probe{Name: "home", Method: "GET", URL: "https://example.com/"}
	err := p.Validate()
	fmt.Println(err == nil)
	// Output:
	// true
}

func ExampleRunner_Run() {
	r := &syntheticprobe.Runner{
		Doer: &fakeDoer{resp: &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("ok"))}},
		Now:  func() time.Time { return time.Unix(0, 0) },
	}
	p := syntheticprobe.Probe{Name: "home", Method: "GET", URL: "https://example.com/"}
	res := r.Run(context.Background(), p)
	fmt.Println(res.OK, res.Status)
	// Output:
	// true 200
}
