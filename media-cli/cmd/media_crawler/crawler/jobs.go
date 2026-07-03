package crawler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	crawlerquery "media_agent/hertz_gen/media_crawler/crawler_query_service"
	crawlermodel "media_agent/hertz_gen/model/media_crawler/crawler"
	"media_agent/media_cli/cmd"
)

func newClient() (crawlerquery.Client, error) {
	return crawlerquery.NewCrawlerQueryServiceClient(cmd.FlagHost)
}

func listJobsCmd() *cobra.Command {
	var platform, status string
	var limit, offset int32
	c := &cobra.Command{
		Use:   "list-jobs",
		Short: "GET /api/crawler/jobs",
		RunE: func(c *cobra.Command, args []string) error {
			cli, err := newClient()
			if err != nil {
				return err
			}
			resp, _, err := cli.ListJobs(context.Background(), &crawlermodel.ListJobsRequest{
				Platform: platform, Status: status, Limit: limit, Offset: offset,
			})
			if err != nil {
				return err
			}
			return printJSON(resp)
		},
	}
	c.Flags().StringVar(&platform, "platform", "", "按平台过滤")
	c.Flags().StringVar(&status, "status", "", "按状态过滤")
	c.Flags().Int32Var(&limit, "limit", 20, "分页大小")
	c.Flags().Int32Var(&offset, "offset", 0, "偏移")
	return c
}

func getJobCmd() *cobra.Command {
	var jobID string
	c := &cobra.Command{
		Use:   "get-job",
		Short: "GET /api/crawler/jobs/:job_id",
		RunE: func(c *cobra.Command, args []string) error {
			cli, err := newClient()
			if err != nil {
				return err
			}
			resp, _, err := cli.GetJob(context.Background(), &crawlermodel.GetJobRequest{JobId: jobID})
			if err != nil {
				return err
			}
			return printJSON(resp)
		},
	}
	c.Flags().StringVar(&jobID, "job-id", "", "job id")
	_ = c.MarkFlagRequired("job-id")
	return c
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func init() {
	CrawlerCmd.AddCommand(listJobsCmd())
	CrawlerCmd.AddCommand(getJobCmd())
}
