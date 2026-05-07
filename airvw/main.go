package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/blinkbean/dingtalk"
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
	Language       string // 评审语言：golang/java/python/javascript（默认golang）
	Model          string // AI模型名称，默认qwen3-coder-plus
	Debug          bool   // 是否开启调试模式，默认false
	DingTalkToken  string // 钉钉机器人Token
	DingTalkSecret string // 钉钉机器人Secret
	EnableDingTalk bool   // 是否启用钉钉通知，默认false
	MaxIssues      int    // 钉钉通知中显示的最大问题数量，默认10
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

// CompareResponseV2 适配最新版云效OpenAPI返回的结构体
type CompareResponseV2 struct {
	Commits []struct {
		AuthorEmail    string    `json:"authorEmail"`
		AuthorName     string    `json:"authorName"`
		AuthoredDate   time.Time `json:"authoredDate"`
		CommittedDate  time.Time `json:"committedDate"`
		CommitterEmail string    `json:"committerEmail"`
		CommitterName  string    `json:"committerName"`
		Id             string    `json:"id"`
		Message        string    `json:"message"`
		ParentIds      []string  `json:"parentIds"`
		ShortId        string    `json:"shortId"`
		Stats          struct {
			Additions int `json:"additions"`
			Deletions int `json:"deletions"`
			Total     int `json:"total"`
		} `json:"stats"`
		Title string `json:"title"`
	} `json:"commits"`
	Diffs []struct {
		AMode       string `json:"aMode"`
		BMode       string `json:"bMode"`
		DeletedFile bool   `json:"deletedFile"`
		Diff        string `json:"diff"`
		IsBinary    bool   `json:"isBinary"`
		NewFile     bool   `json:"newFile"`
		NewId       string `json:"newId"`
		NewPath     string `json:"newPath"`
		OldId       string `json:"oldId"`
		OldPath     string `json:"oldPath"`
		RenamedFile bool   `json:"renamedFile"`
	} `json:"diffs"`
}

var client = resty.New()
var debugMode = false // 全局调试模式标志

// logDebug 仅在debug模式下输出日志
func logDebug(format string, args ...interface{}) {
	if debugMode {
		fmt.Printf(format, args...)
	}
}

// logDebugln 仅在debug模式下输出日志（带换行）
func logDebugln(args ...interface{}) {
	if debugMode {
		fmt.Println(args...)
	}
}

// BlockIssue 阻断问题结构体
type BlockIssue struct {
	Level      string `json:"level"`      // 问题等级
	File       string `json:"file"`       // 文件名
	Line       string `json:"line"`       // 行号
	Issue      string `json:"issue"`      // 问题描述
	Suggestion string `json:"suggestion"` // 修复建议
}

// ReviewResult 评审结果结构体
type ReviewResult struct {
	Status      string       `json:"status"`                 // 状态: success/blocked
	TotalIssues int          `json:"total_issues"`           // 总问题数
	BlockReason string       `json:"block_reason,omitempty"` // 阻断原因
	BlockIssues []BlockIssue `json:"block_issues,omitempty"` // 阻断问题列表
	Message     string       `json:"message"`                // 消息
	CommitInfo  *CommitInfo  `json:"commit_info,omitempty"`  // Commit信息
	Model       string       `json:"model,omitempty"`        // 使用的AI模型
}

// CommitInfo Commit信息结构体
type CommitInfo struct {
	AuthorName string `json:"author_name"` // 提交人姓名
	Message    string `json:"message"`     // 提交消息
}

// formatBlockIssues 将问题字符串转换为结构化的BlockIssue，并按重要性等级排序
func formatBlockIssues(issues []string) []BlockIssue {
	var blockIssues []BlockIssue
	for _, issue := range issues {
		// 解析格式: [等级] 文件名:行号 - 问题描述 - 修复建议
		re := regexp.MustCompile(`\[([^\]]+)\]\s*([^:]+):\s*(\d+)\s*-\s*([^\-]+)\s*-\s*(.+)`)
		matches := re.FindStringSubmatch(issue)
		if len(matches) == 6 {
			blockIssues = append(blockIssues, BlockIssue{
				Level:      matches[1],
				File:       matches[2],
				Line:       matches[3],
				Issue:      strings.TrimSpace(matches[4]),
				Suggestion: strings.TrimSpace(matches[5]),
			})
		} else {
			// 如果无法解析，则将整个字符串作为问题描述
			blockIssues = append(blockIssues, BlockIssue{
				Level:      "unknown",
				File:       "unknown",
				Line:       "0",
				Issue:      issue,
				Suggestion: "",
			})
		}
	}
	// 按重要性等级排序：block > high > medium > suggest > unknown
	levelPriority := map[string]int{
		LevelBlock:   0,
		LevelHigh:    1,
		LevelMedium:  2,
		LevelSuggest: 3,
		"unknown":    4,
	}

	sort.Slice(blockIssues, func(i, j int) bool {
		priorityI, okI := levelPriority[blockIssues[i].Level]
		priorityJ, okJ := levelPriority[blockIssues[j].Level]
		if !okI {
			priorityI = 4
		}
		if !okJ {
			priorityJ = 4
		}
		return priorityI < priorityJ
	})

	return blockIssues
}

// printJSONResult 以JSON格式输出评审结果
func printJSONResult(result ReviewResult) {
	jsonData, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fmt.Printf("❌【aiutoCR】JSON格式化失败：%s\n", err)
		return
	}
	fmt.Println(string(jsonData))
}

