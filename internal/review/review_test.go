package review

import (
	"context"
	"testing"

	"gitlab-proxy/internal/apperr"
	"gitlab-proxy/internal/gitlab"
)

type fakeClient struct {
	mrs         []gitlab.MergeRequest
	mr          gitlab.MergeRequest
	discussions []gitlab.Discussion
	diffs       []gitlab.Diff
}

func (f fakeClient) ListMergeRequests(context.Context, string, string) ([]gitlab.MergeRequest, error) {
	return f.mrs, nil
}

func (f fakeClient) GetMergeRequest(context.Context, string, int) (gitlab.MergeRequest, error) {
	return f.mr, nil
}

func (f fakeClient) ListDiscussions(context.Context, string, int) ([]gitlab.Discussion, error) {
	return f.discussions, nil
}

func (f fakeClient) ListDiffs(context.Context, string, int) ([]gitlab.Diff, error) {
	return f.diffs, nil
}

func TestResolveMRAmbiguous(t *testing.T) {
	_, err := ResolveMR(context.Background(), fakeClient{
		mrs: []gitlab.MergeRequest{{IID: 1}, {IID: 2}},
	}, "group/project", MRSelector{Branch: "feature"})
	if err == nil {
		t.Fatal("expected error")
	}
	app, ok := err.(*apperr.Error)
	if !ok {
		t.Fatalf("error type = %T", err)
	}
	if app.Code != apperr.CodeAmbiguousMR {
		t.Fatalf("code = %q", app.Code)
	}
}

func TestFlattenCommentsFiltersResolvedByDefault(t *testing.T) {
	line := 42
	mr := gitlab.MergeRequest{IID: 7, WebURL: "https://gitlab.example.com/mr/7"}
	discussions := []gitlab.Discussion{{
		ID: "discussion-1",
		Notes: []gitlab.Note{
			{ID: 1, Body: "keep", Resolvable: true, Resolved: false, Author: gitlab.Author{Username: "alice", Name: "Alice"}, Position: &gitlab.Position{NewPath: "main.go", NewLine: &line}},
			{ID: 2, Body: "drop resolved", Resolvable: true, Resolved: true},
			{ID: 3, Body: "drop non-resolvable", Resolvable: false, Resolved: false},
		},
	}}
	got := FlattenComments("group/project", mr, discussions, false)
	if len(got) != 1 {
		t.Fatalf("len = %d", len(got))
	}
	if got[0].Body != "keep" || got[0].FilePath != "main.go" || *got[0].NewLine != 42 {
		t.Fatalf("comment = %+v", got[0])
	}
}

func TestFlattenCommentsIncludeResolved(t *testing.T) {
	mr := gitlab.MergeRequest{IID: 7}
	discussions := []gitlab.Discussion{{
		ID: "discussion-1",
		Notes: []gitlab.Note{
			{ID: 1, Resolvable: true, Resolved: false},
			{ID: 2, Resolvable: true, Resolved: true},
			{ID: 3, Resolvable: false, Resolved: false},
		},
	}}
	got := FlattenComments("group/project", mr, discussions, true)
	if len(got) != 3 {
		t.Fatalf("len = %d", len(got))
	}
}
