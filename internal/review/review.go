package review

import (
	"context"
	"strconv"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
	"github.com/Depik400/agent-gitlab-proxy/internal/gitlab"
)

type GitLabClient interface {
	ListMergeRequests(ctx context.Context, repo string, branch string) ([]gitlab.MergeRequest, error)
	ListMergeRequestsByBranches(ctx context.Context, repo string, sourceBranch string, targetBranch string) ([]gitlab.MergeRequest, error)
	GetMergeRequest(ctx context.Context, repo string, iid int) (gitlab.MergeRequest, error)
	ListDiscussions(ctx context.Context, repo string, iid int) ([]gitlab.Discussion, error)
	ListMergeRequestVersions(ctx context.Context, repo string, iid int) ([]gitlab.MergeRequestVersion, error)
	ListDiffs(ctx context.Context, repo string, iid int) ([]gitlab.Diff, error)
	CreateMergeRequest(ctx context.Context, repo string, input gitlab.CreateMergeRequestInput) (gitlab.MergeRequest, error)
	AddMergeRequestNote(ctx context.Context, repo string, iid int, body string) (gitlab.Note, error)
	ReplyToMergeRequestDiscussion(ctx context.Context, repo string, iid int, discussionID string, body string) (gitlab.Note, error)
	CreateMergeRequestDiscussion(ctx context.Context, repo string, iid int, input gitlab.CreateMergeRequestDiscussionInput) (gitlab.Discussion, error)
}

type MRSelector struct {
	Branch string
	MRIID  int
}

