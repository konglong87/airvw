package utils

import (
	"fmt"
	"regexp"
	"testing"
)

// [suggest] service/breathe/patients.go:557 - 魔法数字-1硬编码，缺乏常量定义或注释说明其含义 - 建议定义常量如const InvalidDiagnosisYear = -1并添加注释说明其业务含义
// [suggest] mod/src/main/java/com/home/entering/EnteringPefAnalysisFragment.kt:164 - try-catch捕获通用Exception可能掩盖具体错误类型 - 建议捕获具体的异常类型如Resources.NotFoundException
const LogPattern1 = `\[([^\]]+)\]\s+([^:]+):(\d+)\s+-\s+([^\-]+)\s+-\s+(.+)`
const LogPattern2 = `\[([^\]]+)\]\s+([^:]+):(\d+)\s+-\s+([^\-]+)\s+-\s+(.+)`
const LogPattern3 = `\[([^\]]+)\]\s*([^:]+):\s*(\d+)\s*-\s*([^\-]+)\s*-\s*(.+)`

func TestMust1(t *testing.T) {
	issue01 := `[suggest] service/breathe/patients.go:557 - 魔法数字-1硬编码，缺乏常量定义或注释说明其含义 - 建议定义常量如const InvalidDiagnosisYear = -1并添加注释说明其业务含义`
	issue02 := `[suggest] /src/main/java/com/home/entering/EnteringPefAnalysisFragment.kt:164 - try-catch捕获通用Exception可能掩盖具体错误类型 - 建议捕获具体的异常类型如Resources.NotFoundException`
	issueArr := []string{issue01, issue02}

	for _, issue := range issueArr {
		re := regexp.MustCompile(LogPattern3)
		matches := re.FindStringSubmatch(issue)
		if len(matches) == 6 {
			fmt.Println("must succ:", matches)
		} else {
			fmt.Println("must fail File:", issue)
		}
	}
}

// TestTriangle 测试三角形分类问题
func TestTriangle(t *testing.T) {
	issue01 := `[suggest] service/breathe/patients.go:557 - 魔法数字-1硬编码，缺乏常量定义或注释说明其含义 - 建议定义常量如const InvalidDiagnosisYear = -1并添加注释说明其业务含义`
	issue02 := `[suggest] /src/main/java/com/home/entering/EnteringPefAnalysisFragment.kt:164 - try-catch捕获通用Exception可能掩盖具体错误类型 - 建议捕获具体的异常类型如Resources.NotFoundException`
	issueArr := []string{issue01, issue02}
	type testCase struct {
		Name       string
		LogPattern string
		want       bool
	}

	// 测试用例表
	tests := []testCase{
		{"LogPattern1", LogPattern1, false},
		{"LogPattern2", LogPattern2, false},
		{"LogPattern3", LogPattern3, true},
	}

	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			//got := classifyIssueArr(tt.LogPattern, issueArr)
			//if got != tt.want {
			//	t.Errorf("classifyIssueArr(%d, %d, %d) = %v, want %v", tt.a, tt.b, tt.c, got, tt.want)
			//}
		})
	}
	_ = issueArr
}
