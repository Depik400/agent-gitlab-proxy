package output

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/Depik400/agent-gitlab-proxy/internal/apperr"
)

func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func Error(w io.Writer, err error) {
	if app, ok := err.(*apperr.Error); ok {
		_ = JSON(w, app)
		return
	}
	_ = JSON(w, apperr.New(apperr.CodeGitLabAPI, err.Error(), nil))
}

func Usage(w io.Writer, msg string) {
	_, _ = fmt.Fprintln(w, msg)
}
