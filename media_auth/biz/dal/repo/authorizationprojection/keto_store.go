package authorizationprojection

import (
	"context"

	domain "media_agent/media_auth/biz/domain/authorizationprojection"
	ketoclient "media_agent/media_auth/biz/shared/client/keto"
)

type KetoStore struct{ client *ketoclient.Client }

func NewKetoStore(client *ketoclient.Client) *KetoStore { return &KetoStore{client: client} }

func (s *KetoStore) List(ctx context.Context, namespace, object string) ([]domain.Tuple, error) {
	rows, err := s.client.List(ctx, namespace, object)
	if err != nil {
		return nil, err
	}
	out := make([]domain.Tuple, 0, len(rows))
	for _, row := range rows {
		out = append(out, fromKeto(row))
	}
	return out, nil
}

func (s *KetoStore) Put(ctx context.Context, tuple domain.Tuple) error {
	return s.client.Put(ctx, toKeto(tuple))
}

func (s *KetoStore) Delete(ctx context.Context, tuple domain.Tuple) error {
	return s.client.Delete(ctx, toKeto(tuple))
}

func fromKeto(v ketoclient.Tuple) domain.Tuple {
	out := domain.Tuple{Namespace: v.Namespace, Object: v.Object, Relation: v.Relation, SubjectID: v.SubjectID}
	if v.SubjectSet != nil {
		out.SubjectSet = &domain.SubjectSet{Namespace: v.SubjectSet.Namespace, Object: v.SubjectSet.Object, Relation: v.SubjectSet.Relation}
	}
	return out
}

func toKeto(v domain.Tuple) ketoclient.Tuple {
	out := ketoclient.Tuple{Namespace: v.Namespace, Object: v.Object, Relation: v.Relation, SubjectID: v.SubjectID}
	if v.SubjectSet != nil {
		out.SubjectSet = &ketoclient.SubjectSet{Namespace: v.SubjectSet.Namespace, Object: v.SubjectSet.Object, Relation: v.SubjectSet.Relation}
	}
	return out
}
