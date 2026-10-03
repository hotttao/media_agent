package media_workflow

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
	"google.golang.org/protobuf/encoding/protojson"
	pb "media_agent/hertz_gen/model/media_workflow/historical_publication"
)

func init() {
	group := &cobra.Command{Use: "historical-publication", Short: "查询独立历史发布记录及有序视频成员"}
	var id string
	get := &cobra.Command{Use: "get", RunE: func(c *cobra.Command, _ []string) error {
		if HistoryCli == nil {
			return errors.New("client.media_workflow is not configured")
		}
		result, _, err := HistoryCli.GetHistoricalVideoPush(c.Context(), &pb.GetHistoricalVideoPushReq{Id: id})
		if err != nil {
			return err
		}
		data, err := protojson.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(c.OutOrStdout(), string(data))
		return err
	}}
	get.Flags().StringVar(&id, "id", "", "历史发布记录 ID")
	_ = get.MarkFlagRequired("id")
	var workspace string
	var page, size int32
	list := &cobra.Command{Use: "list", RunE: func(c *cobra.Command, _ []string) error {
		if HistoryCli == nil {
			return errors.New("client.media_workflow is not configured")
		}
		result, _, err := HistoryCli.ListHistoricalVideoPushes(c.Context(), &pb.ListHistoricalVideoPushesReq{WorkspaceId: workspace, Page: page, PageSize: size})
		if err != nil {
			return err
		}
		data, err := protojson.Marshal(result)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(c.OutOrStdout(), string(data))
		return err
	}}
	list.Flags().StringVar(&workspace, "workspace-id", "", "Workspace ID")
	list.Flags().Int32Var(&page, "page", 1, "页码")
	list.Flags().Int32Var(&size, "page-size", 20, "每页记录数")
	_ = list.MarkFlagRequired("workspace-id")
	group.AddCommand(get, list)
	Cmd.AddCommand(group)
}
