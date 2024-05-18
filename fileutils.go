package main

import (
	"encoding/json"
	"fmt"
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
		fmt.Println("got read err", readErr)
		mkErr := os.MkdirAll(homeDir+"/.promptshell/config/", os.ModePerm)
		if mkErr != nil {
			fmt.Println("unable to create config directory", mkErr)
			err = mkErr
		}
	} else {
		fmt.Println("was able to read")
		parseError := json.Unmarshal(data, &config)
		if parseError != nil {
			fmt.Println("error occurred while parsing json", parseError)
			err = parseError
		}
	}
	return config, err
}

func updateApiKey(newApiKey string) error {
	homeDir, _ := os.UserHomeDir()
	config, err := createOrGetConfig()
	if err == nil {
		fmt.Println("found a config file")
		config.ApiKey = newApiKey
		data, mashErr := json.Marshal(config)
		if mashErr != nil {
			fmt.Println("error occurred while updating api key")
		} else {
			fmt.Println("json", string(data))
		}
		return writeToFile(homeDir+"/.promptshell/config/config.json", string(data))
	} else {
		fmt.Println("error getting a config file", err)
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
