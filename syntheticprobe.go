package syntheticprobe

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// Error is the typed error returned by Probe.Validate and NewTracker.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string {
	return fmt.Sprintf("syntheticprobe: %s: %s", e.Code, e.Msg)
}

// Probe describes a single synthetic HTTP probe.
type Probe struct {
	Name         string
	Method       string
	URL          string
	Header       http.Header
	Body         []byte
	ExpectStatus []int
	BodyContains string
	MaxLatency   time.Duration
	Timeout      time.Duration
}

var validMethods = map[string]struct{}{
	"GET":     {},
	"HEAD":    {},
	"POST":    {},
	"PUT":     {},
	"PATCH":   {},
	"DELETE":  {},
	"OPTIONS": {},
}

// Validate reports whether the probe is well formed. It returns an *Error with code
// "invalid_probe" describing the first problem found.
func (p Probe) Validate() error {
	invalid := func(msg string) error { return &Error{Code: "invalid_probe", Msg: msg} }
	if p.Name == "" {
		return invalid("name must not be empty")
	}
	if _, ok := validMethods[p.Method]; !ok {
		return invalid("method must be one of GET, HEAD, POST, PUT, PATCH, DELETE, OPTIONS")
	}
	u, err := url.Parse(p.URL)
	if err != nil {
		return invalid("url does not parse: " + err.Error())
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return invalid("url scheme must be http or https")
	}
	if u.Host == "" {
		return invalid("url must have a host")
	}
	if u.User != nil {
		return invalid("url must not contain userinfo")
	}
	for _, s := range p.ExpectStatus {
		if s < 100 || s > 599 {
			return invalid("expect status values must be in 100..599")
		}
	}
	if p.Timeout < 0 {
		return invalid("timeout must not be negative")
	}
	return nil
}

// Doer performs HTTP requests; *http.Client satisfies this interface.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// Result is the outcome of a probe run. It never carries headers, URL, or body bytes.
type Result struct {
	Probe   string
	OK      bool
	Failure string
	Status  int
	Latency time.Duration
	At      time.Time
}

// Runner executes probes with an injectable doer and clock.
type Runner struct {
	Doer         Doer
	Now          func() time.Time
	MaxBodyBytes int64
	OnResult     func(Result)
}

// Run executes the probe and returns the result. It validates the probe first;
// if invalid, it returns a result with Failure "invalid_probe" without calling the doer.
// The result is also passed to OnResult if set.
func (r *Runner) Run(ctx context.Context, p Probe) Result {
	start := r.now()
	if err := p.Validate(); err != nil {
		res := Result{Probe: p.Name, Failure: "invalid_probe", At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	if r.Doer == nil {
		res := Result{Probe: p.Name, Failure: "transport", At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	expect := p.ExpectStatus
	if len(expect) == 0 {
		expect = []int{200}
	}
	timeout := p.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	maxBytes := r.MaxBodyBytes
	if maxBytes <= 0 {
		maxBytes = 1_048_576
	}
	bodyReader := bytes.NewReader(p.Body)
	req, err := http.NewRequestWithContext(ctx, p.Method, p.URL, bodyReader)
	if err != nil {
		res := Result{Probe: p.Name, Failure: "transport", At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	if p.Header != nil {
		req.Header = p.Header.Clone()
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req = req.WithContext(runCtx)
	start = r.now()
	resp, err := r.Doer.Do(req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		latency := r.now().Sub(start)
		failure := "transport"
		if errors.Is(err, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded {
			failure = "timeout"
		}
		res := Result{Probe: p.Name, Failure: failure, Status: 0, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxBytes+1)
	bodyData, readErr := io.ReadAll(limited)
	latency := r.now().Sub(start)
	if readErr != nil {
		failure := "transport"
		if errors.Is(readErr, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded {
			failure = "timeout"
		}
		res := Result{Probe: p.Name, Failure: failure, Status: 0, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	statusOK := false
	for _, s := range expect {
		if resp.StatusCode == s {
			statusOK = true
			break
		}
	}
	if !statusOK {
		res := Result{Probe: p.Name, Failure: "status", Status: resp.StatusCode, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	if int64(len(bodyData)) > maxBytes {
		res := Result{Probe: p.Name, Failure: "body_too_large", Status: resp.StatusCode, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	if p.BodyContains != "" && !bytes.Contains(bodyData, []byte(p.BodyContains)) {
		res := Result{Probe: p.Name, Failure: "body", Status: resp.StatusCode, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	if p.MaxLatency > 0 && latency > p.MaxLatency {
		res := Result{Probe: p.Name, Failure: "latency", Status: resp.StatusCode, Latency: latency, At: start}
		if r.OnResult != nil {
			r.OnResult(res)
		}
		return res
	}
	res := Result{Probe: p.Name, OK: true, Status: resp.StatusCode, Latency: latency, At: start}
	if r.OnResult != nil {
		r.OnResult(res)
	}
	return res
}

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now()
}

// Report summarizes the tracker state.
type Report struct {
	Total           int
	Failures        int
	SuccessRatio    float64
	BudgetRemaining float64
	Breached        bool
}

// Tracker maintains a rolling window of probe results and computes SLO metrics.
type Tracker struct {
	target   float64
	window   int
	mu       sync.Mutex
	results  []Result
	idx      int
	count    int
	failures int
}

// NewTracker creates a tracker with the given availability target (exclusive between 0 and 1)
// and window size (1..100000). Returns an *Error with code "invalid_target" on invalid input.
func NewTracker(target float64, window int) (*Tracker, error) {
	if math.IsNaN(target) || target <= 0 || target >= 1 || window < 1 || window > 100_000 {
		return nil, &Error{Code: "invalid_target", Msg: "target must be (0,1) and window 1..100000"}
	}
	return &Tracker{
		target:  target,
		window:  window,
		results: make([]Result, window),
	}, nil
}

// Add records a result, evicting the oldest if the window is full.
func (t *Tracker) Add(r Result) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.count < t.window {
		t.results[t.count] = r
		t.count++
	} else {
		old := t.results[t.idx]
		if !old.OK {
			t.failures--
		}
		t.results[t.idx] = r
		t.idx = (t.idx + 1) % t.window
	}
	if !r.OK {
		t.failures++
	}
}

// Report returns the current SLO report.
func (t *Tracker) Report() Report {
	t.mu.Lock()
	defer t.mu.Unlock()
	total := t.count
	failures := t.failures
	if total == 0 {
		return Report{Total: 0, Failures: 0, SuccessRatio: 1.0, BudgetRemaining: 1.0, Breached: false}
	}
	successRatio := float64(total-failures) / float64(total)
	allowed := (1 - t.target) * float64(total)
	budgetRemaining := 1 - float64(failures)/allowed
	breached := successRatio < t.target
	return Report{
		Total:           total,
		Failures:        failures,
		SuccessRatio:    successRatio,
		BudgetRemaining: budgetRemaining,
		Breached:        breached,
	}
}
