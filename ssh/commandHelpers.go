package ssh

import (
	"fmt"
	"sort"
	"strings"
)

func SetEnvs(envs map[string]any) (result string) {
	var keys []string
	for k := range envs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for i, k := range keys {
		if i > 0 {
			result += " "
		}

		result += fmt.Sprintf("%s=%s", k, shellQuote(fmt.Sprint(envs[k])))
	}

	return
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func LoadAndRunScript(scriptURL, accessToken string, envs map[string]any) (fullCommandlet string) {
	envPrefix := SetEnvs(envs)
	if envPrefix != "" {
		envPrefix += " "
	}

	quotedCookie := shellQuote(fmt.Sprintf("Cookie: Authorization=%s", accessToken))
	quotedURL := shellQuote(scriptURL)

	download := fmt.Sprintf("tmp_script=$(mktemp) && trap 'rm -f \"$tmp_script\"' EXIT && if command -v curl >/dev/null 2>&1; then curl -fsSLk --header %s -o \"$tmp_script\" %s; elif command -v wget >/dev/null 2>&1; then wget --no-check-certificate --header=%s -qO \"$tmp_script\" %s; else echo \"curl or wget required\" >&2; exit 1; fi && test -s \"$tmp_script\"", quotedCookie, quotedURL, quotedCookie, quotedURL)

	fullCommandlet = fmt.Sprintf("%s && %sbash \"$tmp_script\"", download, envPrefix)
	return fullCommandlet
}
