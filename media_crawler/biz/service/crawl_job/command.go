package crawl_job

// StartCrawlerCommand start_crawler 用例输入（对齐 mcp_crawler.proto StartCrawlerRequest）。
type StartCrawlerCommand struct {
	Platform              string
	CrawlerType           string
	Keywords              string
	URLs                  []string
	EnableComments        bool
	EnableSubComments     bool
	MaxNotesCount         int
	SaveDataOption        string
	StartPage             int
	EnableCheckpoint      bool
	CheckpointID          string
	HotSearchMaxItems     int
	HotSearchPerItemCount int
	OnBusy                string // queue(默认)/reject
	WaitForCompletion     bool
}

// ListJobsQuery list_jobs / ListJobs 用例输入。
type ListJobsQuery struct {
	Platform string
	Status   string
	Limit    int
	Offset   int
}
