// suite.go — e2e 测试套件框架（从外部目录加载测试脚本，可扩展）。
//
// media-cli e2e run：tearup 环境 → 执行 seed 脚本 → 顺序执行测试套件 → teardown。
// 测试套件从外部目录加载（每个 .sh 文件一个测试），不硬编码在 Go 里。
//
// 目录结构（media-cli/e2e/）：
//
//	e2e/
//	├── seed.sh              # 数据准备（如 seed 测试用户）
//	├── auth-login-me.sh     # 测试：login → me
//	├── auth-change-pw.sh    # 测试：change password
//	└── ...                  # 后续新增测试只需加 .sh 文件
//
// 每个 .sh 文件返回 0 = PASS，非 0 = FAIL。可用 media-cli 子命令 + curl 等。
// suite.go 只管编排（按序跑 + 汇总），测试逻辑在 .sh 文件里，灵活可扩展。
package e2e

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// runCmd media-cli e2e run [--dir e2e] [--services media_auth] [--skip-seed]
// tearup 被测服务 → seed → 测试套件 → teardown（一体化）。
func runCmd() *cobra.Command {
	var testDir string
	var skipSeed bool
	var services string
	c := &cobra.Command{
		Use:   "run",
		Short: "tearup → seed → 测试套件 → teardown（一体化 e2e）",
		RunE: func(c *cobra.Command, args []string) error {
			ctx := context.Background()
			env := NewEnv(parseServices(services))

			// 1. tearup
			fmt.Println("=== tearup ===")
			if err := env.Up(ctx); err != nil {
				_ = env.Down(ctx)
				return fmt.Errorf("tearup: %w", err)
			}

			// 无论成功失败都 teardown
			defer func() {
				fmt.Println("=== teardown ===")
				_ = env.Down(ctx)
			}()

			// 2. seed（数据准备）
			if !skipSeed {
				seedPath := filepath.Join(testDir, "seed.sh")
				if _, err := os.Stat(seedPath); err == nil {
					fmt.Println("=== seed ===")
					if err := runScript(seedPath); err != nil {
						return fmt.Errorf("seed failed: %w", err)
					}
					fmt.Println("   ✅ seed done")
				}
			}

			// 3. 测试套件（从目录加载 *.sh，排除 seed.sh）
			tests, err := loadTests(testDir)
			if err != nil {
				return fmt.Errorf("load tests: %w", err)
			}
			if len(tests) == 0 {
				fmt.Println("=== no tests found ===")
				return nil
			}

			fmt.Println("=== tests ===")
			failed := 0
			for _, t := range tests {
				fmt.Printf(">> %s ... ", filepath.Base(t))
				if err := runScript(t); err != nil {
					fmt.Printf("❌ FAIL: %v\n", err)
					failed++
				} else {
					fmt.Println("✅ PASS")
				}
			}

			if failed > 0 {
				return fmt.Errorf("%d/%d tests failed", failed, len(tests))
			}
			fmt.Printf("=== all %d tests passed ===\n", len(tests))
			return nil
		},
	}
	c.Flags().StringVar(&testDir, "dir", "e2e", "测试目录（含 seed.sh + *_test.sh）")
	c.Flags().BoolVar(&skipSeed, "skip-seed", false, "跳过 seed.sh")
	c.Flags().StringVar(&services, "services", "", "被测服务（逗号分隔；空=全部）")
	return c
}

// loadTests 从目录加载测试脚本（*.sh，排除 seed.sh），按文件名排序。
func loadTests(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var tests []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".sh") {
			continue
		}
		if name == "seed.sh" {
			continue
		}
		tests = append(tests, filepath.Join(dir, name))
	}
	sort.Strings(tests)
	return tests, nil
}

// runScript 执行 shell 脚本，返回 error 表示非 0 退出码。
func runScript(path string) error {
	cmd := exec.Command("bash", path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}
