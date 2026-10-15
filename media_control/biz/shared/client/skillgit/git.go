// Package skillgit fetches one fixed Git tree without exposing credentials to argv.
package skillgit

import (
	"context"
	"errors"
	"media_agent/hertz_infra/skillarchive"
	d "media_agent/media_control/biz/domain/skill"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var ErrFetch = errors.New("skill git fetch failed")

type Client struct{}

func (Client) Fetch(ctx context.Context, source d.GitSource, token string) (skillarchive.Tree, string, error) {
	if err := d.ValidateGit(source); err != nil {
		return nil, "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	root, err := os.MkdirTemp("", "skill-git-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(root)
	askpass := filepath.Join(root, "askpass")
	script := "#!/bin/sh\ncase \"$1\" in *Username*) printf '%s\\n' 'oauth2';; *) printf '%s\\n' \"$MEDIA_SKILL_GIT_TOKEN\";; esac\n"
	if err := os.WriteFile(askpass, []byte(script), 0700); err != nil {
		return nil, "", err
	}
	env := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "LANG=C.UTF-8", "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_ASKPASS=" + askpass, "MEDIA_SKILL_GIT_TOKEN=" + token}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY", "NO_PROXY", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	args := []string{"-c", "protocol.file.allow=never", "-c", "protocol.ext.allow=never", "-c", "protocol.ssh.allow=never", "-c", "http.followRedirects=false", "clone", "--depth=1", "--single-branch", "--no-tags"}
	if source.Branch != "" {
		args = append(args, "--branch="+source.Branch)
	}
	// URL is parsed solely to keep credentials/query out of command arguments.
	parsed, _ := url.Parse(source.URL)
	args = append(args, "--", parsed.String(), filepath.Join(root, "repo"))
	command := exec.CommandContext(ctx, "git", args...)
	command.Env = env
	if err := command.Run(); err != nil {
		return nil, "", ErrFetch
	}
	repo := filepath.Join(root, "repo")
	revision := exec.CommandContext(ctx, "git", "-C", repo, "rev-parse", "HEAD")
	revision.Env = env
	commit, err := revision.Output()
	if err != nil {
		return nil, "", ErrFetch
	}
	target := filepath.Join(repo, filepath.FromSlash(source.Subpath))
	resolved, err := filepath.EvalSymlinks(target)
	if err != nil {
		return nil, "", ErrFetch
	}
	relative, err := filepath.Rel(repo, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, "", d.ErrInvalid
	}
	tree, err := skillarchive.Directory(resolved)
	if err != nil {
		return nil, "", ErrFetch
	}
	return tree, strings.TrimSpace(string(commit)), nil
}
