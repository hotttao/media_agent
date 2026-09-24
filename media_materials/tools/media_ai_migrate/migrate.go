package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
)

const maxSourceImageBytes = (16 << 20) + 1

type migrator struct {
	cfg        config
	target     *targetClient
	sourceHTTP *http.Client
	logger     *log.Logger
	stats      migrationStats
}

type migrationStats struct {
	Materials, VirtualIPs, Products resourceStats
	Images                          resourceStats
}

type resourceStats struct{ Created, Updated, Skipped, Failed int }

func (m *migrator) run(ctx context.Context, data sourceData) error {
	if m.cfg.only == "material" || m.cfg.only == "virtual-ip" || m.cfg.only == "product" {
		if m.cfg.only != "material" {
			data.Materials = nil
		}
		if m.cfg.only != "virtual-ip" {
			data.VirtualIPs = nil
		}
		if m.cfg.only != "product" {
			data.Products = nil
		}
	}
	if m.cfg.dryRun {
		m.logger.Printf("dry-run: materials=%d virtual_ips=%d products=%d product_images=%d", len(data.Materials), len(data.VirtualIPs), len(data.Products), productImageCount(data.Products))
		return nil
	}
	materials, err := m.target.list(ctx, "materials")
	if err != nil {
		return fmt.Errorf("读取目标 materials: %w", err)
	}
	virtualIPs, err := m.target.list(ctx, "virtual-ips")
	if err != nil {
		return fmt.Errorf("读取目标 virtual-ips: %w", err)
	}
	products, err := m.target.list(ctx, "products")
	if err != nil {
		return fmt.Errorf("读取目标 products: %w", err)
	}

	var failures []string
	for index, item := range data.Materials {
		if err := m.migrateMaterial(ctx, item, materials); err != nil {
			m.stats.Materials.Failed++
			failures = append(failures, "material "+item.ID+": "+err.Error())
			m.logger.Printf("[%d/%d] material %q 失败: %v", index+1, len(data.Materials), item.Name, err)
		} else {
			m.logger.Printf("[%d/%d] material %q 完成", index+1, len(data.Materials), item.Name)
		}
	}
	for index, item := range data.VirtualIPs {
		if err := m.migrateVirtualIP(ctx, item, virtualIPs); err != nil {
			m.stats.VirtualIPs.Failed++
			failures = append(failures, "virtual_ip "+item.ID+": "+err.Error())
			m.logger.Printf("[%d/%d] virtual_ip %q 失败: %v", index+1, len(data.VirtualIPs), item.Nickname, err)
		} else {
			m.logger.Printf("[%d/%d] virtual_ip %q 完成", index+1, len(data.VirtualIPs), item.Nickname)
		}
	}
	for index, item := range data.Products {
		if err := m.migrateProduct(ctx, item, products); err != nil {
			m.stats.Products.Failed++
			failures = append(failures, "product "+item.ID+": "+err.Error())
			m.logger.Printf("[%d/%d] product %q 失败: %v", index+1, len(data.Products), item.Name, err)
		} else {
			m.logger.Printf("[%d/%d] product %q 完成", index+1, len(data.Products), item.Name)
		}
	}
	if len(failures) > 0 {
		sort.Strings(failures)
		return fmt.Errorf("%d 条记录迁移失败:\n%s", len(failures), strings.Join(failures, "\n"))
	}
	return nil
}

func (m *migrator) migrateMaterial(ctx context.Context, item sourceMaterial, existing []targetItem) error {
	objectPath, err := m.migrateImage(ctx, "material", item.ID, "file", item.URL.String, "material")
	if err != nil {
		return err
	}
	metadata := migrationMetadata(item.ID, item.CreatedAt.String(), item.UpdatedAt.String())
	metadata["legacy_tags"] = parseStringList(item.Tags.String)
	payload := map[string]any{
		"name": item.Name, "description": item.Description.String, "prompt": item.Prompt.String,
		"asset_kind": strings.ToLower(item.Type), "object_path": objectPath, "content": "",
		"metadata": mustJSONString(metadata), "visibility": mapVisibility(item.Visibility), "team_id": "",
	}
	id := findExisting(existing, item.ID, item.Name, strings.ToLower(item.Type))
	_, created, err := m.target.upsert(ctx, "materials", m.cfg.scope, id, payload)
	if err != nil {
		return err
	}
	increment(&m.stats.Materials, created)
	return nil
}

