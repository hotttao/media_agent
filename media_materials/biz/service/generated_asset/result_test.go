package generated_asset

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"

	domain "media_agent/media_materials/biz/domain/generated_asset"
	"media_agent/media_materials/biz/shared/objectstorage"
)

type resultStore struct {
	importRepo
	assets      map[string]*domain.GeneratedAsset
	receipts    map[string]domain.ResultReceipt
	inTx        bool
	failReceipt bool
}

func (r *resultStore) Save(ctx context.Context, a *domain.GeneratedAsset) error {
	if !r.inTx {
		return errors.New("asset write outside transaction")
	}
	copy := *a
	r.assets[a.ID] = &copy
	return nil
}
func (r *resultStore) Get(ctx context.Context, id string) (*domain.GeneratedAsset, error) {
	a := r.assets[id]
	if a == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return a, nil
}
func (r *resultStore) GetByStepRun(context.Context, string) (*domain.GeneratedAsset, error) {
	panic("keyed registration must not use latest StepRun asset lookup")
}
func (r *resultStore) GetResult(ctx context.Context, step, key string, lock bool) (*domain.ResultReceipt, error) {
	if lock && !r.inTx {
		return nil, errors.New("lock outside transaction")
	}
	v, ok := r.receipts[step+key]
	if !ok {
		return nil, nil
	}
	return &v, nil
}
func (r *resultStore) SaveResult(ctx context.Context, v domain.ResultReceipt) error {
	if !r.inTx || r.failReceipt {
		return errors.New("receipt save failed")
	}
	r.receipts[v.StepRunID+v.ResultKey] = v
	return nil
}
func (r *resultStore) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	assets, receipts := make(map[string]*domain.GeneratedAsset), make(map[string]domain.ResultReceipt)
	for k, v := range r.assets {
		assets[k] = v
	}
	for k, v := range r.receipts {
		receipts[k] = v
	}
	r.inTx = true
	defer func() { r.inTx = false }()
	err := fn(ctx)
	if err != nil {
		r.assets, r.receipts = assets, receipts
	}
	return err
}

type resultStorage struct {
	store       *resultStore
	calls       int
	unavailable bool
}

func (s *resultStorage) Stat(context.Context, string) (objectstorage.ObjectInfo, error) {
	s.calls++
	if s.store.inTx {
		return objectstorage.ObjectInfo{}, errors.New("storage called inside transaction")
	}
	if s.unavailable {
		return objectstorage.ObjectInfo{}, errors.New("unavailable")
	}
	return objectstorage.ObjectInfo{SizeBytes: 42, Checksum: testDigest, ContentType: "image/png"}, nil
}
func (*resultStorage) PresignGet(context.Context, string) (objectstorage.ResolvedObject, error) {
	return objectstorage.ResolvedObject{}, nil
}

func resultFixture() (*Service, *resultStore, *resultStorage, UpsertCommand) {
	r := &resultStore{assets: map[string]*domain.GeneratedAsset{}, receipts: map[string]domain.ResultReceipt{}}
	storage := &resultStorage{store: r}
	s := NewService(r, importProduct{}, importVIP{})
	s.ConfigureResultReceipts(r, r)
	s.ConfigureHistoryImport(storage, importKinds{})
	s.ConfigureNativeOutputStorage("https://storage.example", "media")
	c := UpsertCommand{Scope: "ai_clothing", AssetKind: "first_frame", Name: "frame", OwnerUserID: "user",
		URL:   "https://storage.example/media/materials/org/ai_clothing/material/frame.png?signature=first",
		JobID: "job", StepRunID: "step", ResultKey: strings.Repeat("a", 64), TemplateID: "tpl", TemplateVersion: 1,
		SourceProductID: "product"}
	return s, r, storage, c
}

func TestGenerationRegistrationReplayAndIndependentRounds(t *testing.T) {
	s, r, storage, cmd := resultFixture()
	first, err := s.Upsert(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	storage.unavailable = true
	cmd.URL = strings.Replace(cmd.URL, "signature=first", "signature=expired", 1)
	replay, err := s.Upsert(context.Background(), cmd)
	if err != nil || replay.ID != first.ID || storage.calls != 1 {
		t.Fatal("same generation did not replay without storage access")
	}
	changed := cmd
	changed.Content = "different"
	if _, err := s.Upsert(context.Background(), changed); !errors.Is(err, domain.ErrResultConflict) {
		t.Fatal("changed payload accepted")
	}
	changed = cmd
	changed.Scope = "other"
	if _, err := s.Upsert(context.Background(), changed); err == nil {
		t.Fatal("replay bypassed source scope verification")
	}
	storage.unavailable = false
	cmd.ResultKey = strings.Repeat("b", 64)
	second, err := s.Upsert(context.Background(), cmd)
	if err != nil || second.ID == first.ID || second.Name == first.Name || len(r.assets) != 2 || len(r.receipts) != 2 {
		t.Fatal("new round did not create independent output")
	}
	delete(r.assets, second.ID)
	if _, err := s.Upsert(context.Background(), cmd); !errors.Is(err, domain.ErrResultConflict) || len(r.assets) != 1 || len(r.receipts) != 2 {
		t.Fatal("deleted generation was recreated")
	}
}

func TestGenerationRegistrationRollsBackAssetOnReceiptFailure(t *testing.T) {
	s, r, _, cmd := resultFixture()
	r.failReceipt = true
	if _, err := s.Upsert(context.Background(), cmd); err == nil || len(r.assets) != 0 || len(r.receipts) != 0 {
		t.Fatal("partial registration survived failed transaction")
	}
	r.failReceipt = false
	if _, err := s.Upsert(context.Background(), cmd); err != nil || len(r.assets) != 1 || len(r.receipts) != 1 {
		t.Fatal("registration could not recover after rollback")
	}
}
