// new_server 工具：生成新 Hertz 微服务骨架。
//
// 用法（在仓库根执行）：
//
//	go run ./harness/tools/new_server <new_module> <new_app> <port>
//
// 例：
//
//	go run ./harness/tools/new_server media_test media-test 8003
//
// 流程：
//  1. 验证参数（module 蛇形、app 短横线、port>1024）
//  2. 调 hz new 生成 Hertz 骨架（带 health.proto）
//  3. 替换占位符（module 名、app 名、端口、DSN、LOG_DIR）
//  4. 改 go.work：追加 use ./<new_module>
//  5. 改 media-cli/conf/{local,dev}.yaml：加 client.<new_module> 段
//  6. 打印后续操作
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

var repoRoot string

func main() {
	if len(os.Args) != 4 {
		log.Fatalf("usage: new_server <new_module> <new_app> <port>\n  e.g. new_server media_test media-test 8003")
	}
	newModule := os.Args[1]
	newApp := os.Args[2]
	portStr := os.Args[3]

	if err := validate(newModule, newApp, portStr); err != nil {
		log.Fatalf("invalid args: %v", err)
	}

	// 仓库根
	if repoRoot == "" {
		wd, _ := os.Getwd()
		repoRoot = wd
	}

	tplIDL, _ := filepath.Abs(filepath.Join(repoRoot, "harness/tools/new_server/idls/health.proto"))
	idlPath := filepath.Join(repoRoot, "idl")
	newPath, _ := filepath.Abs(filepath.Join(repoRoot, newModule))
	if _, err := os.Stat(newPath); err == nil {
		log.Fatalf("target already exists: %s", newPath)
	}

	// 1. 生成临时 health.proto（替换 <MODULE> 占位符）
	tmpProto, _ := os.CreateTemp("", "health-*.proto")
	tmpProtoContent := `syntax = "proto3";

package health;

import "api.proto";

option go_package = "media_agent/` + newModule + `/biz/health";

service Health {
  rpc Health(HealthRequest) returns (HealthResponse) {
    option (api.get) = "/health";
  }
}

message HealthRequest {}

message HealthResponse {
  string status = 1;
}
`
	os.WriteFile(tmpProto.Name(), []byte(tmpProtoContent), 0644)
	defer os.Remove(tmpProto.Name())

	// 2. 调 hz new 生成 Hertz 骨架（含 health 端点）
	log.Printf("hz new: module=%s service=%s out=%s", newModule, newApp, newPath)
	if err := runCmd("hz", "new",
		"--idl", tmpProto.Name(),
		"--proto_path", idlPath,
		"--service", newApp,
		"--mod", "media_agent/"+newModule,
		"--out_dir", newPath,
		"--model_dir", "biz/model",
		"--handler_dir", "biz/handler",
		"--router_dir", "biz/router",
		"--no_recurse",
	); err != nil {
		log.Fatalf("hz new: %v", err)
	}

	// 3. 替换 import 路径和 service 名
	if err := replaceAll(newPath, "health", newModule); err != nil {
		log.Fatalf("placeholder replace: %v", err)
	}

	// 4. 端口替换
	if err := replaceInFile(filepath.Join(newPath, "Dockerfile"), "EXPOSE 8888", "EXPOSE "+portStr); err != nil {
		log.Fatalf("Dockerfile EXPOSE: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "docker-compose.yml"), "8888", portStr); err != nil {
		log.Fatalf("docker-compose port: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "conf/local.yaml"), "8888", portStr); err != nil {
		log.Fatalf("local.yaml port: %v", err)
	}
	if err := replaceInFile(filepath.Join(newPath, "conf/dev.yaml"), "8888", portStr); err != nil {
		log.Fatalf("dev.yaml port: %v", err)
	}

	// 5. go.work
	if err := addToGoWork(filepath.Join(repoRoot, "go.work"), newModule); err != nil {
		log.Fatalf("go.work: %v", err)
	}
	log.Printf("go.work: added use ./%s", newModule)

	// 6. media-cli conf
	if err := addMediaCliClient(filepath.Join(repoRoot, "media-cli/conf"), newModule, newApp, portStr); err != nil {
		log.Fatalf("media-cli conf: %v", err)
	}
	log.Printf("media-cli conf: added client.%s", newModule)

	log.Printf("\n=== done ===")
	log.Printf("next steps:")
	log.Printf("  cd %s", newModule)
	log.Printf("  go mod tidy")
	log.Printf("  # verify: cd %s && go build ./...", newModule)
	log.Printf("  # test: docker compose -f %s/docker-compose.yml up", newModule)
	log.Printf("  #        curl http://localhost:%s/health", portStr)
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
		return fmt.Errorf("new_module must be snake_case (e.g. media_test)")
	}
	if !regexp.MustCompile(`^[a-z][a-z0-9-]*$`).MatchString(newApp) {
		return fmt.Errorf("new_app must be kebab-case (e.g. media-test)")
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
