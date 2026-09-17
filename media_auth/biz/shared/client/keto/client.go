// Package keto provides the narrow relation-tuple contract used by authorization projection.
package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type SubjectSet struct {
	Namespace string `json:"namespace"`
	Object    string `json:"object"`
	Relation  string `json:"relation"`
}

type Tuple struct {
	Namespace  string      `json:"namespace"`
	Object     string      `json:"object"`
	Relation   string      `json:"relation"`
	SubjectID  string      `json:"subject_id,omitempty"`
	SubjectSet *SubjectSet `json:"subject_set,omitempty"`
}

type Client struct {
	readURL, writeURL string
	http              *http.Client
}

func New(readURL, writeURL string, client *http.Client) *Client {
	if client == nil {
		client = http.DefaultClient
	}
	return &Client{readURL: strings.TrimRight(readURL, "/"), writeURL: strings.TrimRight(writeURL, "/"), http: client}
}

func (c *Client) List(ctx context.Context, namespace, object string) ([]Tuple, error) {
	q := url.Values{"namespace": {namespace}, "object": {object}}
	var response struct {
		Tuples []Tuple `json:"relation_tuples"`
	}
	if err := c.do(ctx, http.MethodGet, c.readURL+"/relation-tuples?"+q.Encode(), nil, &response); err != nil {
		return nil, err
	}
	return response.Tuples, nil
}

func (c *Client) Put(ctx context.Context, tuple Tuple) error {
	return c.do(ctx, http.MethodPut, c.writeURL+"/admin/relation-tuples", tuple, nil)
}

func (c *Client) Delete(ctx context.Context, tuple Tuple) error {
	q := url.Values{"namespace": {tuple.Namespace}, "object": {tuple.Object}, "relation": {tuple.Relation}}
	if tuple.SubjectID != "" {
		q.Set("subject_id", tuple.SubjectID)
	} else if tuple.SubjectSet != nil {
		q.Set("subject_set.namespace", tuple.SubjectSet.Namespace)
		q.Set("subject_set.object", tuple.SubjectSet.Object)
		q.Set("subject_set.relation", tuple.SubjectSet.Relation)
	}
	return c.do(ctx, http.MethodDelete, c.writeURL+"/admin/relation-tuples?"+q.Encode(), nil, nil)
}

func (c *Client) do(ctx context.Context, method, endpoint string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("keto %s %s: status %d: %s", method, endpoint, resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
