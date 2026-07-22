package media_workflow

import (
	"github.com/spf13/cobra"
)

// Cmd media_workflow 一级命令（caller 编排：组装 bundle + SubmitJob + job 查询）。
var Cmd = &cobra.Command{
	Use:   "media_workflow",
	Short: "media_workflow caller: assemble JobInput bundle + SubmitJob + job ops",
}

func init() {
	Cmd.AddCommand(submitJobCmd)
}
