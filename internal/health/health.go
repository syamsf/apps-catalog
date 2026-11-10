// Package health checks if apps respond.
//
// Up means any response below 500 within the timeout. Redirects are not
// followed, so a login page that answers 302 counts as up.
package health

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"apps-catalog/internal/catalog"
	"apps-catalog/internal/store"
)

type Store interface {
	AllApps(ctx context.Context) ([]catalog.App, error)
	SaveStatus(ctx context.Context, appID string, st store.Status) error
	ClearStatus(ctx context.Context, appID string) error
}

type Checker struct {
	client      *http.Client
	concurrency int
}

// NewChecker builds the http client. caFile is an optional PEM bundle for
// internal CAs, added on top of the system roots.
func NewChecker(timeout time.Duration, caFile string) (*Checker, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DisableKeepAlives = true
	if caFile != "" {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("health ca file: %w", err)
		}
		if !pool.AppendCertsFromPEM(pem) {
			return nil, fmt.Errorf("health ca file %s: no certificates found", caFile)
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return &Checker{
		client: &http.Client{
			Transport: tr,
			Timeout:   timeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		concurrency: 10,
	}, nil
}

func (c *Checker) Check(ctx context.Context, target string) (st store.Status) {
	st.Target = target
	start := time.Now()
	defer func() {
		st.LatencyMS = int(time.Since(start).Milliseconds())
		st.CheckedAt = time.Now()
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		st.Error = err.Error()
		return st
	}
	req.Header.Set("User-Agent", "app-catalog-health/1")
	resp, err := c.client.Do(req)
	if err != nil {
		st.Error = shortError(err)
		return st
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()

	st.HTTPStatus = resp.StatusCode
	st.Up = resp.StatusCode < 500
	if !st.Up {
		st.Error = resp.Status
	}
	return st
}

// CheckAll checks every app once and saves the results.
func (c *Checker) CheckAll(ctx context.Context, s Store, log zerolog.Logger) error {
	apps, err := s.AllApps(ctx)
	if err != nil {
		return err
	}
	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	for _, app := range apps {
		target := app.HealthTarget()
		if target == "" {
			if err := s.ClearStatus(ctx, app.ID); err != nil {
				log.Error().Err(err).Str("app", app.ID).Msg("clear status")
			}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer func() { <-sem; wg.Done() }()
			st := c.Check(ctx, target)
			if ctx.Err() != nil {
				return
			}
			log.Debug().Str("app", app.ID).Str("target", target).Bool("up", st.Up).
				Int("http_status", st.HTTPStatus).Int("latency_ms", st.LatencyMS).Str("error", st.Error).Msg("checked")
			if err := s.SaveStatus(ctx, app.ID, st); err != nil {
				log.Error().Err(err).Str("app", app.ID).Msg("save status")
			}
		}()
	}
	wg.Wait()
	return nil
}

// Run checks all apps every interval until ctx is done.
func (c *Checker) Run(ctx context.Context, s Store, log zerolog.Logger, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if err := c.CheckAll(ctx, s, log); err != nil && ctx.Err() == nil {
			log.Error().Err(err).Msg("health checks failed")
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func shortError(err error) string {
	var urlErr interface{ Timeout() bool }
	if errors.As(err, &urlErr) && urlErr.Timeout() {
		return "timeout"
	}
	var ue interface{ Unwrap() error }
	if errors.As(err, &ue) && ue.Unwrap() != nil {
		return ue.Unwrap().Error()
	}
	return err.Error()
}
