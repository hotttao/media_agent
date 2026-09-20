package keto

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientRelationTupleContract(t *testing.T) {
	var methods []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.URL.Path == "/relation-tuples/check/openapi" {
			if r.URL.Query().Get("relation") != "use_materials" || r.URL.Query().Get("subject_id") != "u1" {
				t.Fatalf("unexpected check request: %s", r.URL.String())
			}
			_ = json.NewEncoder(w).Encode(map[string]bool{"allowed": true})
			return
		}
		switch r.Method {
		case http.MethodGet:
			if r.URL.Path != "/relation-tuples" || r.URL.Query().Get("namespace") != "Organization" || r.URL.Query().Get("object") != "org1" {
				t.Fatalf("unexpected list request: %s", r.URL.String())
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"relation_tuples": []Tuple{{Namespace: "Organization", Object: "org1", Relation: "members", SubjectID: "u1"}}})
		case http.MethodPut:
			var tuple Tuple
			if err := json.NewDecoder(r.Body).Decode(&tuple); err != nil || tuple.SubjectID != "u1" {
				t.Fatalf("put tuple=%+v err=%v", tuple, err)
			}
			w.WriteHeader(http.StatusCreated)
		case http.MethodDelete:
			if r.URL.Query().Get("subject_id") != "u1" {
				t.Fatalf("delete query=%v", r.URL.Query())
			}
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	client := New(server.URL, server.URL, server.Client())
	tuples, err := client.List(context.Background(), "Organization", "org1")
	if err != nil || len(tuples) != 1 {
		t.Fatalf("list tuples=%v err=%v", tuples, err)
	}
	if err := client.Put(context.Background(), tuples[0]); err != nil {
		t.Fatal(err)
	}
	if err := client.Delete(context.Background(), tuples[0]); err != nil {
		t.Fatal(err)
	}
	allowed, err := client.Check(context.Background(), CheckRequest{Namespace: "Team", Object: "t1", Relation: "use_materials", SubjectID: "u1"})
	if err != nil || !allowed {
		t.Fatalf("check allowed=%v err=%v", allowed, err)
	}
	if len(methods) != 4 {
		t.Fatalf("methods=%v", methods)
	}
}
