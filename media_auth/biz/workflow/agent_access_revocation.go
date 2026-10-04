package workflow

import (
	"context"
	"log"
	"time"

	api "media_agent/media_auth/biz/domain/api_token"
)

type AgentAccessAssessor interface {
	Assess(context.Context, string, string, string) (bool, error)
}

// AgentAccessRevocation is a reconciliation safety net, not an event consumer.
// It persists cleanup only for confirmed invalidation, never authority outages.
type AgentAccessRevocation struct {
	source   api.AgentRevocationSource
	assessor AgentAccessAssessor
	tokens   api.ActorRevocationRepository
	tx       TransactionRunner
}

func NewAgentAccessRevocation(source api.AgentRevocationSource, assessor AgentAccessAssessor, tokens api.ActorRevocationRepository, tx TransactionRunner) *AgentAccessRevocation {
	return &AgentAccessRevocation{source: source, assessor: assessor, tokens: tokens, tx: tx}
}

func (w *AgentAccessRevocation) ProcessBatch(ctx context.Context, afterID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return afterID, err
	}
	tokens, err := w.source.ListAgentRevocationCandidates(ctx, afterID, 100)
	if err != nil {
		return afterID, err
	}
	for _, token := range tokens {
		if err := ctx.Err(); err != nil {
			return afterID, err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		// The repository filters actor_type; defend the boundary again before
		// interpreting authority or writing any cleanup intent.
		if token.ActorType == api.ActorAgent {
			valid, assessErr := w.assessor.Assess(attemptCtx, token.ActorID, token.SubjectUserID, token.OrganizationID)
			if assessErr != nil {
				log.Printf("agent access assessment pending token_id=%s", token.ID)
			} else if !valid && attemptCtx.Err() == nil {
				err := w.tx.RunInTx(attemptCtx, func(txCtx context.Context) error {
					if err := txCtx.Err(); err != nil {
						return err
					}
					return w.tokens.PrepareActorRevocation(txCtx, api.ActorAgent, token.ActorID, token.SubjectUserID, token.OrganizationID)
				})
				if err != nil {
					log.Printf("agent access cleanup pending token_id=%s", token.ID)
				}
			}
		}
		cancel()
		afterID = token.ID
	}
	if len(tokens) < 100 {
		return "", nil
	}
	return afterID, nil
}

func (w *AgentAccessRevocation) Run(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	cursor := ""
	for {
		if ctx.Err() != nil {
			return
		}
		next, err := w.ProcessBatch(ctx, cursor)
		if err != nil && ctx.Err() == nil {
			log.Print("agent access cleanup scan failed")
		}
		cursor = next
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