// DingDingRemind 发送钉钉消息通知
func DingDingRemind(token, secret, content string, maxIssues int) {
	// 初始化钉钉客户端（自动处理加签逻辑）
	cli := dingtalk.InitDingTalkWithSecret(token, secret)

	// 解析ReviewResult
	var result ReviewResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		logDebug("解析评审结果失败: %v \n", err)
		return
	}

	// 构建Markdown消息
	var markdown strings.Builder
	markdown.WriteString("## AI代码审查结果通知\n\n")

	// 添加状态
	if result.Status == "blocked" {
		markdown.WriteString("### ❌ 评审被阻断\n\n")
	} else {
		markdown.WriteString("### ✅ 评审通过\n\n")
	}

	// 添加commit信息
	if result.CommitInfo != nil {
		markdown.WriteString("### 📝 Commit信息\n\n")
		markdown.WriteString(fmt.Sprintf("- **提交人**: %s\n", result.CommitInfo.AuthorName))
		markdown.WriteString(fmt.Sprintf("- **提交消息**: %s\n", result.CommitInfo.Message))
		//markdown.WriteString(fmt.Sprintf("- **Web链接**: [%s](%s)\n\n", result.CommitInfo.WebUrl, result.CommitInfo.WebUrl))
	}

	// 添加评审结果
	markdown.WriteString("### 📊 评审结果\n\n")
	markdown.WriteString(fmt.Sprintf("- **状态**: %s\n", result.Status))
	markdown.WriteString(fmt.Sprintf("- **问题数量**: %d\n", result.TotalIssues))
	if result.BlockReason != "" {
		markdown.WriteString(fmt.Sprintf("- **阻断原因**: %s\n", result.BlockReason))
	}
	markdown.WriteString(fmt.Sprintf("- **消息**: %s\n\n", result.Message))

	// 添加AI模型信息
	if result.Model != "" {
		markdown.WriteString("### 🤖 AI模型\n\n")
		markdown.WriteString(fmt.Sprintf("- **模型**: %s\n\n", result.Model))
	}

	// 添加问题详情
	if len(result.BlockIssues) > 0 {
		markdown.WriteString("### 🐛 问题和建议\n\n")
		// 限制显示的问题数量
		issuesToShow := result.BlockIssues
		if maxIssues > 0 && len(issuesToShow) > maxIssues {
			issuesToShow = issuesToShow[:maxIssues]
			markdown.WriteString(fmt.Sprintf("**注意**: 仅显示前%d个问题（共%d个）\n\n", maxIssues, result.TotalIssues))
		}
		for i, issue := range issuesToShow {
			markdown.WriteString(fmt.Sprintf("**%d. [%s] %s:%s**\n\n", i+1, issue.Level, issue.File, issue.Line))
			markdown.WriteString(fmt.Sprintf("- 问题描述: %s\n", issue.Issue))
			if issue.Suggestion != "" {
				markdown.WriteString(fmt.Sprintf("- 修复建议: %s\n", issue.Suggestion))
			}
			markdown.WriteString("\n")
		}
	}

	// 发送Markdown消息，支持@所有人（也可自定义@指定人）
	// 第一个参数是Markdown消息的标题，第二个是内容，第三个是可选配置（如@所有人）
	err := cli.SendMarkDownMessage("AI代码审查结果通知", markdown.String(), dingtalk.WithAtAll())
	//err := cli.SendMarkdownMessage("AI代码审查结果通知", markdown.String(), dingtalk.WithAtAll())
	if err != nil {
		logDebugln("钉钉机器人发送失败: %v\n", err)
		return
	}
	logDebugln("钉钉消息发送成功！")
}

// ReviewProcess 代码评审流程接口
type ReviewProcess interface {
	// GetFileExtension 获取需要评审的文件扩展名
	GetFileExtension() string
	// GetPrompt 获取AI评审的prompt
	GetPrompt(diffFiles map[string]string, lintResults map[string]string) string
	// RunLint 执行代码静态检查
	RunLint(repoPath string, diffFiles map[string]string) map[string]string
	// FilterFiles 过滤需要评审的文件
	FilterFiles(diffItems []DiffItem) map[string]string
}

// GolangReviewProcess Golang语言的评审流程实现
type GolangReviewProcess struct{}

func (g *GolangReviewProcess) GetFileExtension() string {
	return ".go"
}

