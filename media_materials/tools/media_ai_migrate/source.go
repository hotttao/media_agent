package main

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type sourceMaterial struct {
	ID, Visibility, Type, Name string
	Description, Prompt, URL   sql.NullString
	Tags                       sql.NullString
	CreatedAt, UpdatedAt       time.Time
}

type sourceVirtualIP struct {
	ID, Nickname                                        string
	Age                                                 sql.NullInt64
	Gender, Education, Major, City, Occupation          sql.NullString
	BasicSetting, Catchphrase, SmallHabit               sql.NullString
	FamilyBackground, IncomeLevel, Personality, Hobbies sql.NullString
	Height, Weight, Bust, Waist, Hip                    sql.NullFloat64
	AvatarURL, FullBodyURL, ThreeViewURL, NineViewURL   sql.NullString
	CreatedAt, UpdatedAt                                time.Time
}

type sourceProduct struct {
	ID, Name, TargetAudience                         string
	ProductDetails, DisplayActions, Tags             sql.NullString
	PromotionID, ElasticTitle, ElasticImagesURI      sql.NullString
	FirstCName, SecondCName, ThirdCName, FourthCName sql.NullString
	Price                                            float64
	AwemeID                                          sql.NullString
	CreatedAt, UpdatedAt                             time.Time
	Images                                           []sourceProductImage
}

type sourceProductImage struct {
	ID, URL string
	IsMain  bool
	Order   int
}

type sourceData struct {
	Materials  []sourceMaterial
	VirtualIPs []sourceVirtualIP
	Products   []sourceProduct
}

func openSource(ctx context.Context, rawDSN string) (*sql.DB, error) {
	dsn, err := mysqlDSN(rawDSN)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开源数据库: %w", err)
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接源数据库: %w", err)
	}
	return db, nil
}

func mysqlDSN(raw string) (string, error) {
	if !strings.HasPrefix(raw, "mysql://") {
		if strings.Contains(raw, "parseTime=") {
			return raw, nil
		}
		separator := "?"
		if strings.Contains(raw, "?") {
			separator = "&"
		}
		return raw + separator + "parseTime=true&charset=utf8mb4", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("解析 source DSN: %w", err)
	}
	password, _ := u.User.Password()
	host := u.Host
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, "3306")
	}
	database := strings.TrimPrefix(u.Path, "/")
	if u.User.Username() == "" || database == "" {
		return "", fmt.Errorf("source DSN 必须包含用户名和数据库名")
	}
	params := u.Query()
	params.Set("parseTime", "true")
	params.Set("charset", "utf8mb4")
	return fmt.Sprintf("%s:%s@tcp(%s)/%s?%s", u.User.Username(), password, host, database, params.Encode()), nil
}

func loadSource(ctx context.Context, db *sql.DB) (sourceData, error) {
	materials, err := loadMaterials(ctx, db)
	if err != nil {
		return sourceData{}, err
	}
	virtualIPs, err := loadVirtualIPs(ctx, db)
	if err != nil {
		return sourceData{}, err
	}
	products, err := loadProducts(ctx, db)
	if err != nil {
		return sourceData{}, err
	}
	return sourceData{Materials: materials, VirtualIPs: virtualIPs, Products: products}, nil
}

func loadMaterials(ctx context.Context, db *sql.DB) ([]sourceMaterial, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, visibility, type, name, description, prompt, url, tags, created_at, updated_at FROM materials ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("读取 materials: %w", err)
	}
	defer rows.Close()
	var result []sourceMaterial
	for rows.Next() {
		var item sourceMaterial
		if err := rows.Scan(&item.ID, &item.Visibility, &item.Type, &item.Name, &item.Description, &item.Prompt, &item.URL, &item.Tags, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 material: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadVirtualIPs(ctx context.Context, db *sql.DB) ([]sourceVirtualIP, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, nickname, age, gender, height, weight, bust, waist, hip, education, major, city, occupation, basicSetting, catchphrase, small_habit, family_background, income_level, personality, hobbies, avatar_url, full_body_url, three_view_url, nine_view_url, created_at, updated_at FROM virtual_ips ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("读取 virtual_ips: %w", err)
	}
	defer rows.Close()
	var result []sourceVirtualIP
	for rows.Next() {
		var item sourceVirtualIP
		if err := rows.Scan(&item.ID, &item.Nickname, &item.Age, &item.Gender, &item.Height, &item.Weight, &item.Bust, &item.Waist, &item.Hip, &item.Education, &item.Major, &item.City, &item.Occupation, &item.BasicSetting, &item.Catchphrase, &item.SmallHabit, &item.FamilyBackground, &item.IncomeLevel, &item.Personality, &item.Hobbies, &item.AvatarURL, &item.FullBodyURL, &item.ThreeViewURL, &item.NineViewURL, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 virtual_ip: %w", err)
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func loadProducts(ctx context.Context, db *sql.DB) ([]sourceProduct, error) {
	// targetAudience/productDetails are legacy Prisma columns that were created
	// before the schema standardized most mapped fields on snake_case.
	rows, err := db.QueryContext(ctx, `SELECT id, name, targetAudience, productDetails, display_actions, tags, promotion_id, elastic_title, elastic_images_uri, first_c_name, second_c_name, third_c_name, fourth_c_name, price, aweme_id, created_at, updated_at FROM products ORDER BY created_at, id`)
	if err != nil {
		return nil, fmt.Errorf("读取 products: %w", err)
	}
	defer rows.Close()
	var result []sourceProduct
	byID := make(map[string]int)
	for rows.Next() {
		var item sourceProduct
		if err := rows.Scan(&item.ID, &item.Name, &item.TargetAudience, &item.ProductDetails, &item.DisplayActions, &item.Tags, &item.PromotionID, &item.ElasticTitle, &item.ElasticImagesURI, &item.FirstCName, &item.SecondCName, &item.ThirdCName, &item.FourthCName, &item.Price, &item.AwemeID, &item.CreatedAt, &item.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描 product: %w", err)
		}
		result = append(result, item)
		byID[item.ID] = len(result) - 1
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	imageRows, err := db.QueryContext(ctx, "SELECT id, product_id, url, is_main, `order` FROM product_images ORDER BY product_id, `order`, id")
	if err != nil {
		return nil, fmt.Errorf("读取 product_images: %w", err)
	}
	defer imageRows.Close()
	for imageRows.Next() {
		var image sourceProductImage
		var productID string
		if err := imageRows.Scan(&image.ID, &productID, &image.URL, &image.IsMain, &image.Order); err != nil {
			return nil, fmt.Errorf("扫描 product_image: %w", err)
		}
		if index, ok := byID[productID]; ok {
			result[index].Images = append(result[index].Images, image)
		}
	}
	return result, imageRows.Err()
}
