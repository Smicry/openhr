package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: clean-blank <file>")
		os.Exit(1)
	}

	file := os.Args[1]
	content, err := ioutil.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
		os.Exit(1)
	}

	result := removeBlankLinesInFuncs(string(content))

	err = ioutil.WriteFile(file, []byte(result), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing file: %v\n", err)
		os.Exit(1)
	}
}

func removeBlankLinesInFuncs(content string) string {
	lines := strings.Split(content, "\n")
	var result []string

	inFunc := false
	funcBraceLine := -1

	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)

		// 函数开始
		if strings.HasPrefix(trimmed, "func ") && strings.Contains(trimmed, "{") {
			inFunc = true
			funcBraceLine = i
			result = append(result, line)
			continue
		}

		// 函数结束
		if inFunc && trimmed == "}" && i > funcBraceLine {
			// 删除 } 前的空行
			if len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
				result = result[:len(result)-1]
			}
			inFunc = false
			funcBraceLine = -1
			result = append(result, line)
			continue
		}

		// 函数内，{ 后的空行
		if inFunc && funcBraceLine >= 0 && i == funcBraceLine+1 && trimmed == "" {
			continue
		}

		// 函数内，空行后紧跟 }
		if inFunc && trimmed == "" && i+1 < len(lines) {
			nextTrimmed := strings.TrimSpace(lines[i+1])
			if nextTrimmed == "}" {
				continue
			}
		}

		result = append(result, line)
	}

	return strings.Join(result, "\n")
}