func (g *GolangReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深Golang工程师，仅评审Codeup MR中新增/修改的Go代码，严格按以下要求输出：
1. 评审维度：并发安全、Error处理、内存优化、代码规范、逻辑漏洞、性能问题、内存泄漏、竞态检查、空指针解引用、内存溢出；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (g *GolangReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	logDebugln("\n=====================================")
	logDebugln("【RunGolangciLint】开始执行")
	logDebug("  - 仓库路径：%s\n", repoPath)
	logDebug("  - 待检查文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	lintResults := make(map[string]string)

	if _, err := exec.LookPath("golangci-lint"); err != nil {
		logDebugln("⚠️【RunGolangciLint】未检测到golangci-lint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少golangci-lint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		logDebug("ℹ️【RunGolangciLint】检查文件：%s\n", file)
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf("cd %s && golangci-lint run --new-from-rev=origin/main %s", repoPath, file))
		output, err := cmd.CombinedOutput()

		if err != nil {
			logDebug("⚠️【RunGolangciLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			logDebug("✅【RunGolangciLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			logDebug("⚠️【RunGolangciLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

func (g *GolangReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

// JavaReviewProcess Java语言的评审流程实现
type JavaReviewProcess struct{}

func (j *JavaReviewProcess) GetFileExtension() string {
	return ".java"
}

func (j *JavaReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深Java工程师，仅评审Codeup MR中新增/修改的Java代码，严格按以下要求输出：
1. 评审维度：并发安全、异常处理、内存优化、代码规范、逻辑漏洞、性能问题、资源泄漏、空指针异常、集合使用、线程安全；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (j *JavaReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	fmt.Println("\n=====================================")
	fmt.Println("【RunJavaLint】开始执行")
	fmt.Printf("  - 仓库路径：%s\n", repoPath)
	fmt.Printf("  - 待检查文件数：%d\n", len(diffFiles))
	fmt.Println("=====================================")

	lintResults := make(map[string]string)

	// 检查是否安装了Checkstyle
	if _, err := exec.LookPath("checkstyle"); err != nil {
		fmt.Println("⚠️【RunJavaLint】未检测到checkstyle，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少checkstyle环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		fmt.Printf("ℹ️【RunJavaLint】检查文件：%s\n", file)
		cmd := exec.Command("checkstyle", "-c", "/google_checks.xml", file)
		output, err := cmd.CombinedOutput()

		if err != nil {
			fmt.Printf("⚠️【RunJavaLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			fmt.Printf("✅【RunJavaLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			fmt.Printf("⚠️【RunJavaLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

func (j *JavaReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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

		// 仅保留新增/修改的Java文件
		if (status == "added" || status == "modified") && strings.HasSuffix(filePath, ".java") {
			diffMap[filePath] = diffItem.Diff
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

// PythonReviewProcess Python语言的评审流程实现
type PythonReviewProcess struct{}

func (p *PythonReviewProcess) GetFileExtension() string {
	return ".py"
}

// JavaScriptReviewProcess JavaScript语言的评审流程实现
type JavaScriptReviewProcess struct{}

func (j *JavaScriptReviewProcess) GetFileExtension() string {
	return ".js/.jsx/.ts/.tsx"
}

// KotlinReviewProcess Kotlin语言的评审流程实现
type KotlinReviewProcess struct{}

func (k *KotlinReviewProcess) GetFileExtension() string {
	return ".kt"
}

// SwiftReviewProcess Swift语言的评审流程实现
type SwiftReviewProcess struct{}

func (s *SwiftReviewProcess) GetFileExtension() string {
	return ".swift"
}

func (p *PythonReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深Python工程师，仅评审Codeup MR中新增/修改的Python代码，严格按以下要求输出：
1. 评审维度：异常处理、代码规范(PEP8)、逻辑漏洞、性能问题、资源泄漏、类型注解、导入管理、文档字符串；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (s *SwiftReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深Swift工程师，仅评审Codeup MR中新增/修改的Swift代码，严格按以下要求输出：
1. 评审维度：内存管理、可选项处理、并发安全、错误处理、代码规范、逻辑漏洞、性能问题、资源泄漏、类型安全、协议使用；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (j *JavaScriptReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深JavaScript/TypeScript工程师，仅评审Codeup MR中新增/修改的JavaScript/TypeScript代码，严格按以下要求输出：
1. 评审维度：异步编程、错误处理、代码规范(ESLint)、逻辑漏洞、性能问题、内存泄漏、DOM操作、事件处理、跨浏览器兼容性、TypeScript类型安全、JavaScript类型安全、React组件规范；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (k *KotlinReviewProcess) GetPrompt(diffFiles map[string]string, lintResults map[string]string) string {
	var reviewContent string
	for file, content := range diffFiles {
		reviewContent += fmt.Sprintf("=== 文件：%s ===\n规则检查结果：%s\n代码变更内容：\n%s\n\n",
			file, lintResults[file], content)
	}

	return fmt.Sprintf(`
你是资深Kotlin工程师，仅评审Codeup MR中新增/修改的Kotlin代码，严格按以下要求输出：
1. 评审维度：空安全、协程使用、异常处理、内存优化、代码规范、逻辑漏洞、性能问题、资源泄漏、泛型使用、扩展函数；
2. 每个问题必须标注等级，等级仅能是[%s/%s/%s/%s]，其中[%s]级问题直接阻断MR合并；
3. 输出格式：每行一个问题，格式为「[等级] 文件名:行号 - 问题描述 - 修复建议」；
4. 仅输出问题列表，无冗余前言/结语，无代码块，每行一条；
5. 若无问题，仅输出「✅ 未发现任何问题」。

待评审的MR变更代码-
---------------------
%s`, LevelBlock, LevelHigh, LevelMedium, LevelSuggest, LevelBlock, reviewContent)
}

func (p *PythonReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	logDebugln("\n=====================================")
	logDebugln("【RunPythonLint】开始执行")
	logDebug("  - 仓库路径：%s\n", repoPath)
	logDebug("  - 待检查文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	lintResults := make(map[string]string)

	// 检查是否安装了flake8
	if _, err := exec.LookPath("flake8"); err != nil {
		logDebugln("⚠️【RunPythonLint】未检测到flake8，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少flake8环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		logDebug("ℹ️【RunPythonLint】检查文件：%s\n", file)
		cmd := exec.Command("flake8", file)
		output, err := cmd.CombinedOutput()

		if err != nil {
			logDebug("⚠️【RunPythonLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			logDebug("✅【RunPythonLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			logDebug("⚠️【RunPythonLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

func (s *SwiftReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	logDebugln("\n=====================================")
	logDebugln("【RunSwiftLint】开始执行")
	logDebug("  - 仓库路径：%s\n", repoPath)
	logDebug("  - 待检查文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	lintResults := make(map[string]string)

	// 检查是否安装了swiftlint
	if _, err := exec.LookPath("swiftlint"); err != nil {
		logDebugln("⚠️【RunSwiftLint】未检测到swiftlint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少swiftlint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		logDebug("ℹ️【RunSwiftLint】检查文件：%s\n", file)
		cmd := exec.Command("swiftlint", "lint", file)
		output, err := cmd.CombinedOutput()

		if err != nil {
			logDebug("⚠️【RunSwiftLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			logDebug("✅【RunSwiftLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			logDebug("⚠️【RunSwiftLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

func (p *PythonReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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

		// 仅保留新增/修改的Python文件
		if (status == "added" || status == "modified") && strings.HasSuffix(filePath, ".py") {
			diffMap[filePath] = diffItem.Diff
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

func (s *SwiftReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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

		// 仅保留新增/修改的Swift文件
		if (status == "added" || status == "modified") && strings.HasSuffix(filePath, ".swift") {
			diffMap[filePath] = diffItem.Diff
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

func (j *JavaScriptReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	fmt.Println("\n=====================================")
	fmt.Println("【RunJavaScriptLint】开始执行")
	fmt.Printf("  - 仓库路径：%s\n", repoPath)
	fmt.Printf("  - 待检查文件数：%d\n", len(diffFiles))
	fmt.Println("=====================================")

	lintResults := make(map[string]string)

	// 检查是否安装了ESLint
	if _, err := exec.LookPath("eslint"); err != nil {
		fmt.Println("⚠️【RunJavaScriptLint】未检测到eslint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少eslint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		fmt.Printf("ℹ️【RunJavaScriptLint】检查文件：%s\n", file)
		cmd := exec.Command("eslint", file)
		output, err := cmd.CombinedOutput()

		if err != nil {
			fmt.Printf("⚠️【RunJavaScriptLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			fmt.Printf("✅【RunJavaScriptLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			fmt.Printf("⚠️【RunJavaScriptLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

func (k *KotlinReviewProcess) RunLint(repoPath string, diffFiles map[string]string) map[string]string {
	logDebugln("\n=====================================")
	logDebugln("【RunKotlinLint】开始执行")
	logDebug("  - 仓库路径：%s\n", repoPath)
	logDebug("  - 待检查文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	lintResults := make(map[string]string)

	// 检查是否安装了ktlint
	if _, err := exec.LookPath("ktlint"); err != nil {
		logDebugln("⚠️【RunKotlinLint】未检测到ktlint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少ktlint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		logDebug("ℹ️【RunKotlinLint】检查文件：%s\n", file)
		cmd := exec.Command("ktlint", file)
		output, err := cmd.CombinedOutput()

		if err != nil {
			logDebug("⚠️【RunKotlinLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			logDebug("✅【RunKotlinLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			logDebug("⚠️【RunKotlinLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

// GetReviewProcess 根据语言获取对应的评审流程实现
func GetReviewProcess(language string) ReviewProcess {
	switch strings.ToLower(language) {
	case "java":
		return &JavaReviewProcess{}
	case "python":
		return &PythonReviewProcess{}
	case "javascript", "js", "typescript", "ts", "tsx":
		return &JavaScriptReviewProcess{}
	case "swift":
		return &SwiftReviewProcess{}
	case "kotlin", "kt":
		return &KotlinReviewProcess{}
	case "golang", "go", "":
		fallthrough
	default:
		return &GolangReviewProcess{}
	}
}

func (j *JavaScriptReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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

		// 仅保留新增/修改的JavaScript/TypeScript文件
		if (status == "added" || status == "modified") && (strings.HasSuffix(filePath, ".js") || strings.HasSuffix(filePath, ".jsx") || strings.HasSuffix(filePath, ".ts") || strings.HasSuffix(filePath, ".tsx")) {
			diffMap[filePath] = diffItem.Diff
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

func (k *KotlinReviewProcess) FilterFiles(diffItems []DiffItem) map[string]string {
	diffMap := make(map[string]string)
	for _, diffItem := range diffItems {
		// 跳过二进制文件
		if diffItem.Binary {
			logDebug("ℹ️【GetMRDiff】跳过二进制文件：%s\n", diffItem.NewPath)
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

		// 仅保留新增/修改的Kotlin文件
		if (status == "added" || status == "modified") && strings.HasSuffix(filePath, ".kt") {
			diffMap[filePath] = diffItem.Diff
			logDebug("✅【GetMRDiff】检测到需评审文件：%s（状态：%s）\n", filePath, status)
		}
	}
	return diffMap
}

func maskSensitive(str string) string {
	if len(str) <= 6 {
		return "****"
	}
	return str[:6] + "****"
}

// 1. 拉取MR变更代码
func GetMRDiff(config Config, process ReviewProcess) (map[string]string, *CommitInfo, error) {
	logDebugln("=====================================")
	logDebugln("【GetMRDiff】开始执行，配置详情：")
	logDebug("  - YunxiaoToken: %s\n", maskSensitive(config.YunxiaoToken))
	logDebug("  - OrgID: %s\n", config.OrgID)
	logDebug("  - RepoID: %d\n", config.RepoID)
	logDebug("  - MRID: %d\n", config.MRID)
	logDebug("  - FromCommit: %s\n", config.FromCommit)
	logDebug("  - ToCommit: %s\n", config.ToCommit)
	logDebug("  - CodeupDomain: %s\n", config.CodeupDomain)
	logDebug("  - BaichuanAPIKey: %s\n", maskSensitive(config.BaichuanAPIKey))
	logDebug("  - ReviewLevel: %s\n", config.ReviewLevel)
	logDebug("  - CommentTarget: %s\n", config.CommentTarget)
	logDebug("  - CommitID: %s\n", config.CommitID)
	logDebugln("=======================================")

	logDebugln("🔍 开始拉取MR变更代码（云效OpenAPI）...")

	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Accept", "application/json").
		SetQueryParams(map[string]string{
			"from": config.FromCommit, // from为提交ID
			"to":   config.ToCommit,   // to为提交ID
		}).
		Get(fmt.Sprintf("https://%s/oapi/v1/codeup/organizations/%s/repositories/%d/compares",
			config.CodeupDomain, config.OrgID, config.RepoID))

	if err != nil {
		logDebug("❌【GetMRDiff】云效OpenAPI请求失败：%v\n", err)
		return nil, nil, fmt.Errorf("云效OpenAPI请求失败：%w", err)
	}
	if resp.StatusCode() != 200 {
		logDebug("❌【GetMRDiff】云效OpenAPI返回异常状态码：%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return nil, nil, fmt.Errorf("云效OpenAPI返回异常状态码：%d，响应内容：%s",
			resp.StatusCode(), string(resp.Body()))
	}

	var compareResp CompareResponseV2
	if err := json.Unmarshal(resp.Body(), &compareResp); err != nil {
		logDebug("❌【GetMRDiff】解析云效OpenAPI响应失败：%v，响应内容：%s\n", err, string(resp.Body()))
		return nil, nil, fmt.Errorf("解析云效OpenAPI响应失败：%w，响应内容：%s", err, string(resp.Body()))
	}

	logDebug("✅【GetMRDiff】成功拉取响应，共检测到%d个变更文件\n", len(compareResp.Diffs))

	// 提取commit信息
	var commitInfo *CommitInfo
	if len(compareResp.Commits) > 0 {
		commit := compareResp.Commits[0]
		commitInfo = &CommitInfo{
			AuthorName: commit.AuthorName,
			Message:    commit.Message,
		}
	}

	// 将CompareResponseV2中的Diffs转换为[]DiffItem类型
	var diffItems []DiffItem
	for _, diff := range compareResp.Diffs {
		diffItems = append(diffItems, DiffItem{
			Diff:        diff.Diff,
			NewPath:     diff.NewPath,
			OldPath:     diff.OldPath,
			NewFile:     diff.NewFile,
			DeletedFile: diff.DeletedFile,
			RenamedFile: diff.RenamedFile,
			Binary:      diff.IsBinary,
		})
	}

	diffMap := process.FilterFiles(diffItems)

	if len(diffMap) == 0 {
		logDebug("ℹ️【GetMRDiff】未检测到新增/修改的%s文件，无需评审\n", process.GetFileExtension())
		return diffMap, commitInfo, nil
	}
	logDebug("📌【GetMRDiff】共筛选出%d个需评审的%s文件\n", len(diffMap), process.GetFileExtension())
	return diffMap, commitInfo, nil
}

// 2. 执行golangci-lint规则检查
func RunGolangciLint(repoPath string, diffFiles map[string]string) map[string]string {
	logDebugln("\n=====================================")
	logDebugln("【RunGolangciLint】开始执行")
	logDebug("  - 仓库路径：%s\n", repoPath)
	logDebug("  - 待检查文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	lintResults := make(map[string]string)

	if _, err := exec.LookPath("golangci-lint"); err != nil {
		logDebugln("⚠️【RunGolangciLint】未检测到golangci-lint，跳过规则检查")
		for file := range diffFiles {
			lintResults[file] = "【规则检查】未执行：缺少golangci-lint环境"
		}
		return lintResults
	}

	for file := range diffFiles {
		logDebug("ℹ️【RunGolangciLint】检查文件：%s\n", file)
		cmd := exec.Command("bash", "-c",
			fmt.Sprintf("cd %s && golangci-lint run --new-from-rev=origin/main %s", repoPath, file))
		output, err := cmd.CombinedOutput()

		if err != nil {
			logDebug("⚠️【RunGolangciLint】文件%s检查失败：%v\n", file, err)
			lintResults[file] = fmt.Sprintf("【规则检查】执行失败：%s，输出：%s", err.Error(), string(output))
			continue
		}

		if string(output) == "" {
			logDebug("✅【RunGolangciLint】文件%s未发现违规问题\n", file)
			lintResults[file] = "【规则检查】未发现违规问题"
		} else {
			logDebug("⚠️【RunGolangciLint】文件%s发现违规问题：%s\n", file, string(output))
			lintResults[file] = fmt.Sprintf("【规则检查】发现问题：%s", string(output))
		}
	}

	return lintResults
}

// 3. 调用阿里云百炼API进行AI代码评审
func AICodeReview(config Config, diffFiles map[string]string, lintResults map[string]string, process ReviewProcess) (string, []string, []string, error) {
	logDebugln("\n=====================================")
	logDebugln("【AICodeReview】开始执行")
	logDebug("  - 待评审文件数：%d\n", len(diffFiles))
	logDebugln("=====================================")

	// 使用ReviewProcess接口获取prompt
	prompt := process.GetPrompt(diffFiles, lintResults)

	// 使用配置的模型名称，如果没有指定则使用默认值
	modelName := "qwen3-coder-plus"
	if config.Model != "" {
		modelName = config.Model
	}

	requestBody := map[string]interface{}{
		"model": modelName,
		"input": map[string]interface{}{
			"messages": []map[string]interface{}{
				{
					"role":    "user",
					"content": prompt,
				},
			},
		},
		"parameters": map[string]interface{}{
			"max_new_tokens": 9999,
			"temperature":    0.2,
			"top_p":          0.9,
		},
	}

	requestBodyJSON, err := json.MarshalIndent(requestBody, "", "  ")
	if err != nil {
		logDebug("❌【AICodeReview】构造请求体JSON失败：%v\n", err)
		return "", nil, nil, fmt.Errorf("构造请求体JSON失败：%w", err)
	}
	logDebug("ℹ️【AICodeReview】构造的请求体：\n%s\n", string(requestBodyJSON))

	logDebugln("ℹ️【AICodeReview】开始调用百炼原生API...")
	resp, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", config.BaichuanAPIKey)).
		SetBody(requestBody).
		//Post("https://coding.dashscope.aliyuncs.com/v1/chat/completions")//coding plan
		Post("https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation")

	if err != nil {
		logDebug("❌【AICodeReview】百炼API调用失败：%v\n", err)
		return "", nil, nil, fmt.Errorf("百炼API调用失败：%w", err)
	}

	logDebug("ℹ️【AICodeReview】百炼API响应状态码：%d\n", resp.StatusCode())
	logDebug("ℹ️【AICodeReview】百炼API响应内容：%s\n", string(resp.Body()))

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
		Code      string `json:"code"`
		Message   string `json:"message"`
	}
	if err := json.Unmarshal(resp.Body(), &aiResp); err != nil {
		logDebug("❌【AICodeReview】解析百炼API响应失败：%v，响应内容：%s\n", err, string(resp.Body()))
		return "", nil, nil, fmt.Errorf("解析百炼API响应失败：%w，响应内容：%s", err, string(resp.Body()))
	}

	if aiResp.Code != "" {
		logDebug("❌【AICodeReview】百炼API返回业务错误：code=%s, message=%s\n", aiResp.Code, aiResp.Message)
		return "", nil, nil, fmt.Errorf("百炼API业务错误：%s - %s", aiResp.Code, aiResp.Message)
	}

	var aiResult string
	if len(aiResp.Output.Choices) > 0 {
		aiResult = strings.TrimSpace(aiResp.Output.Choices[0].Message.Content)
	}
	logDebug("✅【AICodeReview】百炼API调用成功，RequestID：%s\n", aiResp.RequestID)
	logDebug("ℹ️【AICodeReview】Token使用情况：Total=%d, Input=%d, Output=%d\n",
		aiResp.Usage.TotalTokens, aiResp.Usage.InputTokens, aiResp.Usage.OutputTokens)
	logDebug("ℹ️【AICodeReview】AI评审结果：%s\n", aiResult)

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
				logDebug("❌【AICodeReview】检测到阻断级问题：%s\n", line)
			} else if strings.Contains(line, fmt.Sprintf("[%s]", LevelHigh)) {
				highIssues = append(highIssues, line)
				logDebug("⚠️【AICodeReview】检测到高级别问题：%s\n", line)
			}
		}
	}

	logDebug("📊【AICodeReview】AI评审完成，检测到%d个阻断级问题，%d个高级别问题.\n", len(blockIssues), len(highIssues))
	return aiResult, blockIssues, highIssues, nil
}

// 4. 将评审结果评论到Codeup MR
func CommentMR(config Config, reviewResult string) error {
	logDebugln("\n=====================================")
	logDebugln("【CommentMR】开始执行")
	logDebug("  - MRID：%d\n", config.MRID)
	logDebugln("=====================================")

	// 根据语言类型获取对应的文件扩展名描述
	var langDesc string
	switch config.Language {
	case "java":
		langDesc = "Java"
	case "python":
		langDesc = "Python"
	case "javascript", "js", "typescript", "ts", "tsx":
		langDesc = "JavaScript/TypeScript"
	case "swift":
		langDesc = "Swift"
	case "kotlin", "kt":
		langDesc = "Kotlin"
	case "golang", "go", "":
		fallthrough
	default:
		langDesc = "Go"
	}

	commentBody := fmt.Sprintf(`
### 🤖 AI Code Review 结果（MR #%d）
#### 评审范围：提交ID %s → %s 变更的%s文件
#### 问题等级说明：
- [%s]：阻断级，必须修复才能合并
- [%s]：高风险，建议优先修复
- [%s]：中风险，建议修复
- [%s]：优化建议，不强制

---
%s`, config.MRID, config.FromCommit, config.ToCommit, langDesc,
		LevelBlock, LevelHigh, LevelMedium, LevelSuggest, reviewResult)

	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"content": commentBody,
			// 可选参数（如需回复特定评论，可添加parentId）
			// "parentId": 0,
		}).
		Post(fmt.Sprintf("https://%s/oapi/v1/codeup/change_requests/%d/comments",
			config.CodeupDomain, config.MRID))

	if err != nil {
		logDebug("❌【CommentMR】创建MR评论API调用失败：%v\n", err)
		return fmt.Errorf("创建MR评论API调用失败：%w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		logDebug("❌【CommentMR】创建MR评论失败：状态码%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return fmt.Errorf("创建MR评论失败：状态码%d，响应内容：%s", resp.StatusCode(), string(resp.Body()))
	}

	var commentResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &commentResp); err != nil {
		logDebug("⚠️【CommentMR】解析MR评论响应失败（但评论已提交）：%s\n", err)
	} else {
		logDebug("✅【CommentMR】评审结果评论成功，评论ID：%v\n", commentResp["id"])
	}

	return nil
}

// 5. 将评审结果评论到Codeup Commit
func CommentCommit(config Config, reviewResult string) error {
	logDebugln("\n=====================================")
	logDebugln("【CommentCommit】开始执行")
	logDebug("  - OrgID：%s\n", config.OrgID)
	logDebug("  - RepoID：%d\n", config.RepoID)
	logDebug("  - CommitID：%s\n", config.CommitID)
	logDebug("  - reviewResult：%s\n", reviewResult)
	logDebugln("=====================================")

	if reviewResult == "" {
		logDebugln("ℹ️【CommentCommit】AI评审结果为空，跳过评论提交")
		return nil
	}
	// 根据语言类型获取对应的文件扩展名描述
	var langDesc string
	switch config.Language {
	case "java":
		langDesc = "Java"
	case "python":
		langDesc = "Python"
	case "javascript", "js", "typescript", "ts", "tsx":
		langDesc = "JavaScript/TypeScript"
	case "swift":
		langDesc = "Swift"
	case "kotlin", "kt":
		langDesc = "Kotlin"
	case "golang", "go", "":
		fallthrough
	default:
		langDesc = "Go"
	}

	commentBody := fmt.Sprintf(`
### 🤖 AI Code Review 结果（Commit %s）
#### 评审范围：提交ID %s → %s 变更的%s文件
#### 问题等级说明：
- [%s]：阻断级，必须修复
- [%s]：高风险，建议优先修复
- [%s]：中风险，建议修复
- [%s]：优化建议，不强制

---
%s`, config.CommitID, config.FromCommit, config.ToCommit, langDesc,
		LevelBlock, LevelHigh, LevelMedium, LevelSuggest, reviewResult)

	resp, err := client.R().
		SetHeader("x-yunxiao-token", config.YunxiaoToken).
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]interface{}{
			"content": commentBody,
		}).
		// 官方指定的API路径：organizations/{orgId}/repositories/{repoId}/commits/{commitId}/comments
		Post(fmt.Sprintf("https://%s/oapi/v1/codeup/organizations/%s/repositories/%d/commits/%s/comments",
			config.CodeupDomain, config.OrgID, config.RepoID, config.CommitID))

	if err != nil {
		logDebug("❌【CommentCommit】创建Commit评论API调用失败：%v\n", err)
		return fmt.Errorf("创建Commit评论API调用失败：%w", err)
	}

	if resp.StatusCode() != 200 && resp.StatusCode() != 201 {
		if resp.StatusCode() == 403 {
			logDebug("❌【CommentCommit】创建Commit评论失败：Token权限不足！\n")
			logDebug("   解决方案：\n")
			logDebug("   1. 登录云效控制台 → 个人设置 → 访问令牌，检查Token权限\n")
			logDebug("   2. 确保Token包含Codeup仓库的写权限和Commit评论权限\n")
			logDebug("   3. 确认你的账号对目标仓库有开发者及以上权限\n")
		}
		logDebug("❌【CommentCommit】创建Commit评论失败：状态码%d，响应内容：%s\n", resp.StatusCode(), string(resp.Body()))
		return fmt.Errorf("创建Commit评论失败：状态码%d，响应内容：%s", resp.StatusCode(), string(resp.Body()))
	}

	logDebug("✅【CommentCommit】Commit评论提交成功（状态码：%d）\n", resp.StatusCode())
	respBody := string(resp.Body())
	if respBody == "" {
		logDebugln("ℹ️【CommentCommit】云效返回空响应体，跳过JSON解析（评论已提交）")
		return nil
	}

	var commentResp map[string]interface{}
	if err := json.Unmarshal(resp.Body(), &commentResp); err != nil {
		logDebug("ℹ️【CommentCommit】解析响应失败（但评论已提交）：%s，响应体：%s\n", err, respBody)
		return nil // 解析失败不返回错误，因为核心功能（评论提交）已完成
	}

	logDebug("✅【CommentCommit】评审结果评论成功，评论ID：%v\n", commentResp["id"])
	return nil
}

// 帮助信息
func printUsage() {
	usage := `
🚀 airvw - AI驱动的阿里云效平台Codeup代码评审工具
=====================***=======================
功能：自动拉取Codeup MR/Commit的代码变更，执行静态检查，调用阿里云百炼AI评审，
      支持将评审结果评论到MR/Commit，阻断级问题直接终止流程。
      支持多种编程语言：Golang/Java/Python/JavaScript/Swift/Kotlin

📦 安装方式：
  go install github.com/konglong87/airvw@latest

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
    --language string         评审语言（默认：golang，可选：golang/java/python/javascript/swift/kotlin）
    --model string            AI模型名称（默认：qwen3-coder-plus）
    --dingtalk-token string   钉钉机器人Token（可选）
    --dingtalk-secret string   钉钉机器人Secret（可选）
    --enable-dingtalk         是否启用钉钉通知（默认：false）
    --max-issues int          钉钉通知中显示的最大问题数量（默认：10）
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

  4. 评审Java代码：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --language java

  5. 评审Python代码：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --language python

  6. 评审Swift代码：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --language swift

  7. 评审Kotlin代码：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --language kotlin

  8. 启用钉钉通知：
     airvw --yunxiao-token pt-xxx --org-id 67aaaaaaaaaa --repo-id 5023797 \
           --from-commit xxxxxx --to-commit xxxxxx --baichuan-key sk-xxx \
           --enable-dingtalk --dingtalk-token xxx --dingtalk-secret xxx

⚠️ 注意事项：
  1. Golang需提前安装golangci-lint（可选，未安装则跳过规则检查）
  2. Java需提前安装checkstyle（可选，未安装则跳过规则检查）
  3. Python需提前安装flake8（可选，未安装则跳过规则检查）
  4. JavaScript需提前安装eslint（可选，未安装则跳过规则检查）
  5. Swift需提前安装swiftlint（可选，未安装则跳过规则检查）
  6. Kotlin需提前安装ktlint（可选，未安装则跳过规则检查）
  7. 百炼API Key需具备文本生成权限
  8. 云效Token需具备Codeup MR/Commit评论权限
  9. 仅评审新增/修改的对应语言文件，二进制文件、删除/重命名文件会被过滤
`
	fmt.Println(usage)
}

func main() {
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
	flag.StringVar(&config.Language, "language", "golang", "评审语言：golang/java/python/javascript/swift/kotlin（默认golang）")
	flag.StringVar(&config.Model, "model", "qwen3-coder-plus", "AI模型名称（默认qwen3-coder-plus）")
	flag.BoolVar(&config.Debug, "debug", false, "是否开启调试模式，默认false")
	flag.StringVar(&config.DingTalkToken, "dingtalk-token", "", "钉钉机器人Token（可选）")
	flag.StringVar(&config.DingTalkSecret, "dingtalk-secret", "", "钉钉机器人Secret（可选）")
	flag.BoolVar(&config.EnableDingTalk, "enable-dingtalk", false, "是否启用钉钉通知，默认false")
	flag.IntVar(&config.MaxIssues, "max-issues", 10, "钉钉通知中显示的最大问题数量，默认10")
	flag.Parse()

	debugMode = config.Debug

	if len(os.Args) == 2 && (os.Args[1] == "--help" || os.Args[1] == "-h") {
		printUsage()
		os.Exit(0)
	}

	logDebugln("\n=====================================")
	logDebugln("【aiutoCR】命令行参数解析完成")
	logDebugln("=====================================")

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

	if config.CommentTarget == "mr" && config.MRID == 0 {
		missingParams = append(missingParams, "mr-id（评论MR时必填）")
	}
	if config.CommentTarget == "commit" && config.CommitID == "" {
		missingParams = append(missingParams, "commit-id（评论Commit时必填）")
	}

	if len(missingParams) > 0 {
		fmt.Printf("❌【aiutoCR】错误：缺少必填参数：%s\n", strings.Join(missingParams, ", "))
		printUsage()
		os.Exit(1)
	}

	reviewProcess := GetReviewProcess(config.Language)
	logDebug("ℹ️【aiutoCR】使用%s语言评审流程\n", config.Language)

	diffFiles, commitInfo, err := GetMRDiff(config, reviewProcess)
	if err != nil {
		fmt.Printf("❌【aiutoCR】拉取MR变更失败：%s\n", err)
		os.Exit(1)
	}
	if len(diffFiles) == 0 {
		fmt.Printf("✅【aiutoCR】无变更的%s文件，评审通过\n", reviewProcess.GetFileExtension())
		os.Exit(0)
	}

	lintResults := reviewProcess.RunLint(".", diffFiles)

	aiResult, blockIssues, highIssues, err := AICodeReview(config, diffFiles, lintResults, reviewProcess)
	if err != nil {
		fmt.Printf("❌【aiutoCR】AI评审失败：%s\n", err)
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
		logDebugln("ℹ️【aiutoCR】未指定有效评论目标（mr/commit），跳过评论操作")
	}
	if commentErr != nil {
		logDebug("⚠️【aiutoCR】评论%s失败（不终止评审）：%s\n", config.CommentTarget, commentErr)
	}

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
		logDebug("\n❌【aiutoCR】检测到%d个%s问题，终止流程！\n", len(blockList), blockReason)
		formattedIssues := formatBlockIssues(blockList)
		result := ReviewResult{
			Status:      "blocked",
			TotalIssues: len(blockList),
			BlockReason: blockReason,
			BlockIssues: formattedIssues,
			Message:     fmt.Sprintf("检测到%d个%s问题，终止流程", len(blockList), blockReason),
			CommitInfo:  commitInfo,
			Model:       config.Model,
		}
		fmt.Println("\n======= ********** [代码问题详情] ********** =======")
		printJSONResult(result)

		// 发送钉钉通知
		if config.EnableDingTalk {
			jsonData, _ := json.MarshalIndent(result, "", "  ")
			DingDingRemind(config.DingTalkToken, config.DingTalkSecret, string(jsonData), config.MaxIssues)
		}

		os.Exit(1)
	}
	// 即使评审通过（不阻塞），用户也能看到AI评审提供的所有建议结果，而不仅仅是看到"评审通过"的提示
	// 解析AI评审结果中的所有问题（包括建议级）
	var allIssues []string
	if aiResult != "✅ 未发现任何问题" {
		lines := strings.Split(aiResult, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// 提取所有等级的问题
			if strings.Contains(line, fmt.Sprintf("[%s]", LevelBlock)) ||
				strings.Contains(line, fmt.Sprintf("[%s]", LevelHigh)) ||
				strings.Contains(line, fmt.Sprintf("[%s]", LevelMedium)) ||
				strings.Contains(line, fmt.Sprintf("[%s]", LevelSuggest)) {
				allIssues = append(allIssues, line)
			}
		}
	}

	// 显示任何问题（包括建议级）
	if len(allIssues) > 0 {
		formattedIssues := formatBlockIssues(allIssues)
		result := ReviewResult{
			Status:      "success",
			TotalIssues: len(allIssues),
			BlockIssues: formattedIssues,
			Message:     fmt.Sprintf("评审通过，发现%d个非阻塞问题", len(allIssues)),
			CommitInfo:  commitInfo,
			Model:       config.Model,
		}
		fmt.Println("\n======= ********** [AI评审建议详情] ********** =======")
		printJSONResult(result)

		// 发送钉钉通知
		if config.EnableDingTalk {
			jsonData, _ := json.MarshalIndent(result, "", "  ")
			DingDingRemind(config.DingTalkToken, config.DingTalkSecret, string(jsonData), config.MaxIssues)
		}
	}

	fmt.Printf("\n✅【aiutoCR】所有评审完成，无阻断级问题，评审通过 ✅）\n")
	os.Exit(0)
}
