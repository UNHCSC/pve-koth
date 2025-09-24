package sshcomm

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
	fullCommandlet = fmt.Sprintf("wget --header='Cookie: Authorization=%s' -qO- '%s' | %s bash -s --", accessToken, scriptURL, SetEnvs(envs))
	return
}
