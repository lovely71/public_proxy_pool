package sources

import "testing"

func TestBuiltInGitHubSources(t *testing.T) {
	items := BuiltInGitHubSources()
	if len(items) < 100 {
		t.Fatalf("expected at least 100 built-in sources, got %d", len(items))
	}

	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		if item.Name == "" || item.URL == "" {
			t.Fatalf("found invalid built-in source: %+v", item)
		}
		if _, ok := seen[item.Name]; ok {
			t.Fatalf("duplicate built-in source name: %s", item.Name)
		}
		seen[item.Name] = struct{}{}
	}
}
