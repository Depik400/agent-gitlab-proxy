package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
)

type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func NewClientWithHTTP(baseURL, token string, httpClient *http.Client) *Client {
	c := NewClient(baseURL, token)
	c.httpClient = httpClient
	return c
}

type User struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type MergeRequest struct {
	ID           int    `json:"id"`
	IID          int    `json:"iid"`
	ProjectID    int    `json:"project_id"`
	Title        string `json:"title"`
	State        string `json:"state"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
	WebURL       string `json:"web_url"`
	UpdatedAt    string `json:"updated_at"`
	Description  string `json:"description,omitempty"`
}

type Discussion struct {
	ID    string `json:"id"`
	Notes []Note `json:"notes"`
}

type MergeRequestVersion struct {
	ID             int    `json:"id"`
	HeadCommitSHA  string `json:"head_commit_sha"`
	BaseCommitSHA  string `json:"base_commit_sha"`
	StartCommitSHA string `json:"start_commit_sha"`
	CreatedAt      string `json:"created_at"`
	State          string `json:"state"`
	RealSize       string `json:"real_size"`
}

type Note struct {
	ID          int          `json:"id"`
	Body        string       `json:"body"`
	Author      Author       `json:"author"`
	CreatedAt   string       `json:"created_at"`
	UpdatedAt   string       `json:"updated_at"`
	System      bool         `json:"system"`
	Resolvable  bool         `json:"resolvable"`
	Resolved    bool         `json:"resolved"`
	Position    *Position    `json:"position"`
	Suggestions []Suggestion `json:"suggestions"`
}

type Author struct {
	ID       int    `json:"id"`
	Username string `json:"username"`
	Name     string `json:"name"`
}

type Position struct {
	NewPath string `json:"new_path"`
	OldPath string `json:"old_path"`
	NewLine *int   `json:"new_line"`
	OldLine *int   `json:"old_line"`
}

type Suggestion struct {
	ID          int    `json:"id"`
	FromLine    int    `json:"from_line"`
	ToLine      int    `json:"to_line"`
	Appliable   bool   `json:"appliable"`
	Applied     bool   `json:"applied"`
	FromContent string `json:"from_content"`
	ToContent   string `json:"to_content"`
}

type Diff struct {
	OldPath     string `json:"old_path"`
	NewPath     string `json:"new_path"`
	AMode       string `json:"a_mode"`
	BMode       string `json:"b_mode"`
	Diff        string `json:"diff"`
	NewFile     bool   `json:"new_file"`
	RenamedFile bool   `json:"renamed_file"`
	DeletedFile bool   `json:"deleted_file"`
}

type CreateMergeRequestInput struct {
	SourceBranch       string
	TargetBranch       string
	Title              string
	Description        string
	RemoveSourceBranch bool
	AllowCollaboration bool
}

type UpdateMergeRequestInput struct {
	Title       *string
	Description *string
}

type CreateMergeRequestDiscussionInput struct {
	Body     string
	BaseSHA  string
	StartSHA string
	HeadSHA  string
	OldPath  string
	NewPath  string
	OldLine  int
	NewLine  int
}

func ProjectID(path string) string {
	return url.PathEscape(path)
}

func (c *Client) VerifyToken(ctx context.Context) error {
	var user User
	if err := c.get(ctx, "/user", nil, &user); err != nil {
		return err
	}
	return nil
}

func (c *Client) ListMergeRequests(ctx context.Context, repo string, branch string) ([]MergeRequest, error) {
	values := url.Values{}
	values.Set("state", "opened")
	values.Set("source_branch", branch)
	return c.listMergeRequests(ctx, repo, values)
}

func (c *Client) ListMergeRequestsByBranches(ctx context.Context, repo string, sourceBranch string, targetBranch string) ([]MergeRequest, error) {
	values := url.Values{}
	values.Set("state", "opened")
	values.Set("source_branch", sourceBranch)
	values.Set("target_branch", targetBranch)
	return c.listMergeRequests(ctx, repo, values)
}

func (c *Client) listMergeRequests(ctx context.Context, repo string, values url.Values) ([]MergeRequest, error) {
	values.Set("per_page", "100")
	var out []MergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests", ProjectID(repo))
	if err := c.getPaged(ctx, path, values, func(data []byte) error {
		var page []MergeRequest
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) CreateMergeRequest(ctx context.Context, repo string, input CreateMergeRequestInput) (MergeRequest, error) {
	values := url.Values{}
	values.Set("source_branch", input.SourceBranch)
	values.Set("target_branch", input.TargetBranch)
	values.Set("title", input.Title)
	if input.Description != "" {
		values.Set("description", input.Description)
	}
	if input.RemoveSourceBranch {
		values.Set("remove_source_branch", "true")
	}
	if input.AllowCollaboration {
		values.Set("allow_collaboration", "true")
	}
	var mr MergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests", ProjectID(repo))
	if err := c.postForm(ctx, path, values, &mr); err != nil {
		return MergeRequest{}, err
	}
	return mr, nil
}

func (c *Client) GetMergeRequest(ctx context.Context, repo string, iid int) (MergeRequest, error) {
	var mr MergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests/%d", ProjectID(repo), iid)
	if err := c.get(ctx, path, nil, &mr); err != nil {
		return MergeRequest{}, err
	}
	return mr, nil
}

func (c *Client) UpdateMergeRequest(ctx context.Context, repo string, iid int, input UpdateMergeRequestInput) (MergeRequest, error) {
	values := url.Values{}
	if input.Title != nil {
		values.Set("title", *input.Title)
	}
	if input.Description != nil {
		values.Set("description", *input.Description)
	}
	var mr MergeRequest
	path := fmt.Sprintf("/projects/%s/merge_requests/%d", ProjectID(repo), iid)
	if err := c.putForm(ctx, path, values, &mr); err != nil {
		return MergeRequest{}, err
	}
	return mr, nil
}

func (c *Client) ListDiscussions(ctx context.Context, repo string, iid int) ([]Discussion, error) {
	values := url.Values{}
	values.Set("per_page", "100")
	var out []Discussion
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions", ProjectID(repo), iid)
	if err := c.getPaged(ctx, path, values, func(data []byte) error {
		var page []Discussion
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListMergeRequestVersions(ctx context.Context, repo string, iid int) ([]MergeRequestVersion, error) {
	values := url.Values{}
	values.Set("per_page", "100")
	var out []MergeRequestVersion
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/versions", ProjectID(repo), iid)
	if err := c.getPaged(ctx, path, values, func(data []byte) error {
		var page []MergeRequestVersion
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) ListDiffs(ctx context.Context, repo string, iid int) ([]Diff, error) {
	values := url.Values{}
	values.Set("per_page", "100")
	var out []Diff
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/diffs", ProjectID(repo), iid)
	if err := c.getPaged(ctx, path, values, func(data []byte) error {
		var page []Diff
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		out = append(out, page...)
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) AddMergeRequestNote(ctx context.Context, repo string, iid int, body string) (Note, error) {
	values := url.Values{}
	values.Set("body", body)
	var note Note
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/notes", ProjectID(repo), iid)
	if err := c.postForm(ctx, path, values, &note); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (c *Client) ReplyToMergeRequestDiscussion(ctx context.Context, repo string, iid int, discussionID string, body string) (Note, error) {
	values := url.Values{}
	values.Set("body", body)
	var note Note
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s/notes", ProjectID(repo), iid, url.PathEscape(discussionID))
	if err := c.postForm(ctx, path, values, &note); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (c *Client) UpdateMergeRequestNote(ctx context.Context, repo string, iid, noteID int, discussionID, body string) (Note, error) {
	values := url.Values{}
	values.Set("body", body)
	var note Note
	path := mergeRequestNotePath(repo, iid, discussionID, noteID)
	if err := c.putForm(ctx, path, values, &note); err != nil {
		return Note{}, err
	}
	return note, nil
}

func (c *Client) DeleteMergeRequestNote(ctx context.Context, repo string, iid, noteID int, discussionID string) error {
	_, _, err := c.request(ctx, http.MethodDelete, mergeRequestNotePath(repo, iid, discussionID, noteID), nil, nil, "")
	return err
}

func mergeRequestNotePath(repo string, iid int, discussionID string, noteID int) string {
	if discussionID != "" {
		return fmt.Sprintf("/projects/%s/merge_requests/%d/discussions/%s/notes/%d", ProjectID(repo), iid, url.PathEscape(discussionID), noteID)
	}
	return fmt.Sprintf("/projects/%s/merge_requests/%d/notes/%d", ProjectID(repo), iid, noteID)
}

func (c *Client) CreateMergeRequestDiscussion(ctx context.Context, repo string, iid int, input CreateMergeRequestDiscussionInput) (Discussion, error) {
	values := url.Values{}
	values.Set("body", input.Body)
	values.Set("position[position_type]", "text")
	values.Set("position[base_sha]", input.BaseSHA)
	values.Set("position[start_sha]", input.StartSHA)
	values.Set("position[head_sha]", input.HeadSHA)
	values.Set("position[old_path]", input.OldPath)
	values.Set("position[new_path]", input.NewPath)
	if input.OldLine > 0 {
		values.Set("position[old_line]", strconv.Itoa(input.OldLine))
	}
	if input.NewLine > 0 {
		values.Set("position[new_line]", strconv.Itoa(input.NewLine))
	}
	var discussion Discussion
	path := fmt.Sprintf("/projects/%s/merge_requests/%d/discussions", ProjectID(repo), iid)
	if err := c.postForm(ctx, path, values, &discussion); err != nil {
		return Discussion{}, err
	}
	return discussion, nil
}

func (c *Client) get(ctx context.Context, path string, values url.Values, dst any) error {
	data, _, err := c.request(ctx, http.MethodGet, path, values, nil, "")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return apperr.Wrap(apperr.CodeGitLabAPI, "decode GitLab response", err, nil)
	}
	return nil
}

func (c *Client) getPaged(ctx context.Context, path string, values url.Values, consume func([]byte) error) error {
	page := 1
	for {
		q := cloneValues(values)
		q.Set("page", strconv.Itoa(page))
		data, next, err := c.request(ctx, http.MethodGet, path, q, nil, "")
		if err != nil {
			return err
		}
		if err := consume(data); err != nil {
			return apperr.Wrap(apperr.CodeGitLabAPI, "decode GitLab response", err, nil)
		}
		if next == "" {
			return nil
		}
		n, err := strconv.Atoi(next)
		if err != nil {
			return apperr.Wrap(apperr.CodeGitLabAPI, "invalid GitLab pagination header", err, map[string]string{"x_next_page": next})
		}
		page = n
	}
}

func (c *Client) postForm(ctx context.Context, path string, values url.Values, dst any) error {
	return c.form(ctx, http.MethodPost, path, values, dst)
}

func (c *Client) putForm(ctx context.Context, path string, values url.Values, dst any) error {
	return c.form(ctx, http.MethodPut, path, values, dst)
}

func (c *Client) form(ctx context.Context, method, path string, values url.Values, dst any) error {
	body := bytes.NewBufferString(values.Encode())
	data, _, err := c.request(ctx, method, path, nil, body, "application/x-www-form-urlencoded")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return apperr.Wrap(apperr.CodeGitLabAPI, "decode GitLab response", err, nil)
	}
	return nil
}

func (c *Client) request(ctx context.Context, method string, path string, values url.Values, bodyReader io.Reader, contentType string) ([]byte, string, error) {
	u, err := url.Parse(c.baseURL + "/api/v4" + path)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeGitLabAPI, "build GitLab request url", err, nil)
	}
	if values != nil {
		u.RawQuery = values.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), bodyReader)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeGitLabAPI, "build GitLab request", err, nil)
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)
	req.Header.Set("Accept", "application/json")
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeGitLabAPI, "call GitLab API", err, map[string]string{
			"method": method,
			"url":    u.Redacted(),
			"error":  err.Error(),
		})
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", apperr.Wrap(apperr.CodeGitLabAPI, "read GitLab response", err, nil)
	}
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, "", apperr.New(apperr.CodeAuth, "GitLab authentication failed", map[string]any{"status": resp.StatusCode, "body": string(body)})
	}
	if resp.StatusCode == http.StatusNotFound {
		return nil, "", apperr.New(apperr.CodeNotFound, "GitLab resource not found", map[string]any{"status": resp.StatusCode, "body": string(body)})
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", apperr.New(apperr.CodeGitLabAPI, "GitLab API returned non-success status", map[string]any{"status": resp.StatusCode, "body": string(body)})
	}
	return body, resp.Header.Get("X-Next-Page"), nil
}

func cloneValues(values url.Values) url.Values {
	out := url.Values{}
	for key, vals := range values {
		out[key] = append([]string(nil), vals...)
	}
	return out
}
