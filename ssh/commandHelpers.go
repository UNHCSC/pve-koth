package ssh

import "fmt"

func SetEnvs(envs map[string]any) (result string) {
	for k, v := range envs {
		result += fmt.Sprintf("%s=\"%v\" ", k, v)
	}

	if result != "" {
		result = result[:len(result)-1]
	}

	return
}

func LoadAndRunScript(scriptURL, accessToken string, envs map[string]any) (fullCommandlet string) {
	envPrefix := SetEnvs(envs)
	if envPrefix != "" {
		envPrefix += " "
	}

	download := fmt.Sprintf("if command -v curl >/dev/null 2>&1; then curl -fsSLk --header \"Cookie: Authorization=%s\" \"%s\"; elif command -v wget >/dev/null 2>&1; then wget --no-check-certificate --header='Cookie: Authorization=%s' -qO- '%s'; else echo \"curl or wget required\" >&2; exit 1; fi", accessToken, scriptURL, accessToken, scriptURL)

	fullCommandlet = fmt.Sprintf("%s| %sbash -s --", download, envPrefix)
	return fullCommandlet
}
