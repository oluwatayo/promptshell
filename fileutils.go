package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
)

func writeToFile(filename string, content string) error {
	return ioutil.WriteFile(filename, []byte(content), 0666)
}

func createOrGetConfig() (Config, error) {
	homeDir, _ := os.UserHomeDir()
	var config Config
	var err error
	data, readErr := ioutil.ReadFile(homeDir + "/.promptshell/config/config.json")
	if readErr != nil {
		mkErr := os.MkdirAll(homeDir+"/.promptshell/config/", os.ModePerm)
		if mkErr != nil {
			err = mkErr
		}
	} else {
		parseError := json.Unmarshal(data, &config)
		if parseError != nil {
			err = parseError
		}
	}
	return config, err
}

func updateApiKey(newApiKey string) error {
	homeDir, _ := os.UserHomeDir()
	config, err := createOrGetConfig()
	if err == nil {
		config.ApiKey = newApiKey
		data, mashErr := json.Marshal(config)
		if mashErr != nil {
			return mashErr
		} else {
			return writeToFile(homeDir+"/.promptshell/config/config.json", string(data))
		}
	} else {
		return err
	}
}

func getApiKey() (string, error) {
	config, err := createOrGetConfig()
	if err != nil {
		return "", err
	} else {
		return config.ApiKey, nil
	}
}
