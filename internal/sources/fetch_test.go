package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFetchText_ETagNotModified(t *testing.T) {
	const etag = "abc"
	const lm = "Wed, 21 Oct 2015 07:28:00 GMT"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Last-Modified", lm)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()

	ctx := context.Background()
	res1 := FetchText(ctx, srv.URL, 2*time.Second, "", "")
	if !res1.OK || res1.NotModified {
		t.Fatalf("first fetch: OK=%v NotModified=%v err=%q", res1.OK, res1.NotModified, res1.Error)
	}
	if res1.Content != "hello" {
		t.Fatalf("first fetch content = %q", res1.Content)
	}
	if res1.ETag != etag {
		t.Fatalf("first fetch etag = %q", res1.ETag)
	}
	if res1.LastModified == "" {
		t.Fatalf("first fetch last_modified empty")
	}

	res2 := FetchText(ctx, srv.URL, 2*time.Second, res1.ETag, res1.LastModified)
	if !res2.OK || !res2.NotModified {
		t.Fatalf("second fetch: OK=%v NotModified=%v err=%q status=%d", res2.OK, res2.NotModified, res2.Error, res2.Status)
	}
}