type Candidate struct {
	IID          int    `json:"iid"`
	Title        string `json:"title"`
	WebURL       string `json:"web_url"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	UpdatedAt    string `json:"updated_at"`
}

type Comment struct {
	Repo           string              `json:"repo"`
	MRIID          int                 `json:"mr_iid"`
	MRWebURL       string              `json:"mr_web_url"`
	DiscussionID   string              `json:"discussion_id"`
	NoteID         int                 `json:"note_id"`
	AuthorUsername string              `json:"author_username"`
	AuthorName     string              `json:"author_name"`
	Body           string              `json:"body"`
	CreatedAt      string              `json:"created_at"`
	UpdatedAt      string              `json:"updated_at"`
	System         bool                `json:"system"`
	Resolvable     bool                `json:"resolvable"`
	Resolved       bool                `json:"resolved"`
	FilePath       string              `json:"file_path,omitempty"`
	OldLine        *int                `json:"old_line,omitempty"`
	NewLine        *int                `json:"new_line,omitempty"`
	Suggestions    []gitlab.Suggestion `json:"suggestions,omitempty"`
}

type Context struct {
	Host         string              `json:"host"`
	Repo         string              `json:"repo"`
	MergeRequest gitlab.MergeRequest `json:"merge_request"`
	Diffs        []gitlab.Diff       `json:"diffs"`
	Comments     []Comment           `json:"comments"`
}

type CreateMergeRequestInput struct {
	SourceBranch       string
	TargetBranch       string
	Title              string
	Description        string
	RemoveSourceBranch bool
	AllowCollaboration bool
}

type CreateMergeRequestResult struct {
	Created      bool                `json:"created"`
	Repo         string              `json:"repo"`
	MergeRequest gitlab.MergeRequest `json:"merge_request"`
}

type AddMergeRequestCommentResult struct {
	Repo         string              `json:"repo"`
	MergeRequest gitlab.MergeRequest `json:"merge_request"`
	Note         gitlab.Note         `json:"note"`
}

type ReplyToMergeRequestDiscussionResult struct {
	Repo         string              `json:"repo"`
	MergeRequest gitlab.MergeRequest `json:"merge_request"`
	DiscussionID string              `json:"discussion_id"`
	Note         gitlab.Note         `json:"note"`
}

type AddMergeRequestThreadInput struct {
	Body    string
	File    string
	OldFile string
	NewLine int
	OldLine int
}

type AddMergeRequestThreadResult struct {
	Repo         string              `json:"repo"`
	MergeRequest gitlab.MergeRequest `json:"merge_request"`
	Discussion   gitlab.Discussion   `json:"discussion"`
	Position     map[string]any      `json:"position"`
}

func ResolveMR(ctx context.Context, client GitLabClient, repo string, selector MRSelector) (gitlab.MergeRequest, error) {
	if selector.MRIID > 0 {
		return client.GetMergeRequest(ctx, repo, selector.MRIID)
	}
	if selector.Branch == "" {
		return gitlab.MergeRequest{}, apperr.New(apperr.CodeInvalidArgs, "either --branch or --mr-iid is required", nil)
	}
	mrs, err := client.ListMergeRequests(ctx, repo, selector.Branch)
	if err != nil {
		return gitlab.MergeRequest{}, err
	}
	switch len(mrs) {
	case 0:
		return gitlab.MergeRequest{}, apperr.New(apperr.CodeNotFound, "no opened merge request found for branch", map[string]string{"repo": repo, "branch": selector.Branch})
	case 1:
		return mrs[0], nil
	default:
		candidates := make([]Candidate, 0, len(mrs))
		for _, mr := range mrs {
			candidates = append(candidates, Candidate{
				IID:          mr.IID,
				Title:        mr.Title,
				WebURL:       mr.WebURL,
				SourceBranch: mr.SourceBranch,
				TargetBranch: mr.TargetBranch,
				UpdatedAt:    mr.UpdatedAt,
			})
		}
		return gitlab.MergeRequest{}, apperr.New(apperr.CodeAmbiguousMR, "multiple opened merge requests found; pass --mr-iid", map[string]any{"repo": repo, "branch": selector.Branch, "candidates": candidates})
	}
}

func Comments(ctx context.Context, client GitLabClient, repo string, selector MRSelector, includeResolved bool) ([]Comment, error) {
	mr, err := ResolveMR(ctx, client, repo, selector)
	if err != nil {
		return nil, err
	}
	discussions, err := client.ListDiscussions(ctx, repo, mr.IID)
	if err != nil {
		return nil, err
	}
	return FlattenComments(repo, mr, discussions, includeResolved), nil
}

func MRContext(ctx context.Context, client GitLabClient, hostName, repo string, selector MRSelector, includeResolved bool) (Context, error) {
	mr, err := ResolveMR(ctx, client, repo, selector)
	if err != nil {
		return Context{}, err
	}
	discussions, err := client.ListDiscussions(ctx, repo, mr.IID)
	if err != nil {
		return Context{}, err
	}
	diffs, err := client.ListDiffs(ctx, repo, mr.IID)
	if err != nil {
		return Context{}, err
	}
	return Context{
		Host:         hostName,
		Repo:         repo,
		MergeRequest: mr,
		Diffs:        diffs,
		Comments:     FlattenComments(repo, mr, discussions, includeResolved),
	}, nil
}

func CreateMergeRequest(ctx context.Context, client GitLabClient, repo string, input CreateMergeRequestInput) (CreateMergeRequestResult, error) {
	mrs, err := client.ListMergeRequestsByBranches(ctx, repo, input.SourceBranch, input.TargetBranch)
	if err != nil {
		return CreateMergeRequestResult{}, err
	}
	switch len(mrs) {
	case 0:
		mr, err := client.CreateMergeRequest(ctx, repo, gitlab.CreateMergeRequestInput{
			SourceBranch:       input.SourceBranch,
			TargetBranch:       input.TargetBranch,
			Title:              input.Title,
			Description:        input.Description,
			RemoveSourceBranch: input.RemoveSourceBranch,
			AllowCollaboration: input.AllowCollaboration,
		})
		if err != nil {
			return CreateMergeRequestResult{}, err
		}
		return CreateMergeRequestResult{Created: true, Repo: repo, MergeRequest: mr}, nil
	case 1:
		return CreateMergeRequestResult{Created: false, Repo: repo, MergeRequest: mrs[0]}, nil
	default:
		candidates := make([]Candidate, 0, len(mrs))
		for _, mr := range mrs {
			candidates = append(candidates, Candidate{
				IID:          mr.IID,
				Title:        mr.Title,
				WebURL:       mr.WebURL,
				SourceBranch: mr.SourceBranch,
				TargetBranch: mr.TargetBranch,
				UpdatedAt:    mr.UpdatedAt,
			})
		}
		return CreateMergeRequestResult{}, apperr.New(apperr.CodeAmbiguousMR, "multiple opened merge requests found for source and target branches", map[string]any{
			"repo":          repo,
			"source_branch": input.SourceBranch,
			"target_branch": input.TargetBranch,
			"candidates":    candidates,
		})
	}
}

func AddMergeRequestComment(ctx context.Context, client GitLabClient, repo string, selector MRSelector, body string) (AddMergeRequestCommentResult, error) {
	if body == "" {
		return AddMergeRequestCommentResult{}, apperr.New(apperr.CodeInvalidArgs, "--body is required", nil)
	}
	mr, err := ResolveMR(ctx, client, repo, selector)
	if err != nil {
		return AddMergeRequestCommentResult{}, err
	}
	note, err := client.AddMergeRequestNote(ctx, repo, mr.IID, body)
	if err != nil {
		return AddMergeRequestCommentResult{}, err
	}
	return AddMergeRequestCommentResult{Repo: repo, MergeRequest: mr, Note: note}, nil
}

func ReplyToMergeRequestDiscussion(ctx context.Context, client GitLabClient, repo string, selector MRSelector, discussionID, body string) (ReplyToMergeRequestDiscussionResult, error) {
	if discussionID == "" {
		return ReplyToMergeRequestDiscussionResult{}, apperr.New(apperr.CodeInvalidArgs, "--discussion-id is required", nil)
	}
	if body == "" {
		return ReplyToMergeRequestDiscussionResult{}, apperr.New(apperr.CodeInvalidArgs, "--body is required", nil)
	}
	mr, err := ResolveMR(ctx, client, repo, selector)
	if err != nil {
		return ReplyToMergeRequestDiscussionResult{}, err
	}
	note, err := client.ReplyToMergeRequestDiscussion(ctx, repo, mr.IID, discussionID, body)
	if err != nil {
		return ReplyToMergeRequestDiscussionResult{}, err
	}
	return ReplyToMergeRequestDiscussionResult{Repo: repo, MergeRequest: mr, DiscussionID: discussionID, Note: note}, nil
}

func AddMergeRequestThread(ctx context.Context, client GitLabClient, repo string, selector MRSelector, input AddMergeRequestThreadInput) (AddMergeRequestThreadResult, error) {
	if input.Body == "" {
		return AddMergeRequestThreadResult{}, apperr.New(apperr.CodeInvalidArgs, "--body is required", nil)
	}
	if input.File == "" {
		return AddMergeRequestThreadResult{}, apperr.New(apperr.CodeInvalidArgs, "--file is required", nil)
	}
	if input.NewLine <= 0 && input.OldLine <= 0 {
		return AddMergeRequestThreadResult{}, apperr.New(apperr.CodeInvalidArgs, "at least one of --new-line or --old-line is required", nil)
	}
	oldFile := input.OldFile
	if oldFile == "" {
		oldFile = input.File
	}
	mr, err := ResolveMR(ctx, client, repo, selector)
	if err != nil {
		return AddMergeRequestThreadResult{}, err
	}
	versions, err := client.ListMergeRequestVersions(ctx, repo, mr.IID)
	if err != nil {
		return AddMergeRequestThreadResult{}, err
	}
	if len(versions) == 0 {
		return AddMergeRequestThreadResult{}, apperr.New(apperr.CodeNotFound, "merge request versions not found", map[string]any{"repo": repo, "mr_iid": mr.IID})
	}
	version := versions[0]
	if version.BaseCommitSHA == "" || version.StartCommitSHA == "" || version.HeadCommitSHA == "" {
		return AddMergeRequestThreadResult{}, apperr.New(apperr.CodeGitLabAPI, "latest merge request version is missing position SHAs", map[string]any{"repo": repo, "mr_iid": mr.IID, "version_id": version.ID})
	}
	discussion, err := client.CreateMergeRequestDiscussion(ctx, repo, mr.IID, gitlab.CreateMergeRequestDiscussionInput{
		Body:     input.Body,
		BaseSHA:  version.BaseCommitSHA,
		StartSHA: version.StartCommitSHA,
		HeadSHA:  version.HeadCommitSHA,
		OldPath:  oldFile,
		NewPath:  input.File,
		OldLine:  input.OldLine,
		NewLine:  input.NewLine,
	})
	if err != nil {
		return AddMergeRequestThreadResult{}, err
	}
	return AddMergeRequestThreadResult{
		Repo:         repo,
		MergeRequest: mr,
		Discussion:   discussion,
		Position: map[string]any{
			"base_sha":  version.BaseCommitSHA,
			"start_sha": version.StartCommitSHA,
			"head_sha":  version.HeadCommitSHA,
			"old_path":  oldFile,
			"new_path":  input.File,
			"old_line":  input.OldLine,
			"new_line":  input.NewLine,
		},
	}, nil
}

func FlattenComments(repo string, mr gitlab.MergeRequest, discussions []gitlab.Discussion, includeResolved bool) []Comment {
	var out []Comment
	for _, discussion := range discussions {
		for _, note := range discussion.Notes {
			if !includeResolved && (!note.Resolvable || note.Resolved) {
				continue
			}
			comment := Comment{
				Repo:           repo,
				MRIID:          mr.IID,
				MRWebURL:       mr.WebURL,
				DiscussionID:   discussion.ID,
				NoteID:         note.ID,
				AuthorUsername: note.Author.Username,
				AuthorName:     note.Author.Name,
				Body:           note.Body,
				CreatedAt:      note.CreatedAt,
				UpdatedAt:      note.UpdatedAt,
				System:         note.System,
				Resolvable:     note.Resolvable,
				Resolved:       note.Resolved,
				Suggestions:    note.Suggestions,
			}
			if note.Position != nil {
				comment.FilePath = note.Position.NewPath
				if comment.FilePath == "" {
					comment.FilePath = note.Position.OldPath
				}
				comment.NewLine = note.Position.NewLine
				comment.OldLine = note.Position.OldLine
			}
			out = append(out, comment)
		}
	}
	return out
}

func ParseMRIID(raw string) (int, error) {
	iid, err := strconv.Atoi(raw)
	if err != nil || iid <= 0 {
		return 0, apperr.New(apperr.CodeInvalidArgs, "--mr-iid must be a positive integer", map[string]string{"mr_iid": raw})
	}
	return iid, nil
}
