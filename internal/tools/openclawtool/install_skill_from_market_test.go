package openclawtool

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAsStringNilIsEmpty(t *testing.T) {
	if got := asString(nil); got != "" {
		t.Fatalf("asString(nil) = %q, want empty string", got)
	}
}

func TestSearchSkillsSupportsClawdAPIShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/skills" {
			t.Fatalf("path = %q, want /api/skills", r.URL.Path)
		}
		if got := r.URL.Query().Get("q"); got != "3d-image-generator" {
			t.Fatalf("q = %q, want 3d-image-generator", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{
				"id": "irre-nnn/3d-image-generator",
				"name": "3d-image-generator",
				"description": "Generate 3D rendered art and icons."
			}
		]`))
	}))
	defer server.Close()

	items, err := searchSkills(context.Background(), server.Client(), server.URL+"/api", "3d-image-generator")
	if err != nil {
		t.Fatalf("searchSkills returned error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("len(items) = %d, want 1", len(items))
	}
	if items[0].Slug != "irre-nnn/3d-image-generator" {
		t.Fatalf("Slug = %q, want irre-nnn/3d-image-generator", items[0].Slug)
	}
	if items[0].DisplayName != "3d-image-generator" {
		t.Fatalf("DisplayName = %q, want 3d-image-generator", items[0].DisplayName)
	}
}
