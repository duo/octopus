package common

import (
	"strconv"
	"strings"
)

func Itoa(i int64) string {
	return strconv.FormatInt(i, 10)
}

func Atoi(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

func EscapeText(parseMode string, text string) string {
	var replacer *strings.Replacer

	if parseMode == "Markdown" {
		replacer = strings.NewReplacer("_", "\\_", "*", "\\*", "`", "\\`", "[", "\\[")
	} else if parseMode == "MarkdownV2" {
		replacer = strings.NewReplacer(
			"_", "\\_", "*", "\\*", "[", "\\[", "]", "\\]", "(",
			"\\(", ")", "\\)", "~", "\\~", "`", "\\`", ">", "\\>",
			"#", "\\#", "+", "\\+", "-", "\\-", "=", "\\=", "|",
			"\\|", "{", "\\{", "}", "\\}", ".", "\\.", "!", "\\!",
		)
	} else {
		return ""
	}

	return replacer.Replace(text)
}
