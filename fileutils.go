package main

import (
	"io/ioutil"
)

func writeToFile(filename string, content string) error {
	return ioutil.WriteFile(filename, []byte(content), 0666)
}