func (m *migrator) migrateVirtualIP(ctx context.Context, item sourceVirtualIP, existing []targetItem) error {
	images := map[string]struct{ source, purpose string }{
		"avatar_object_path":     {item.AvatarURL.String, "virtual_ip_avatar"},
		"full_body_object_path":  {item.FullBodyURL.String, "virtual_ip_full_body"},
		"three_view_object_path": {item.ThreeViewURL.String, "virtual_ip_three_view"},
		"nine_view_object_path":  {item.NineViewURL.String, "virtual_ip_nine_view"},
	}
	paths := make(map[string]string, len(images))
	for field, image := range images {
		objectPath, err := m.migrateImage(ctx, "virtual-ip", item.ID, field, image.source, image.purpose)
		if err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		paths[field] = objectPath
	}
	persona := map[string]any{}
	putNullInt(persona, "age", item.Age)
	putNullFloat(persona, "height", item.Height)
	putNullFloat(persona, "weight", item.Weight)
	putNullFloat(persona, "bust", item.Bust)
	putNullFloat(persona, "waist", item.Waist)
	putNullFloat(persona, "hip", item.Hip)
	for key, value := range map[string]sql.NullString{
		"gender": item.Gender, "education": item.Education, "major": item.Major, "city": item.City,
		"occupation": item.Occupation, "basic_setting": item.BasicSetting, "catchphrase": item.Catchphrase,
		"small_habit": item.SmallHabit, "family_background": item.FamilyBackground, "income_level": item.IncomeLevel,
		"personality": item.Personality, "hobbies": item.Hobbies,
	} {
		if value.Valid {
			persona[key] = value.String
		}
	}
	payload := map[string]any{
		"name": item.Nickname, "description": "", "persona_profile": mustJSONString(persona), "visibility": "private",
		"avatar_object_path": paths["avatar_object_path"], "full_body_object_path": paths["full_body_object_path"],
		"three_view_object_path": paths["three_view_object_path"], "nine_view_object_path": paths["nine_view_object_path"],
	}
	id := findExisting(existing, item.ID, item.Nickname, "")
	_, created, err := m.target.upsert(ctx, "virtual-ips", m.cfg.scope, id, payload)
	if err != nil {
		return err
	}
	increment(&m.stats.VirtualIPs, created)
	return nil
}

func (m *migrator) migrateProduct(ctx context.Context, item sourceProduct, existing []targetItem) error {
	images := make([]map[string]any, 0, len(item.Images))
	for _, image := range item.Images {
		objectPath, err := m.migrateImage(ctx, "product", item.ID, "image-"+image.ID, image.URL, "product_image")
		if err != nil {
			return fmt.Errorf("image %s: %w", image.ID, err)
		}
		images = append(images, map[string]any{"object_path": objectPath, "is_main": image.IsMain, "sort_order": image.Order})
	}
	externalCommerce := map[string]any{
		"promotion_id": item.PromotionID.String, "elastic_title": item.ElasticTitle.String,
		"elastic_images_uri": item.ElasticImagesURI.String, "first_c_name": item.FirstCName.String,
		"second_c_name": item.SecondCName.String, "third_c_name": item.ThirdCName.String,
		"fourth_c_name": item.FourthCName.String, "price": item.Price, "aweme_id": item.AwemeID.String,
	}
	payload := map[string]any{
		"name": item.Name, "description": "", "target_audience": item.TargetAudience,
		"product_details": item.ProductDetails.String, "display_actions": mustJSONString(parseJSONList(item.DisplayActions.String)),
		"tags": mustJSONString(parseStringList(item.Tags.String)), "external_commerce": mustJSONString(externalCommerce),
		"metadata": mustJSONString(migrationMetadata(item.ID, item.CreatedAt.String(), item.UpdatedAt.String())), "images": images,
	}
	id := findExisting(existing, item.ID, item.Name, "")
	_, created, err := m.target.upsert(ctx, "products", m.cfg.scope, id, payload)
	if err != nil {
		return err
	}
	increment(&m.stats.Products, created)
	return nil
}

