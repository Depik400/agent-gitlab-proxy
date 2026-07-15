package gitlab

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestProjectIDEscapesProjectPath(t *testing.T) {
	got := ProjectID("group/sub/project")
	if got != "group%2Fsub%2Fproject" {
		t.Fatalf("ProjectID() = %q", got)
	}
}

func TestListMergeRequestsPagination(t *testing.T) {
	var seenToken string
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seenToken = r.Header.Get("PRIVATE-TOKEN")
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fproject/merge_requests" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}
		if r.URL.Query().Get("source_branch") != "feature" {
			t.Fatalf("source_branch = %q", r.URL.Query().Get("source_branch"))
		}
		switch r.URL.Query().Get("page") {
		case "1":
			return jsonResponse(200, `[{"iid":1,"title":"one"}]`, "2"), nil
		case "2":
			return jsonResponse(200, `[{"iid":2,"title":"two"}]`, ""), nil
		default:
			t.Fatalf("page = %q", r.URL.Query().Get("page"))
		}
		return nil, nil
	})

	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	mrs, err := client.ListMergeRequests(context.Background(), "group/project", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if seenToken != "token" {
		t.Fatalf("PRIVATE-TOKEN = %q", seenToken)
	}
	if len(mrs) != 2 {
		t.Fatalf("len = %d", len(mrs))
	}
}

func TestListMergeRequestsByBranches(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Query().Get("source_branch") != "feature-fix" {
			t.Fatalf("source_branch = %q", r.URL.Query().Get("source_branch"))
		}
		if r.URL.Query().Get("target_branch") != "feature" {
			t.Fatalf("target_branch = %q", r.URL.Query().Get("target_branch"))
		}
		return jsonResponse(200, `[{"iid":1}]`, ""), nil
	})
	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	mrs, err := client.ListMergeRequestsByBranches(context.Background(), "group/project", "feature-fix", "feature")
	if err != nil {
		t.Fatal(err)
	}
	if len(mrs) != 1 || mrs[0].IID != 1 {
		t.Fatalf("mrs = %+v", mrs)
	}
}

func TestCreateMergeRequest(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fproject/merge_requests" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Fatalf("PRIVATE-TOKEN = %q", r.Header.Get("PRIVATE-TOKEN"))
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Fatalf("Content-Type = %q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"source_branch":        "feature-fix",
			"target_branch":        "feature",
			"title":                "Fix comments",
			"description":          "Automated fixes",
			"remove_source_branch": "true",
			"allow_collaboration":  "true",
		}
		for key, value := range want {
			if got := r.Form.Get(key); got != value {
				t.Fatalf("%s = %q, want %q", key, got, value)
			}
		}
		return jsonResponse(201, `{"iid":3,"source_branch":"feature-fix","target_branch":"feature","title":"Fix comments"}`, ""), nil
	})
	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	mr, err := client.CreateMergeRequest(context.Background(), "group/project", CreateMergeRequestInput{
		SourceBranch:       "feature-fix",
		TargetBranch:       "feature",
		Title:              "Fix comments",
		Description:        "Automated fixes",
		RemoveSourceBranch: true,
		AllowCollaboration: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mr.IID != 3 {
		t.Fatalf("mr = %+v", mr)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(status int, body string, nextPage string) *http.Response {
	header := http.Header{}
	header.Set("Content-Type", "application/json")
	if nextPage != "" {
		header.Set("X-Next-Page", nextPage)
	}
	return &http.Response{
		StatusCode: status,
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
