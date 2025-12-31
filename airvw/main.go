package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/go-resty/resty/v2"
)

const (
	LevelBlock   = "block"   // 阻断级
	LevelHigh    = "high"    // 高风险
	LevelMedium  = "medium"  // 中风险
	LevelSuggest = "suggest" // 建议
)

// Config 综合配置结构体（新增评论目标/CommitID）
type Config struct {
	YunxiaoToken   string // 云效Token（x-yunxiao-token）
	OrgID          string // 组织ID（如67aaaaaaaaaa）
	RepoID         int    // 仓库ID（如5023797）
	MRID           int    // MR的ID（changeRequestId，评论MR时必填）
	FromCommit     string // 源提交ID（commit hash）
	ToCommit       string // 目标提交ID（commit hash）
	CodeupDomain   string // 云效域名，默认openapi-rdc.aliyuncs.com
	BaichuanAPIKey string // 阿里云百炼API Key
	ReviewLevel    string // 评审等级，默认block
	CommentTarget  string // 评论目标：mr（默认）/commit/空（不评论）
	CommitID       string // 评论Commit时的commit hash（comment-target=commit时必填）
}

// DiffItem 对应接口返回的diffs数组元素
type DiffItem struct {
	Diff        string `json:"diff"`        // 变更内容（diff格式）
	NewPath     string `json:"newPath"`     // 文件路径（新增/修改后）
	OldPath     string `json:"oldPath"`     // 原文件路径（重命名/删除时）
	NewFile     bool   `json:"newFile"`     // 是否为新增文件
	DeletedFile bool   `json:"deletedFile"` // 是否为删除文件
	RenamedFile bool   `json:"renamedFile"` // 是否为重命名文件
	Binary      bool   `json:"binary"`      // 是否为二进制文件
}

// CompareResponse 适配云效OpenAPI返回的结构体
type CompareResponse struct {
	Commits  []interface{} `json:"commits"`  // 提交记录（暂不使用）
	Diffs    []DiffItem    `json:"diffs"`    // 核心：变更文件列表
	Messages []string      `json:"messages"` //
}

var client = resty.New()

func maskSensitive(str string) string {
	if len(str) <= 6 {
		return "****"
	}
	return str[:6] + "****"
}

