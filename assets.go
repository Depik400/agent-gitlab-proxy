package assets

import _ "embed"

//go:embed agents/skills/gitlab-review-comments/SKILL.md
var GitLabReviewCommentsSkill string

//go:embed agents/skills/gitlab/SKILL.md
var GitLabSkill string

//go:embed agents/skills/gitlab-review-branch/SKILL.md
var ReviewBranchSkill string

func Skills() map[string]string {
	return map[string]string{
		"gitlab-review-comments": GitLabReviewCommentsSkill,
		"gitlab":                 GitLabSkill,
		"gitlab-review-branch":   ReviewBranchSkill,
	}
}
