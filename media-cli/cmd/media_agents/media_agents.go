// Package media_agents exposes HTTP drivers for Agent Runtime APIs.
package media_agents

import (
	"github.com/spf13/cobra"

	"media_agent/media_cli/cmd"
)

var MediaAgentsCmd = &cobra.Command{Use: "media_agents", Short: "调用 Agent Runtime API"}

func init() { cmd.RegisterCmd(MediaAgentsCmd) }
