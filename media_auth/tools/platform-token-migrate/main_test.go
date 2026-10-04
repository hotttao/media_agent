package main

import (
	"bytes"
	"errors"
	"testing"

	"media_agent/media_auth/biz/workflow"
)

func TestInvalidArgumentsFailClosedWithoutEchoingInput(t *testing.T) {
	for _, args := range [][]string{nil, {"--secret=must-not-echo"}, {"--apply", "unexpected-secret"}, {"--config=unused", "--timeout=0"}, {"--config=unused", "--timeout=31m"}, {"--config=unused", "--key-file=/not/a/key", "--key-version=v1"}} {
		var out bytes.Buffer
		if err := run(args, &out); !errors.Is(err, workflow.ErrPlatformMigration) || out.Len() != 0 {
			t.Fatal("invalid invocation exposed data or did not fail closed")
		}
	}
}
