// new_server 工具：从 media_example 复制初始化新服务骨架。
//
// 用法（在仓库根执行）：
//
//	go run ./harness/tools/new_server <new_module> <new_app> <port>
//
// 例：
//
//	go run ./harness/tools/new_server media_workflow media-workflow 8002
//
// 流程：
//  1. 验证参数
//  2. 复制 media_example/ → <new_module>/
//  3. 批量替换占位符（go.mod module + import 路径 + 配置文件 + .env + docker-compose + e2e）
//  4. 改 go.work：追加 use ./<new_module>
//  5. 改 media-cli/conf/{local,dev}.yaml：加 client.<new_module> 段
//  6. 打印后续操作
//
// 命名约定：
//   - <new_module>：Go module 名（media_<x> 模式，如 media_workflow）
//   - <new_app>    ：app.name + 数据库名（kebab-case，如 media-workflow）
//   - <port>        ：服务端口
package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// repoRoot 仓库根（new_server 工具在 harness/tools/new_server/ 下）。
// 工具从仓库根运行（go run ./harness/tools/...），但 os.Getwd 是仓库根。
var repoRoot string

func main() {
	if len(os.Args) != 4 {
		log.Fatalf("usage: new_server <new_module> <new_app> <port>\n  e.g. new_server media_workflow media-workflow 8002")
	}
	newModule := os.Args[1]
	newApp := os.Args[2]
	portStr := os.Args[3]

	if err := validate(newModule, newApp, portStr); err != nil {
		log.Fatalf("invalid args: %v", err)
	}
	port, _ := strconv.Atoi(portStr)

	// 仓库根
	if repoRoot == "" {
		wd, _ := os.Getwd()
		// 如果从 harness/tools/new_server 启动，往上两级
		if filepath.Base(wd) == "new_server" {
			repoRoot = filepath.Join(wd, "..", "..")
		} else {
			repoRoot = wd
		}
	}
	log.Printf("repo root: %s", repoRoot)

	tplPath := filepath.Join(repoRoot, "media_example")
	newPath := filepath.Join(repoRoot, newModule)
	if _, err := os.Stat(newPath); err == nil {
		log.Fatalf("target already exists: %s", newPath)
	}

	// 1. 复制
	log.Printf("copying %s → %s", tplPath, newPath)
	if err := runCmd("cp", "-r", tplPath, newPath); err != nil {
		log.Fatalf("copy: %v", err)
	}

	// 2. 批量替换
	log.Printf("replacing placeholders...")
	if err := replaceAll(newPath, "media_example", newModule); err != nil {
		log.Fatalf("replace media_example → %s: %v", newModule, err)
	}
	if err := replaceAll(newPath, "media-example", newApp); err != nil {
		log.Fatalf("replace media-example → %s: %v", newApp, err)
	}

	// 端口替换
	if err := replaceInFile(filepath.Join(newPath, "Dockerfile"), "EXPOSE 8000", "EXPOSE "+portStr); err != nil {
		log.Fatalf("Dockerfile EXPOSE: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "docker-compose.yml"), ":8000", ":"+portStr); err != nil {
		log.Fatalf("docker-compose port: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "conf/local.yaml"), ":8000", ":"+portStr); err != nil {
		log.Fatalf("local.yaml port: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "conf/dev.yaml"), ":8000", ":"+portStr); err != nil {
		log.Fatalf("dev.yaml port: %v", err)
	}

	// .env: POSTGRES_DSN 数据库名 + LOG_DIR
	oldDB := "media_example?sslmode=disable"
	newDB := newApp + "?sslmode=disable"
	if err := replaceInFile(filepath.Join(newPath, ".env"), oldDB, newDB); err != nil {
		log.Fatalf(".env DSN: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, ".env"), "media_logs/media-example", "media_logs/"+newApp); err != nil {
		log.Fatalf(".env LOG_DIR: %v", err)
	}
	// docker-compose: POSTGRES_DSN 数据库名 + LOG_DIR volumes
	if err := replaceInFile(filepath.Join(newPath, "docker-compose.yml"), oldDB, newDB); err != nil {
		log.Fatalf("docker-compose DSN: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "docker-compose.yml"), "media_logs/media-example", "media_logs/"+newApp); err != nil {
		log.Fatalf("docker-compose LOG_DIR: %v", err)
	}

	// e2e fixtures: TEST_DB
	if err := replaceInFile(filepath.Join(newPath, "e2e/fixtures/pg.go"), "media_example_e2e", newApp+"_e2e"); err != nil {
		log.Fatalf("e2e pg.go: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "e2e/fixtures/migrate.go"), "media_example_e2e", newApp+"_e2e"); err != nil {
		log.Fatalf("e2e migrate.go: %v", err)
	}

	// 3. go.work
	if err := addToGoWork(filepath.Join(repoRoot, "go.work"), newModule); err != nil {
		log.Fatalf("go.work: %v", err)
	}
	log.Printf("go.work: added use ./%s", newModule)

	// 4. media-cli/conf
	if err := addMediaCliClient(filepath.Join(repoRoot, "media-cli/conf"), newModule, newApp, portStr); err != nil {
		log.Fatalf("media-cli conf: %v", err)
	}
	log.Printf("media-cli conf: added client.%s", newModule)

	log.Printf("\n=== done ===")
	log.Printf("next steps:")
	log.Printf("  cd %s", newModule)
	log.Printf("  go mod tidy")
	log.Printf("  # edit main.go, biz/, conf/local.yaml as needed")
	log.Printf("  # add new schema to migrations/ (or skip for now)")
	log.Printf("\n  # test:")
	log.Printf("  INFRA=e2e docker compose -f docker-compose/docker-compose.yml up -d postgres")
	log.Printf("  INFRA=e2e docker compose -f %s/docker-compose.yml up -d %s", newModule, newModule)
	log.Printf("  curl http://localhost:%s/health", portStr)
	_ = port
}

func validate(newModule, newApp, portStr string) error {
	if newModule == "" {
		return fmt.Errorf("new_module is empty")
	}
	if newApp == "" {
		return fmt.Errorf("new_app is empty")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("port must be numeric: %s", portStr)
	}
	if port <= 1024 {
		return fmt.Errorf("port must be > 1024 (avoid system ports)")
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9_]*$`).MatchString(newModule) {
		return fmt.Errorf("new_module must be snake_case (e.g. media_workflow)")
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(newApp) {
		return fmt.Errorf("new_app must be kebab-case (e.g. media-workflow)")
	}
	return nil
}

func replaceAll(root, old, new string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		switch ext {
		case ".go", ".yml", ".yaml", ".env", ".mod", ".sum":
			return runCmd("sed", "-i", "s/"+old+"/"+new+"/g", path)
		}
		return nil
	})
}

func replaceInFile(path, old, new string) error {
	return runCmd("sed", "-i", "s|"+old+"|"+new+"|g", path)
}

func addToGoWork(goWorkPath, newModule string) error {
	data, err := os.ReadFile(goWorkPath)
	if err != nil {
		return err
	}
	entry := "\t./" + newModule + "\n"
	if strings.Contains(string(data), entry) {
		return nil
	}
	content := string(data)
	idx := strings.Index(content, "use (")
	if idx < 0 {
		return fmt.Errorf("go.work has no 'use (' block")
	}
	insertAt := strings.Index(content[idx:], "\n")
	if insertAt < 0 {
		return fmt.Errorf("go.work format unexpected")
	}
	insertAt += idx + 1
	newContent := content[:insertAt] + entry + content[insertAt:]
	return os.WriteFile(goWorkPath, []byte(newContent), 0644)
}

func addMediaCliClient(confDir, newModule, newApp, portStr string) error {
	envVarName := strings.ReplaceAll(strings.ToUpper(newApp), "-", "_") + "_ADDR"
	clientBlock := fmt.Sprintf("  %s:\n"+
		"    enabled: true\n"+
		"    service_name: %s\n"+
		"    base_domain: ${%s:-http://127.0.0.1:%s}\n"+
		"    use_discovery: false\n"+
		"    timeout:\n"+
		"      enabled: true\n"+
		"      connect_timeout_ms: 200\n"+
		"      read_timeout_ms: 5000\n"+
		"      write_timeout_ms: 5000\n"+
		"      request_timeout_ms: 10000\n",
		newModule, newApp, envVarName, portStr,
	)
	for _, f := range []string{"local.yaml", "dev.yaml"} {
		path := filepath.Join(confDir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		content := string(data)
		idx := strings.Index(content, "client:")
		if idx < 0 {
			continue
		}
		endIdx := strings.Index(content[idx:], "\n")
		insertAt := idx + endIdx + 1
		newContent := content[:insertAt] + "\n" + clientBlock + content[insertAt:]
		if err := os.WriteFile(path, []byte(newContent), 0644); err != nil {
			return err
		}
	}
	return nil
}

func runCmd(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
