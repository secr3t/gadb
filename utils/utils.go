package utils

import (
	"regexp"
	"strings"
)

const (
	MainAction       = "android.intent.action.MAIN"
	LauncherCategory = "android.intent.category.LAUNCHER"
)

// escapeRegExp 문자열에서 정규식을 탈출하는 함수
func escapeRegExp(str string) string {
	re := regexp.MustCompile(`[.*+?^${}()|\[\]\\]`)
	return re.ReplaceAllString(str, `\$0`)
}

// matchComponentName 컴포넌트 이름이 유효한지 확인하는 함수
func matchComponentName(name string) bool {
	// 이 함수는 완전한 액티비티 이름인지 확인하는 로직을 구현
	return strings.Contains(name, ".")
}

// ParseLaunchableActivityNames dumpsys 출력에서 launchable activity를 파싱하는 함수
func ParseLaunchableActivityNames(dumpsys string) []string {
	mainActivityNameRe := regexp.MustCompile(`^\s*` + escapeRegExp(MainAction) + `:$`)
	categoryNameRe := regexp.MustCompile(`^\s*Category:\s+"([a-zA-Z0-9._/-]+)"$`)

	var blocks [][]string
	var blockStartIndent *int
	var block []string

	for _, line := range strings.Split(dumpsys, "\n") {
		line = strings.TrimRight(line, " ")
		currentIndent := len(line) - len(strings.TrimLeft(line, " "))

		if mainActivityNameRe.MatchString(line) {
			blockStartIndent = &currentIndent
			if len(block) > 0 {
				blocks = append(blocks, block)
				block = []string{}
			}
			continue
		}

		if blockStartIndent == nil {
			continue
		}

		if currentIndent > *blockStartIndent {
			block = append(block, line)
		} else {
			if len(block) > 0 {
				blocks = append(blocks, block)
				block = []string{}
			}
			blockStartIndent = nil
		}
	}

	if len(block) > 0 {
		blocks = append(blocks, block)
	}

	var result []string
	for _, item := range blocks {
		hasCategory := false
		isLauncherCategory := false

		for _, line := range item {
			match := categoryNameRe.FindStringSubmatch(line)
			if match == nil {
				continue
			}

			hasCategory = true
			isLauncherCategory = match[1] == LauncherCategory
			break
		}

		// 오래된 Android 버전에서는 카테고리 이름이 없을 수도 있으므로 일단 다 추가
		if hasCategory && !isLauncherCategory {
			continue
		}

		for _, activityNameStr := range item {
			activityNameStr = strings.TrimSpace(activityNameStr)
			if activityNameStr == "" {
				continue
			}

			parts := strings.Fields(activityNameStr)
			if len(parts) < 2 {
				continue
			}

			fqActivityName := parts[1]
			if !matchComponentName(fqActivityName) {
				continue
			}

			if isLauncherCategory {
				return []string{fqActivityName}
			}
			result = append(result, fqActivityName)
		}
	}
	return result
}
