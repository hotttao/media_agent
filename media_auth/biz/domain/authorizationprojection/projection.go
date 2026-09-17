// Package authorizationprojection defines the durable event and tuple ports for Keto projection.
package authorizationprojection

import (
	"context"
	"encoding/json"
	"time"
)

type Event struct {
	ID, AggregateType, AggregateID, EventType string
	Payload                                   json.RawMessage
	Sequence                                  int64
	Attempts                                  int
	CreatedAt                                 time.Time
}

type OrganizationSnapshot struct {
	ID           string
	Status       string
	Members      []OrganizationMember
	Entitlements []string
}

type OrganizationMember struct{ UserID, Role string }

type Tuple struct {
	Namespace, Object, Relation string
	SubjectID                   string
	SubjectSet                  *SubjectSet
}

type SubjectSet struct{ Namespace, Object, Relation string }

type Outbox interface {
	ClaimPending(context.Context, int) ([]Event, error)
	MarkDelivered(context.Context, Event) error
	MarkFailed(context.Context, Event, string, time.Time) error
}

type SnapshotSource interface {
	Organization(context.Context, string) (OrganizationSnapshot, error)
}

type TupleStore interface {
	List(context.Context, string, string) ([]Tuple, error)
	Put(context.Context, Tuple) error
	Delete(context.Context, Tuple) error
}