func (m *migrator) migrateImage(ctx context.Context, resource, sourceID, field, rawURL, purpose string) (string, error) {
	if strings.TrimSpace(rawURL) == "" {
		m.stats.Images.Skipped++
		return "", nil
	}
	fullURL := rawURL
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		fullURL = m.cfg.sourceAssetsURL + "/" + strings.TrimLeft(rawURL, "/")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := m.sourceHTTP.Do(req)
	if err != nil {
		m.stats.Images.Failed++
		return "", fmt.Errorf("下载 %s: %w", fullURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		m.stats.Images.Failed++
		return "", fmt.Errorf("下载 %s: HTTP %d", fullURL, resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxSourceImageBytes))
	if err != nil {
		m.stats.Images.Failed++
		return "", fmt.Errorf("读取 %s: %w", fullURL, err)
	}
	if len(data) == 0 || len(data) >= maxSourceImageBytes {
		m.stats.Images.Failed++
		return "", fmt.Errorf("图片为空或超过 16 MiB: %s (%d bytes)", fullURL, len(data))
	}
	filename := sourceFilename(rawURL)
	contentType := normalizedContentType(resp.Header.Get("Content-Type"), filename, data)
	keyHash := sha256.Sum256([]byte("media_ai:" + resource + ":" + sourceID + ":" + field + ":" + rawURL))
	result, err := m.target.upload(ctx, purpose, "media-ai-"+hex.EncodeToString(keyHash[:16]), filename, contentType, data)
	if err != nil {
		m.stats.Images.Failed++
		return "", err
	}
	m.stats.Images.Created++
	return result.ObjectPath, nil
}

func findExisting(items []targetItem, sourceID, name, assetKind string) string {
	for _, item := range items {
		if metadataSourceID(item.Metadata) == sourceID {
			return item.ID
		}
	}
	for _, item := range items {
		if item.Name == name && (assetKind == "" || item.AssetKind == assetKind) {
			return item.ID
		}
	}
	return ""
}

func metadataSourceID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var encoded string
	if raw[0] == '"' && json.Unmarshal(raw, &encoded) == nil {
		raw = []byte(encoded)
	}
	var metadata map[string]any
	if json.Unmarshal(raw, &metadata) != nil {
		return ""
	}
	if migration, ok := metadata["migration"].(map[string]any); ok {
		if sourceID, ok := migration["source_id"].(string); ok {
			return sourceID
		}
	}
	return ""
}

func migrationMetadata(sourceID, createdAt, updatedAt string) map[string]any {
	return map[string]any{"migration": map[string]any{"source": "media_ai", "source_id": sourceID, "source_created_at": createdAt, "source_updated_at": updatedAt}}
}

func mapVisibility(value string) string {
	switch strings.ToUpper(value) {
	case "PUBLIC":
		return "public"
	case "TEAM":
		return "team"
	default:
		return "private"
	}
}

func parseStringList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return []string{}
	}
	var result []string
	if json.Unmarshal([]byte(value), &result) == nil {
		return result
	}
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '，' || r == '\n' })
	result = result[:0]
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseJSONList(value string) []any {
	value = strings.TrimSpace(value)
	if value == "" {
		return []any{}
	}
	var result []any
	if json.Unmarshal([]byte(value), &result) == nil {
		return result
	}
	stringsResult := parseStringList(value)
	result = make([]any, len(stringsResult))
	for i := range stringsResult {
		result[i] = stringsResult[i]
	}
	return result
}

func mustJSONString(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func putNullInt(target map[string]any, key string, value sql.NullInt64) {
	if value.Valid {
		target[key] = value.Int64
	}
}

func putNullFloat(target map[string]any, key string, value sql.NullFloat64) {
	if value.Valid {
		target[key] = value.Float64
	}
}

func increment(stats *resourceStats, created bool) {
	if created {
		stats.Created++
	} else {
		stats.Updated++
	}
}

func productImageCount(products []sourceProduct) int {
	total := 0
	for _, product := range products {
		total += len(product.Images)
	}
	return total
}
