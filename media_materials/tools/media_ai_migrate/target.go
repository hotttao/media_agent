package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"path"
	"strings"
)

type targetClient struct {
	baseURL, organizationID, scope string
	sessionCookieName              string
	sessionCookie, bearerToken     string
	http                           *http.Client
}

type targetItem struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	AssetKind string          `json:"asset_kind"`
	Metadata  json.RawMessage `json:"metadata"`
}

type targetPage struct {
	Items []targetItem `json:"items"`
	Total int          `json:"total"`
}

type uploadResult struct {
	ObjectPath string `json:"object_path"`
	Checksum   string `json:"checksum"`
}

func (c *targetClient) list(ctx context.Context, resource string) ([]targetItem, error) {
	var all []targetItem
	for page := 1; ; page++ {
		query := url.Values{
			"organization_id": []string{c.organizationID},
			"scope":           []string{c.scope},
			"page":            []string{fmt.Sprint(page)},
			"page_size":       []string{"100"},
		}
		var result targetPage
		if err := c.json(ctx, http.MethodGet, "/api/v1/"+resource+"?"+query.Encode(), nil, &result); err != nil {
			return nil, err
		}
		all = append(all, result.Items...)
		if len(result.Items) == 0 || len(all) >= result.Total {
			return all, nil
		}
	}
}

func (c *targetClient) upsert(ctx context.Context, resource, scope, id string, payload any) (string, bool, error) {
	method := http.MethodPost
	endpoint := "/api/v1/" + resource
	created := true
	query := url.Values{"organization_id": []string{c.organizationID}}
	if id == "" {
		query.Set("scope", scope)
	} else {
		method = http.MethodPut
		endpoint += "/" + url.PathEscape(id)
		created = false
	}
	var response struct {
		ID string `json:"id"`
	}
	if err := c.json(ctx, method, endpoint+"?"+query.Encode(), payload, &response); err != nil {
		return "", false, err
	}
	return response.ID, created, nil
}

func (c *targetClient) upload(ctx context.Context, purpose, idempotencyKey, filename, contentType string, data []byte) (uploadResult, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(map[string][]string)
	header["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename="%s"`, escapeQuotes(filename))}
	header["Content-Type"] = []string{contentType}
	part, err := writer.CreatePart(textproto.MIMEHeader(header))
	if err != nil {
		return uploadResult{}, err
	}
	if _, err := part.Write(data); err != nil {
		return uploadResult{}, err
	}
	if err := writer.Close(); err != nil {
		return uploadResult{}, err
	}
	query := url.Values{
		"organization_id": []string{c.organizationID},
		"scope":           []string{c.scope},
		"purpose":         []string{purpose},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/library-objects:upload?"+query.Encode(), &body)
	if err != nil {
		return uploadResult{}, err
	}
	c.authorize(req)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Idempotency-Key", idempotencyKey)
	var result uploadResult
	if err := c.do(req, &result); err != nil {
		return uploadResult{}, err
	}
	if result.ObjectPath == "" {
		return uploadResult{}, fmt.Errorf("上传响应缺少 object_path")
	}
	return result, nil
}

func (c *targetClient) json(ctx context.Context, method, endpoint string, payload, result any) error {
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+endpoint, body)
	if err != nil {
		return err
	}
	c.authorize(req)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.do(req, result)
}

func (c *targetClient) authorize(req *http.Request) {
	if c.sessionCookie != "" {
		req.AddCookie(&http.Cookie{Name: c.sessionCookieName, Value: c.sessionCookie})
	}
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
}

func (c *targetClient) do(req *http.Request, result any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: HTTP %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("解析 %s %s 响应: %w", req.Method, req.URL.Path, err)
		}
	}
	return nil
}

func sourceFilename(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err == nil {
		if name := path.Base(u.Path); name != "" && name != "." && name != "/" {
			return name
		}
	}
	return "image.jpg"
}

func normalizedContentType(header, filename string, data []byte) string {
	if mediaType, _, err := mime.ParseMediaType(header); err == nil && (strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/")) {
		return mediaType
	}
	if mediaType := mime.TypeByExtension(path.Ext(filename)); strings.HasPrefix(mediaType, "image/") || strings.HasPrefix(mediaType, "video/") {
		return mediaType
	}
	return http.DetectContentType(data)
}

func escapeQuotes(value string) string {
	return strings.NewReplacer("\\", "_", `"`, "_").Replace(value)
}
