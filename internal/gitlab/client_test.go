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

func TestAddMergeRequestNote(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fproject/merge_requests/3/notes" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("body"); got != "Review comment" {
			t.Fatalf("body = %q", got)
		}
		return jsonResponse(201, `{"id":99,"body":"Review comment"}`, ""), nil
	})
	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	note, err := client.AddMergeRequestNote(context.Background(), "group/project", 3, "Review comment")
	if err != nil {
		t.Fatal(err)
	}
	if note.ID != 99 || note.Body != "Review comment" {
		t.Fatalf("note = %+v", note)
	}
}

func TestListMergeRequestVersions(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fproject/merge_requests/3/versions" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}
		return jsonResponse(200, `[{"id":7,"base_commit_sha":"base","start_commit_sha":"start","head_commit_sha":"head"}]`, ""), nil
	})
	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	versions, err := client.ListMergeRequestVersions(context.Background(), "group/project", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].HeadCommitSHA != "head" {
		t.Fatalf("versions = %+v", versions)
	}
}

func TestCreateMergeRequestDiscussion(t *testing.T) {
	transport := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %q", r.Method)
		}
		if r.URL.EscapedPath() != "/api/v4/projects/group%2Fproject/merge_requests/3/discussions" {
			t.Fatalf("path = %q", r.URL.EscapedPath())
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		want := map[string]string{
			"body":                    "Review comment",
			"position[position_type]": "text",
			"position[base_sha]":      "base",
			"position[start_sha]":     "start",
			"position[head_sha]":      "head",
			"position[old_path]":      "old.go",
			"position[new_path]":      "new.go",
			"position[old_line]":      "40",
			"position[new_line]":      "42",
		}
		for key, value := range want {
			if got := r.Form.Get(key); got != value {
				t.Fatalf("%s = %q, want %q", key, got, value)
			}
		}
		return jsonResponse(201, `{"id":"discussion-1","notes":[{"id":99,"body":"Review comment"}]}`, ""), nil
	})
	client := NewClientWithHTTP("https://gitlab.example.com", "token", &http.Client{Transport: transport})
	discussion, err := client.CreateMergeRequestDiscussion(context.Background(), "group/project", 3, CreateMergeRequestDiscussionInput{
		Body:     "Review comment",
		BaseSHA:  "base",
		StartSHA: "start",
		HeadSHA:  "head",
		OldPath:  "old.go",
		NewPath:  "new.go",
		OldLine:  40,
		NewLine:  42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if discussion.ID != "discussion-1" || len(discussion.Notes) != 1 {
		t.Fatalf("discussion = %+v", discussion)
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
