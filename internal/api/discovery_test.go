package api

import "testing"

func TestNormalizePostTags(t *testing.T) {
	tags, err := normalizePostTags([]string{"#Go", " go ", "Postgres"})
	if err != nil {
		t.Fatalf("normalizePostTags returned error: %v", err)
	}
	want := []string{"go", "postgres"}
	if len(tags) != len(want) {
		t.Fatalf("got %d tags, want %d", len(tags), len(want))
	}
	for i := range want {
		if tags[i] != want[i] {
			t.Fatalf("tags[%d] = %q, want %q", i, tags[i], want[i])
		}
	}
}

func TestNormalizePostTagsLimit(t *testing.T) {
	tags := make([]string, 11)
	if _, err := normalizePostTags(tags); err == nil {
		t.Fatal("normalizePostTags accepted more than 10 tags")
	}
}

func TestSearchType(t *testing.T) {
	if got, err := searchType(""); err != nil || got != "all" {
		t.Fatalf("empty type = %q, %v; want all", got, err)
	}
	if _, err := searchType("unknown"); err == nil {
		t.Fatal("searchType accepted an unknown type")
	}
}
