// argon2hash 是一个小工具：对命令行传入的明文密码生成 argon2id hash。
// 供 e2e seed.sh 调用（seed 测试用户时需要 argon2 hash）。
//
// 用法：go run ./tools/argon2hash <password>
package main

import (
	"fmt"
	"os"

	"media_agent/media_auth/biz/shared/argon2"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: argon2hash <password>")
		os.Exit(1)
	}
	hash, err := argon2.Hash(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "hash: %v\n", err)
		os.Exit(1)
	}
	fmt.Print(hash)
}
