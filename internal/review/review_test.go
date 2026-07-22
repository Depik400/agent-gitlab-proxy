package review

import (
	"context"
	"testing"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
	"github.com/Depik400/agent-gitlab-proxy/internal/gitlab"
)

type fakeClient struct {
	mrs         []gitlab.MergeRequest
	mr          gitlab.MergeRequest
	discussions []gitlab.Discussion
	diffs       []gitlab.Diff
	versions    []gitlab.MergeRequestVersion
	createdMR   gitlab.MergeRequest
	createInput gitlab.CreateMergeRequestInput
	note        gitlab.Note
	discussion  gitlab.Discussion
}

func (f fakeClient) ListMergeRequests(context.Context, string, string) ([]gitlab.MergeRequest, error) {
	return f.mrs, nil
}

func (f fakeClient) ListMergeRequestsByBranches(context.Context, string, string, string) ([]gitlab.MergeRequest, error) {
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

func (f fakeClient) ListMergeRequestVersions(context.Context, string, int) ([]gitlab.MergeRequestVersion, error) {
	return f.versions, nil
}

func (f fakeClient) CreateMergeRequest(context.Context, string, gitlab.CreateMergeRequestInput) (gitlab.MergeRequest, error) {
	return f.createdMR, nil
}

func (f fakeClient) UpdateMergeRequest(context.Context, string, int, gitlab.UpdateMergeRequestInput) (gitlab.MergeRequest, error) {
	return f.mr, nil
}

func (f fakeClient) AddMergeRequestNote(context.Context, string, int, string) (gitlab.Note, error) {
	return f.note, nil
}

func (f fakeClient) ReplyToMergeRequestDiscussion(context.Context, string, int, string, string) (gitlab.Note, error) {
	return f.note, nil
}

func (f fakeClient) UpdateMergeRequestNote(context.Context, string, int, int, string, string) (gitlab.Note, error) {
	return f.note, nil
}

func (f fakeClient) DeleteMergeRequestNote(context.Context, string, int, int, string) error {
	return nil
}

func (f fakeClient) CreateMergeRequestDiscussion(context.Context, string, int, gitlab.CreateMergeRequestDiscussionInput) (gitlab.Discussion, error) {
	return f.discussion, nil
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

func TestCreateMergeRequestCreatesWhenNoneExists(t *testing.T) {
	got, err := CreateMergeRequest(context.Background(), fakeClient{
		createdMR: gitlab.MergeRequest{IID: 10, SourceBranch: "feature-fix", TargetBranch: "feature"},
	}, "group/project", CreateMergeRequestInput{
		SourceBranch: "feature-fix",
		TargetBranch: "feature",
		Title:        "Fix comments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Created || got.MergeRequest.IID != 10 {
		t.Fatalf("result = %+v", got)
	}
}

func TestCreateMergeRequestReusesExisting(t *testing.T) {
	got, err := CreateMergeRequest(context.Background(), fakeClient{
		mrs: []gitlab.MergeRequest{{IID: 11, SourceBranch: "feature-fix", TargetBranch: "feature"}},
	}, "group/project", CreateMergeRequestInput{
		SourceBranch: "feature-fix",
		TargetBranch: "feature",
		Title:        "Fix comments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Created || got.MergeRequest.IID != 11 {
		t.Fatalf("result = %+v", got)
	}
}

func TestCreateMergeRequestAmbiguous(t *testing.T) {
	_, err := CreateMergeRequest(context.Background(), fakeClient{
		mrs: []gitlab.MergeRequest{{IID: 1}, {IID: 2}},
	}, "group/project", CreateMergeRequestInput{
		SourceBranch: "feature-fix",
		TargetBranch: "feature",
		Title:        "Fix comments",
	})
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

func TestAddMergeRequestComment(t *testing.T) {
	got, err := AddMergeRequestComment(context.Background(), fakeClient{
		mr:   gitlab.MergeRequest{IID: 12, WebURL: "https://gitlab.example.com/mr/12"},
		note: gitlab.Note{ID: 99, Body: "Review comment"},
	}, "group/project", MRSelector{MRIID: 12}, "Review comment")
	if err != nil {
		t.Fatal(err)
	}
	if got.MergeRequest.IID != 12 || got.Note.ID != 99 {
		t.Fatalf("result = %+v", got)
	}
}

func TestReplyToMergeRequestDiscussion(t *testing.T) {
	got, err := ReplyToMergeRequestDiscussion(context.Background(), fakeClient{
		mr:   gitlab.MergeRequest{IID: 12, WebURL: "https://gitlab.example.com/mr/12"},
		note: gitlab.Note{ID: 100, Body: "**Done**"},
	}, "group/project", MRSelector{MRIID: 12}, "discussion-1", "**Done**")
	if err != nil {
		t.Fatal(err)
	}
	if got.MergeRequest.IID != 12 || got.DiscussionID != "discussion-1" || got.Note.ID != 100 {
		t.Fatalf("result = %+v", got)
	}
}

func TestUpdateMergeRequest(t *testing.T) {
	title := "Updated title"
	got, err := UpdateMergeRequest(context.Background(), fakeClient{
		mr: gitlab.MergeRequest{IID: 12, Title: title},
	}, "group/project", MRSelector{MRIID: 12}, UpdateMergeRequestInput{Title: &title})
	if err != nil {
		t.Fatal(err)
	}
	if got.MergeRequest.Title != title {
		t.Fatalf("result = %+v", got)
	}
}

func TestUpdateAndDeleteMergeRequestDiscussionComment(t *testing.T) {
	client := fakeClient{
		mr:   gitlab.MergeRequest{IID: 12},
		note: gitlab.Note{ID: 99, Body: "**Updated**"},
	}
	updated, err := UpdateMergeRequestComment(context.Background(), client, "group/project", MRSelector{MRIID: 12}, "discussion-1", 99, "**Updated**")
	if err != nil {
		t.Fatal(err)
	}
	if updated.DiscussionID != "discussion-1" || updated.Note.ID != 99 {
		t.Fatalf("updated = %+v", updated)
	}
	deleted, err := DeleteMergeRequestComment(context.Background(), client, "group/project", MRSelector{MRIID: 12}, "discussion-1", 99)
	if err != nil {
		t.Fatal(err)
	}
	if !deleted.Deleted || deleted.NoteID != 99 || deleted.DiscussionID != "discussion-1" {
		t.Fatalf("deleted = %+v", deleted)
	}
}

func TestAddMergeRequestThread(t *testing.T) {
	got, err := AddMergeRequestThread(context.Background(), fakeClient{
		mr: gitlab.MergeRequest{IID: 12, WebURL: "https://gitlab.example.com/mr/12"},
		versions: []gitlab.MergeRequestVersion{{
			ID:             1,
			BaseCommitSHA:  "base",
			StartCommitSHA: "start",
			HeadCommitSHA:  "head",
		}},
		discussion: gitlab.Discussion{ID: "discussion-1"},
	}, "group/project", MRSelector{MRIID: 12}, AddMergeRequestThreadInput{
		Body:    "Review comment",
		File:    "main.go",
		NewLine: 42,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.MergeRequest.IID != 12 || got.Discussion.ID != "discussion-1" {
		t.Fatalf("result = %+v", got)
	}
	if got.Position["new_path"] != "main.go" || got.Position["new_line"] != 42 {
		t.Fatalf("position = %+v", got.Position)
	}
}
