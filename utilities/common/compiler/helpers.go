package compiler

import (
	"bytes"
	"io/ioutil"
	"regexp"
)

var versionRegexp = regexp.MustCompile(`([0-9]+)\.([0-9]+)\.([0-9]+)`)

type Contract struct {
	Code        string            `json:"code"`
	RuntimeCode string            `json:"runtime-code"`
	Info        ContractInfo      `json:"info"`
	Hashes      map[string]string `json:"hashes"`
}

type ContractInfo struct {
	Source          string      `json:"source"`
	Language        string      `json:"language"`
	LanguageVersion string      `json:"languageVersion"`
	CompilerVersion string      `json:"compilerVersion"`
	CompilerOptions string      `json:"compilerOptions"`
	SrcMap          interface{} `json:"srcMap"`
	SrcMapRuntime   string      `json:"srcMapRuntime"`
	AbiDefinition   interface{} `json:"abiDefinition"`
	UserDoc         interface{} `json:"userDoc"`
	DeveloperDoc    interface{} `json:"developerDoc"`
	Metadata        string      `json:"metadata"`
}

func slurpFiles(files []string) (string, error) {
	var concat bytes.Buffer
	for _, file := range files {
		content, err := ioutil.ReadFile(file)
		if err != nil {
			return "", err
		}
		concat.Write(content)
	}
	return concat.String(), nil
}
