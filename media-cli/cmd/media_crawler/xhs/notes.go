package xhs

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	xhsquery "media_agent/hertz_gen/media_crawler/xhs_query_service"
	xhsmodel "media_agent/hertz_gen/model/media_crawler/xhs"
	"media_agent/media_cli/cmd"
)

func newClient() (xhsquery.Client, error) {
	return xhsquery.NewXhsQueryServiceClient(cmd.FlagHost)
}

func listNotesCmd() *cobra.Command {
	var keywords, ids, sortField, sortOrder, sourceKeyword string
	var limit, offset int32
	c := &cobra.Command{
		Use:   "list-notes",
		Short: "GET /api/xhs/notes",
		RunE: func(c *cobra.Command, args []string) error {
			cli, err := newClient()
			if err != nil {
				return err
			}
			resp, _, err := cli.ListXhsNotes(context.Background(), &xhsmodel.ListXhsNotesRequest{
				Keywords: keywords, Ids: ids, SortField: sortField, SortOrder: sortOrder,
				SourceKeyword: sourceKeyword, Limit: limit, Offset: offset,
			})
			if err != nil {
				return err
			}
			return printJSON(resp)
		},
	}
	c.Flags().StringVar(&keywords, "keywords", "", "逗号分隔关键词")
	c.Flags().StringVar(&ids, "ids", "", "逗号分隔 note_id")
	c.Flags().StringVar(&sourceKeyword, "source-keyword", "", "精确匹配 source_keyword")
	c.Flags().StringVar(&sortField, "sort-field", "time", "time|liked_count|comment_count")
	c.Flags().StringVar(&sortOrder, "sort-order", "desc", "desc|asc")
	c.Flags().Int32Var(&limit, "limit", 20, "分页大小")
	c.Flags().Int32Var(&offset, "offset", 0, "偏移")
	return c
}

func getNoteCmd() *cobra.Command {
	var noteID string
	c := &cobra.Command{
		Use:   "get-note",
		Short: "GET /api/xhs/notes/:note_id",
		RunE: func(c *cobra.Command, args []string) error {
			cli, err := newClient()
			if err != nil {
				return err
			}
			resp, _, err := cli.GetXhsNote(context.Background(), &xhsmodel.GetXhsNoteRequest{NoteId: noteID})
			if err != nil {
				return err
			}
			return printJSON(resp)
		},
	}
	c.Flags().StringVar(&noteID, "note-id", "", "笔记 id")
	_ = c.MarkFlagRequired("note-id")
	return c
}

func listCommentsCmd() *cobra.Command {
	var noteID string
	var limit, offset int32
	c := &cobra.Command{
		Use:   "list-comments",
		Short: "GET /api/xhs/notes/:note_id/comments",
		RunE: func(c *cobra.Command, args []string) error {
			cli, err := newClient()
			if err != nil {
				return err
			}
			resp, _, err := cli.ListXhsComments(context.Background(), &xhsmodel.ListXhsCommentsRequest{
				NoteId: noteID, Limit: limit, Offset: offset,
			})
			if err != nil {
				return err
			}
			return printJSON(resp)
		},
	}
	c.Flags().StringVar(&noteID, "note-id", "", "笔记 id")
	_ = c.MarkFlagRequired("note-id")
	c.Flags().Int32Var(&limit, "limit", 20, "分页大小")
	c.Flags().Int32Var(&offset, "offset", 0, "偏移")
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
	XhsCmd.AddCommand(listNotesCmd())
	XhsCmd.AddCommand(getNoteCmd())
	XhsCmd.AddCommand(listCommentsCmd())
}
