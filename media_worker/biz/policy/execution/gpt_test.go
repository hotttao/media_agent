package execution

import "testing"

func TestGPTWebParameters(t *testing.T) {
	for _, s := range []string{"auto", "1024x1024", "1536x1024", "1024x1536"} {
		if ValidateGPTParameters(s, "Thinking", "") != nil {
			t.Fatal(s)
		}
	}
	for _, args := range [][3]string{{"unsupported", "Thinking", ""}, {"auto", "", ""}, {"auto", "Thinking", "https://evil.example/project"}, {"auto", "Thinking", "https://chatgpt.com/g/g-p-valid/project?token=secret"}} {
		if ValidateGPTParameters(args[0], args[1], args[2]) == nil {
			t.Fatal("unsupported config accepted")
		}
	}
}
