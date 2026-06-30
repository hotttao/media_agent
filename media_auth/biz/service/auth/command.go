// command.go：auth 用例的 Command 类型（用例输入，非生成 DTO）。
package auth

// LoginCommand 是 Login 用例入参。
type LoginCommand struct {
	Username string
	Password string
}

// UpdateMeCommand 是 UpdateMe 用例入参。
// AutoEvolution 非 nil 表示显式传入（tri-state）；nil 表示未传（不改）。
type UpdateMeCommand struct {
	AutoEvolution *bool
}

// ChangePasswordCommand 是 ChangePassword 用例入参。
type ChangePasswordCommand struct {
	CurrentPassword string
	NewPassword     string
}

// SetDefaultPromptCommand 是 SetDefaultPrompt 用例入参。
// PromptID 为 *string：nil=清除，非空=设置（tri-state，对齐 spec optional 语义）。
type SetDefaultPromptCommand struct {
	PromptID *string
}
