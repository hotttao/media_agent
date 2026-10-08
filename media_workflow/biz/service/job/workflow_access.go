package job

import "context"

func (s *Service) ConfigureWorkflowAccess(check func(context.Context, string, string, string, string) error) {
	s.workflowAccess = check
}
