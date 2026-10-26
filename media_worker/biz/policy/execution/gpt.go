package execution

import (
	"errors"
	"regexp"
	"strings"
)

func ValidateGPTParameters(size, model, project string) error {
	if !map[string]bool{"auto": true, "1024x1024": true, "1536x1024": true, "1024x1536": true}[size] || strings.TrimSpace(model) == "" {
		return errors.New("explicit supported GPT size hint and web model required")
	}
	if project != "" && !regexp.MustCompile(`^https://chatgpt\.com/g/g-p-[A-Za-z0-9-]+/project$`).MatchString(project) {
		return errors.New("GPT project must be an exact ChatGPT Project URL")
	}
	return nil
}
