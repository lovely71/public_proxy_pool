package sources

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

type FetchResult struct {
	OK          bool
	Status      int
	NotModified bool
	DurationMS  int
	Error       string

	Content      string
	ETag         string
	LastModified string
}

func FetchText(ctx context.Context, url string, timeout time.Duration, etag, lastModified string) FetchResult {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return FetchResult{OK: false, Status: 0, Error: err.Error()}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if lastModified != "" {
		req.Header.Set("If-Modified-Since", lastModified)
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return FetchResult{
			OK:         false,
			Status:     0,
			Error:      err.Error(),
			DurationMS: int(time.Since(start).Milliseconds()),
		}
	}
	defer resp.Body.Close()

	res := FetchResult{
		OK:          resp.StatusCode >= 200 && resp.StatusCode < 400,
		Status:      resp.StatusCode,
		DurationMS:  int(time.Since(start).Milliseconds()),
		ETag:        resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	if resp.StatusCode == http.StatusNotModified {
		res.OK = true
		res.NotModified = true
		return res
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		res.OK = false
		res.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return res
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		res.OK = false
		res.Error = err.Error()
		return res
	}
	res.Content = string(body)
	return res
}

