package checker

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/korotkovfedor/pingwisp/internal/models"
)

const (
	checkTimeout         = 3 * time.Second
	maxResponseBodyBytes = 1 * 1024 * 1024
)

type HTTPChecker struct {
	client *http.Client
}

// NewHTTP creates an HTTP checker. A nil client uses the default transport.
func NewHTTP(client *http.Client) *HTTPChecker {
	if client == nil {
		client = &http.Client{}
	}
	return &HTTPChecker{client: client}
}

func (c *HTTPChecker) Check(ctx context.Context, target models.Target) models.CheckResult {
	ctx, cancel := context.WithTimeout(ctx, checkTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return models.CheckResult{
			Status:      models.StatusDown,
			CompletedAt: time.Now(),
			Error:       err,
		}
	}

	start := time.Now()
	result := models.CheckResult{Status: models.StatusDown}
	resp, err := c.client.Do(req)
	if resp != nil {
		statusCode := resp.StatusCode
		result.StatusCode = &statusCode
	}
	if err == nil {
		defer resp.Body.Close()
		limited := io.LimitReader(resp.Body, maxResponseBodyBytes+1)
		var n int64
		n, err = io.Copy(io.Discard, limited)
		if err == nil && n > maxResponseBodyBytes {
			err = errors.New("response limit exceeded")
		}
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			result.Status = models.StatusUp
		}
	}

	result.CompletedAt = time.Now()
	result.Latency = result.CompletedAt.Sub(start)
	result.Error = err
	return result
}