// 1. 拉取MR变更代码
func GetMRDiff(config Config) (map[string]string, error) {
	fmt.Println("=====================================")
	fmt.Println("【GetMRDiff】开始执行，配置详情：")
	fmt.Printf("  - YunxiaoToken: %s\n", maskSensitive(config.YunxiaoToken))
	fmt.Printf("  - OrgID: %s\n", config.OrgID)
	fmt.Printf("  - RepoID: %d\n", config.RepoID)
	fmt.Printf("  - MRID: %d\n", config.MRID)
	fmt.Printf("  - FromCommit: %s\n", config.FromCommit)
	fmt.Printf("  - ToCommit: %s\n", config.ToCommit)
	fmt.Printf("  - CodeupDomain: %s\n", config.CodeupDomain)
	fmt.Printf("  - BaichuanAPIKey: %s\n", maskSensitive(config.BaichuanAPIKey))
	fmt.Printf("  - ReviewLevel: %s\n", config.ReviewLevel)
	fmt.Printf("  - CommentTarget: %s\n", config.CommentTarget)
	fmt.Printf("  - CommitID: %s\n", config.CommitID)
	fmt.Println("=====================================")

	fmt.Println("🔍 开始拉取MR变更代码（云效OpenAPI）...")

	// 构建请求：核心修正 - 域名/Header/路径/参数
	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Accept", "application/json").
		SetQueryParams(map[string]string{
			"from": config.FromCommit, // from为提交ID
			"to":   config.ToCommit,   // to为提交ID
		}).
		// API路径（组织ID/仓库ID）
		Get(fmt.Sprintf("https://%s/oapi/v1/codeup/organizations/%s/repositories/%d/compares",
			config.CodeupDomain, config.OrgID, config.RepoID))

	if err != nil {
		fmt.Printf("❌【GetMRDiff】云效OpenAPI请求失败：%v\n", err)
		return nil, fmt.Errorf("云效OpenAPI请求失败：%w", err)
	}
	if resp.StatusCode() != 200 {
		fmt.Printf("❌【GetMRDiff】云效OpenAPI返回异常状态码：%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return nil, fmt.Errorf("云效OpenAPI返回异常状态码：%d，响应内容：%s",
			resp.StatusCode(), string(resp.Body()))
	}

	var compareResp CompareResponse
	if err := json.Unmarshal(resp.Body(), &compareResp); err != nil {
		fmt.Printf("❌【GetMRDiff】解析云效OpenAPI响应失败：%v，响应内容：%s\n", err, string(resp.Body()))
		return nil, fmt.Errorf("解析云效OpenAPI响应失败：%w，响应内容：%s", err, string(resp.Body()))
	}

	fmt.Printf("✅【GetMRDiff】成功拉取响应，共检测到%d个变更文件\n", len(compareResp.Diffs))

	// 过滤：仅保留新增/修改的Go文件
	diffMap := make(map[string]string)
	for _, diffItem := range compareResp.Diffs {
		// 跳过二进制文件
		if diffItem.Binary {
			fmt.Printf("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
			continue
		}

		// 确定文件路径（兼容重命名/删除场景）
		filePath := diffItem.NewPath
		if filePath == "" {
			filePath = diffItem.OldPath
		}

		// 确定文件状态
		var status string
		if diffItem.NewFile {
			status = "added"
		} else if diffItem.DeletedFile {
			status = "removed"
		} else if diffItem.RenamedFile {
			status = "renamed"
		} else {
			status = "modified"
		}

		// 仅保留新增/修改的Go文件
		if (status == "added" || status == "modified") && strings.HasSuffix(filePath, ".go") {
			diffMap[filePath] = diffItem.Diff
			fmt.Printf("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}

	if len(diffMap) == 0 {
		fmt.Println("ℹ️【GetMRDiff】未检测到新增/修改的Go文件，无需评审")
		return diffMap, nil
	}
	fmt.Printf("📌【GetMRDiff】共筛选出%d个需评审的Go文件\n", len(diffMap))
	return diffMap, nil
}

// 2. 执行golangci-lint规则检查（增加日志）
func RunGolangciLint(repoPath string, diffFiles map[string]string) map[string]string {
	fmt.Println("\n=====================================")
	fmt.Println("【RunGolangciLint】开始执行")
	fmt.Printf("  - 仓库路径：%s\n", repoPath)
	fmt.Printf("  - 待检查文件数：%d\n", len(diffFiles))
	fmt.Println("=====================================")

	lintResults := make(map[string]string)

	if _, err := exec.LookPath("golangci-lint"); err != nil {
		fmt.Println("⚠️【RunGolangciLint】未检测到golangci-lint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少golangci-lint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		fmt.Printf("ℹ️【RunGolangciLint】检查文件：%s\n", file)
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf("cd %s && golangci-lint run --new-from-rev=origin/main %s", repoPath, file))
		output, err := cmd.CombinedOutput()

		if err != nil {
			fmt.Printf("⚠️【RunGolangciLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			fmt.Printf("✅【RunGolangciLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			fmt.Printf("⚠️【RunGolangciLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

// 3. 调用阿里云百炼API进行AI代码评审（修复JSON格式 + 新增请求体日志）
func AICodeReview(config Config, diffFiles map[string]string, lintResults map[string]string) (string, []string, []string, error) {
	fmt.Println("\n=====================================")
	fmt.Println("【AICodeReview】开始执行")
	fmt.Printf("  - 待评审文件数：%d\n", len(diffFiles))
	fmt.Println("=====================================")

	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	prompt := fmt.Sprintf(`
你是资深Golang工程师，仅评审Codeup MR中新增/修改的Go代码，严格按以下要求输出：
1. 评审维度：并发安全、Error处理、内存优化、代码规范、逻辑漏洞、性能问题、内存泄漏、竞态检查；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)

	requestBody := map[string]interface{}{
		"model": "qwen3-coder-plus", //
		"input": map[string]interface{}{
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": prompt,
				},
			},
		},
		"parameters": map[string]interface{}{
			"max_new_tokens": 2000,
			"temperature":    0.2,
			"top_p":          0.9,
		},
	}

	// 新增：打印请求体（脱敏后），便于排查JSON格式问题
	requestBodyJSON, err := json.MarshalIndent(requestBody, "", "  ")
	if err != nil {
		fmt.Printf("❌【AICodeReview】构造请求体JSON失败：%v\n", err)
		return "", nil, nil, fmt.Errorf("构造请求体JSON失败：%w", err)
	}
	fmt.Printf("ℹ️【AICodeReview】构造的请求体：\n%s\n", string(requestBodyJSON))

	fmt.Println("ℹ️【AICodeReview】开始调用百炼原生API...")
	resp, err := client.R().
		SetHeader("Content-Type", "application/json"). // 强制指定JSON格式
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", config.BaichuanAPIKey)).
		SetBody(requestBody). // resty会自动序列化为合法JSON
		Post("https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation")

	if err != nil {
		fmt.Printf("❌【AICodeReview】百炼API调用失败：%v\n", err)
		return "", nil, nil, fmt.Errorf("百炼API调用失败：%w", err)
	}

	fmt.Printf("ℹ️【AICodeReview】百炼API响应状态码：%d\n", resp.StatusCode())
	fmt.Printf("ℹ️【AICodeReview】百炼API响应内容：%s\n", string(resp.Body()))

	// 解析百炼原生API响应
	var aiResp struct {
		Output struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
					Role    string `json:"role"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		} `json:"output"`
		Usage struct {
			TotalTokens  int `json:"total_tokens"`
			OutputTokens int `json:"output_tokens"`
			InputTokens  int `json:"input_tokens"`
		} `json:"usage"`
		RequestID string `json:"request_id"`
		Code      string `json:"code"`    // 错误码（成功时为空）
		Message   string `json:"message"` // 错误信息（成功时为空）
	}
	if err := json.Unmarshal(resp.Body(), &aiResp); err != nil {
		fmt.Printf("❌【AICodeReview】解析百炼API响应失败：%v，响应内容：%s\n", err, string(resp.Body()))
		return "", nil, nil, fmt.Errorf("解析百炼API响应失败：%w，响应内容：%s", err, string(resp.Body()))
	}

	// 检查百炼API是否返回错误
	if aiResp.Code != "" {
		fmt.Printf("❌【AICodeReview】百炼API返回业务错误：code=%s, message=%s\n", aiResp.Code, aiResp.Message)
		return "", nil, nil, fmt.Errorf("百炼API业务错误：%s - %s", aiResp.Code, aiResp.Message)
	}

	// 处理AI评审结果
	var aiResult string
	if len(aiResp.Output.Choices) > 0 {
		aiResult = strings.TrimSpace(aiResp.Output.Choices[0].Message.Content)
	}
	fmt.Printf("✅【AICodeReview】百炼API调用成功，RequestID：%s\n", aiResp.RequestID)
	fmt.Printf("ℹ️【AICodeReview】Token使用情况：Total=%d, Input=%d, Output=%d\n",
		aiResp.Usage.TotalTokens, aiResp.Usage.InputTokens, aiResp.Usage.OutputTokens)
	fmt.Printf("ℹ️【AICodeReview】AI评审结果：%s\n", aiResult)

	// 提取阻断级和高级别问题
	var blockIssues []string
	var highIssues []string
	if aiResult != "✅ 未发现任何问题" {
		lines := strings.Split(aiResult, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if strings.Contains(line, fmt.Sprintf("[%s]", LevelBlock)) {
				blockIssues = append(blockIssues, line)
				fmt.Printf("❌【AICodeReview】检测到阻断级问题：%s\n", line)
			} else if strings.Contains(line, fmt.Sprintf("[%s]", LevelHigh)) {
				highIssues = append(highIssues, line)
				fmt.Printf("⚠️【AICodeReview】检测到高级别问题：%s\n", line)
			}
		}
	}

	fmt.Printf("📊【AICodeReview】AI评审完成，检测到%d个阻断级问题，%d个高级别问题\n", len(blockIssues), len(highIssues))
	return aiResult, blockIssues, highIssues, nil
}

// 4. 将评审结果评论到Codeup MR
func CommentMR(config Config, reviewResult string) error {
	fmt.Println("\n=====================================")
	fmt.Println("【CommentMR】开始执行")
	fmt.Printf("  - MRID：%d\n", config.MRID)
	fmt.Println("=====================================")

	// 构造符合官方要求的评论内容
	commentBody := fmt.Sprintf(`
### 🤖 AI Code Review 结果（MR #%d）
#### 评审范围：提交ID %s → %s 变更的Go文件
#### 问题等级说明：
- [%s]：阻断级，必须修复才能合并
- [%s]：高风险，建议优先修复
- [%s]：中风险，建议修复
- [%s]：优化建议，不强制

---
%s`, config.MRID, config.FromCommit, config.ToCommit,
		LevelBlock, LevelHigh, LevelMedium, LevelSuggest, reviewResult)

	// 构建请求：完全匹配官方文档规范
	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"content": commentBody,
			// 可选参数（如需回复特定评论，可添加parentId）
			// "parentId": 0,
		}).
		// 官方指定的API路径：change_requests/{changeRequestId}/comments
		Post(fmt.Sprintf("https://%s/oapi/v1/codeup/change_requests/%d/comments",
			config.CodeupDomain, config.MRID))

	if err != nil {
		fmt.Printf("❌【CommentMR】创建MR评论API调用失败：%v\n", err)
		return fmt.Errorf("创建MR评论API调用失败：%w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		fmt.Printf("❌【CommentMR】创建MR评论失败：状态码%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return fmt.Errorf("创建MR评论失败：状态码%d，响应内容：%s", resp.StatusCode(), string(resp.Body()))
	}

	// 解析响应（可选，验证评论是否创建成功）
	var commentResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &commentResp); err != nil {
		fmt.Printf("⚠️【CommentMR】解析MR评论响应失败（但评论已提交）：%s\n", err)
	} else {
		fmt.Printf("✅【CommentMR】评审结果评论成功，评论ID：%v\n", commentResp["id"])
	}

	return nil
}

// 5. 将评审结果评论到Codeup Commit
func CommentCommit(config Config, reviewResult string) error {
	fmt.Println("\n=====================================")
	fmt.Println("【CommentCommit】开始执行")
	fmt.Printf("  - OrgID：%s\n", config.OrgID)
	fmt.Printf("  - RepoID：%d\n", config.RepoID)
	fmt.Printf("  - CommitID：%s\n", config.CommitID)
	fmt.Printf("  - reviewResult：%s\n", reviewResult)
	fmt.Println("=====================================")

	if reviewResult == "" {
		fmt.Println("ℹ️【CommentCommit】AI评审结果为空，跳过评论提交")
		return nil
	}
	// 构造Commit评论内容（适配Commit场景的文案）
	commentBody := fmt.Sprintf(`
### 🤖 AI Code Review 结果（Commit %s）
#### 评审范围：提交ID %s → %s 变更的Go文件
#### 问题等级说明：
- [%s]：阻断级，必须修复
- [%s]：高风险，建议优先修复
- [%s]：中风险，建议修复
- [%s]：优化建议，不强制

---
%s`, config.CommitID, config.FromCommit, config.ToCommit,
		LevelBlock, LevelHigh, LevelMedium, LevelSuggest, reviewResult)

	// 构建请求：完全匹配云效创建Commit评论的官方API规范
	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Content-Type", "application/json").
		// 官方要求的请求体：仅需content字段
		SetBody(map[string]interface{}{
			"content": commentBody,
		}).
		// 官方指定的API路径：organizations/{orgId}/repositories/{repoId}/commits/{commitId}/comments
		Post(fmt.Sprintf("https://%s/oapi/v1/codeup/organizations/%s/repositories/%d/commits/%s/comments",
			config.CodeupDomain, config.OrgID, config.RepoID, config.CommitID))

	// 错误处理：请求失败
	if err != nil {
		fmt.Printf("❌【CommentCommit】创建Commit评论API调用失败：%v\n", err)
		return fmt.Errorf("创建Commit评论API调用失败：%w", err)
	}

	// 错误处理：非200/201状态码（兼容官方常见成功状态码）
	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		// 新增403权限错误的友好提示
		if resp.StatusCode() == 403 {
			fmt.Printf("❌【CommentCommit】创建Commit评论失败：Token权限不足！\n")
			fmt.Printf("   解决方案：\n")
			fmt.Printf("   1. 登录云效控制台 → 个人设置 → 访问令牌，检查Token权限\n")
			fmt.Printf("   2. 确保Token包含Codeup仓库的写权限和Commit评论权限\n")
			fmt.Printf("   3. 确认你的账号对目标仓库有开发者及以上权限\n")
		}
		fmt.Printf("❌【CommentCommit】创建Commit评论失败：状态码%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return fmt.Errorf("创建Commit评论失败：状态码%d，响应内容：%s", resp.StatusCode(), string(resp.Body()))
	}

	// 优化解析逻辑：先检查响应体是否为空，再解析
	fmt.Printf("✅【CommentCommit】Commit评论提交成功（状态码：%d）\n", resp.StatusCode())
	respBody := string(resp.Body())
	if respBody == "" {
		fmt.Println("ℹ️【CommentCommit】云效返回空响应体，跳过JSON解析（评论已提交）")
		return nil
	}

	// 解析响应（验证评论是否创建成功）
	var commentResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &commentResp); err != nil {
		fmt.Printf("ℹ️【CommentCommit】解析响应失败（但评论已提交）：%s，响应体：%s\n", err, respBody)
		return nil // 解析失败不返回错误，因为核心功能（评论提交）已完成
	}

	// 解析成功则打印评论ID
	fmt.Printf("✅【CommentCommit】评审结果评论成功，评论ID：%v\n", commentResp["id"])
	return nil
}

// 帮助信息
func printUsage() {
	usage := `
🚀 airvw - AI驱动的Codeup Go代码评审工具
=====================***=======================
功能：自动拉取Codeup MR/Commit的Go代码变更，执行golangci-lint检查，调用阿里云百炼AI评审，
      支持将评审结果评论到MR/Commit，阻断级问题直接终止流程。

📦 安装方式：
  go install github.com/你的用户名/airvw@latest

🔧 使用方式：
  airvw [参数]

📋 参数说明：
  必选参数：
    --yunxiao-token string    云效Token（x-yunxiao-token，必填）
    --org-id string           组织ID（如67aaaaaaaaaa，必填）
    --repo-id int             仓库ID（如5023797，必填）
    --from-commit string      源提交ID（commit hash，必填）
    --to-commit string        目标提交ID（commit hash，必填）
    --baichuan-key string     阿里云百炼API Key（必填）

  可选参数：
    --domain string           云效域名（默认：openapi-rdc.aliyuncs.com）
    --level string            评审等级（默认：block，可选：block/high/medium/suggest）
    --comment-target string   评论目标（可选：mr/commit/空，空则不评论）
    --mr-id int               MR的ID（comment-target=mr时必填）
    --commit-id string        Commit的hash（comment-target=commit时必填）
    --help                    显示此帮助信息

💡 使用示例：
  1. 仅执行AI评审（不评论）：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx

  2. 评审并评论到MR：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --mr-id 12345 --from-commit xxxxxx --to-commit xxxxxx \
           --baichuan-key sk-xxx --comment-target mr

  3. 评审并评论到Commit：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --commit-id 2b4f8fc38bdf464359c3a05334654fa27e15a704 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --comment-target commit

⚠️ 注意事项：
  1. 需提前安装golangci-lint（可选，未安装则跳过规则检查）
  2. 百炼API Key需具备文本生成权限
  3. 云效Token需具备Codeup MR/Commit评论权限
  4. 仅评审新增/修改的.go文件，二进制文件、删除/重命名文件会被过滤
`
	fmt.Println(usage)
}

// 主函数：整合所有流程（增加评论目标逻辑）
func main() {
	// 自定义help信息
	flag.Usage = printUsage

	fmt.Println("🚀 开始执行AI Code Review流程...")

	var config Config
	flag.StringVar(&config.YunxiaoToken, "yunxiao-token", "", "云效Token（x-yunxiao-token，必填）")
	flag.StringVar(&config.OrgID, "org-id", "", "组织ID（如67aaaaaaaaaa，必填）")
	flag.IntVar(&config.RepoID, "repo-id", 0, "仓库ID（如5023797，必填）")
	flag.IntVar(&config.MRID, "mr-id", 0, "MR的ID（changeRequestId，评论MR时必填）")
	flag.StringVar(&config.FromCommit, "from-commit", "", "源提交ID（commit hash，必填）")
	flag.StringVar(&config.ToCommit, "to-commit", "", "目标提交ID（commit hash，必填）")
	flag.StringVar(&config.CodeupDomain, "domain", "openapi-rdc.aliyuncs.com", "云效域名（可选）")
	flag.StringVar(&config.BaichuanAPIKey, "baichuan-key", "", "阿里云百炼API Key（必填）")
	flag.StringVar(&config.ReviewLevel, "level", LevelBlock, "评审等级（block/high/medium/suggest）")
	flag.StringVar(&config.CommentTarget, "comment-target", "", "评论目标：mr（评论MR）/commit（评论Commit）/空（不评论）")
	flag.StringVar(&config.CommitID, "commit-id", "", "评论Commit时的commit hash（comment-target=commit时必填）")
	flag.Parse()

	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		os.Exit(0)
	}

	// 打印参数接收日志
	fmt.Println("\n=====================================")
	fmt.Println("【airvw】命令行参数解析完成")
	fmt.Println("=====================================")

	// 强化参数校验（按评论目标区分必填项）
	var missingParams []string
	if config.YunxiaoToken == "" {
		missingParams = append(missingParams, "yunxiao-token")
	}
	if config.OrgID == "" {
		missingParams = append(missingParams, "org-id")
	}
	if config.RepoID == 0 {
		missingParams = append(missingParams, "repo-id")
	}
	if config.FromCommit == "" {
		missingParams = append(missingParams, "from-commit")
	}
	if config.ToCommit == "" {
		missingParams = append(missingParams, "to-commit")
	}
	if config.BaichuanAPIKey == "" {
		missingParams = append(missingParams, "baichuan-key")
	}

	// 仅当评论目标为mr/commit时，校验对应的专属参数
	if config.CommentTarget == "mr" && config.MRID == 0 {
		missingParams = append(missingParams, "mr-id（评论MR时必填）")
	}
	if config.CommentTarget == "commit" && config.CommitID == "" {
		missingParams = append(missingParams, "commit-id（评论Commit时必填）")
	}

	// 输出缺失参数并退出
	if len(missingParams) > 0 {
		fmt.Printf("❌【airvw】错误：缺少必填参数：%s\n", strings.Join(missingParams, ", "))
		printUsage()
		os.Exit(1)
	}

	// 步骤1：拉取MR变更代码
	diffFiles, err := GetMRDiff(config)
	if err != nil {
		fmt.Printf("❌【airvw】拉取MR变更失败：%s\n", err)
		os.Exit(1)
	}
	if len(diffFiles) == 0 {
		fmt.Println("✅【airvw】无变更的Go文件，评审通过")
		os.Exit(0)
	}

	// 步骤2：执行golangci-lint规则检查
	lintResults := RunGolangciLint(".", diffFiles)

	// 步骤3：AI代码评审
	aiResult, blockIssues, highIssues, err := AICodeReview(config, diffFiles, lintResults)
	if err != nil {
		fmt.Printf("❌【airvw】AI评审失败：%s\n", err)
		os.Exit(1)
	}

	// 步骤4：仅当评论目标为mr/commit时，执行评论操作；否则跳过
	var commentErr error
	switch config.CommentTarget {
	case "mr":
		commentErr = CommentMR(config, aiResult)
	case "commit":
		commentErr = CommentCommit(config, aiResult)
	default:
		fmt.Println("ℹ️【airvw】未指定有效评论目标（mr/commit），跳过评论操作")
	}
	if commentErr != nil {
		fmt.Printf("⚠️【airvw】评论%s失败（不终止评审）：%s\n", config.CommentTarget, commentErr)
	}

	// 根据评审等级判断是否终止流程
	var shouldBlock bool
	var blockReason string
	var blockList []string

	if config.ReviewLevel == LevelBlock && len(blockIssues) > 0 {
		shouldBlock = true
		blockReason = "阻断级"
		blockList = blockIssues
	} else if config.ReviewLevel == LevelHigh && (len(blockIssues) > 0 || len(highIssues) > 0) {
		shouldBlock = true
		blockReason = "高级别"
		blockList = append(blockIssues, highIssues...)
	}

	if shouldBlock {
		fmt.Printf("\n❌【airvw】检测到%d个%s问题，终止流程！\n", len(blockList), blockReason)
		for _, issue := range blockList {
			fmt.Printf("  - %s\n", issue)
		}
		os.Exit(1)
	}

	fmt.Printf("\n✅【airvw】所有评审完成，无阻断级问题，评审通过！（评论目标：%s）\n", config.CommentTarget)
	os.Exit(0)
}
