package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func main() {
	arg1 := "bash"
	arg2 := "-c"

	cmdArg := os.Args

	if len(cmdArg) > 1 {
		if cmdArg[1] == "config" {
			if len(cmdArg) < 3 {
				fmt.Println("usage: promptshell config <api-key>")
				return
			}
			if err := updateApiKey(cmdArg[2]); err != nil {
				fmt.Println("error saving api key:", err)
				return
			}
			fmt.Println("api key saved")
			return
		}

		prompt := cmdArg[1]
		fmt.Println("initializing...")

		apiKey := resolveApiKey()
		if apiKey == "" {
			fmt.Println("no api key found. set one with: promptshell config <api-key> (or the PROMPTSHELL_API_KEY environment variable)")
			return
		}

		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			fmt.Println("fatal error occurred", err)
			return
		}

		defer client.Close()

		model := client.GenerativeModel("gemini-pro")
		fmt.Println("generating response to prompt...")
		resp, err := model.GenerateContent(ctx, genai.Text("generate a shell script for this task: "+prompt))
		if err != nil {
			fmt.Println("fatal error occurred", err)
			return
		}
		candidate := resp.Candidates[0]
		text := fmt.Sprintf("%vn", candidate.Content.Parts[0])
		text = text[:len(text)-1]
		text = strings.Replace(text, "```sh\n", "", 1)
		text = strings.TrimSuffix(string(text), "```\n")
		writeToFile("prompt.sh", text)
		permissionCommand := exec.Command(arg1, arg2, "chmod a+x prompt.sh")
		_, err1 := permissionCommand.Output()
		if err1 == nil {
			fmt.Println("result from running permission command is")
			app := "./prompt.sh"
			cmd := exec.Command(arg1, arg2, app)
			stdout, err := cmd.Output()

			if err != nil {
				fmt.Println(err.Error())
				return
			} else {
				fmt.Print("executed prompt.sh", stdout)
			}
		} else {
			fmt.Println("error occurred while granting permission to prompt.sh", err1)
		}
	}
}

// resolveApiKey returns the Gemini API key, preferring the PROMPTSHELL_API_KEY
// environment variable and falling back to the saved config file.
func resolveApiKey() string {
	if key := os.Getenv("PROMPTSHELL_API_KEY"); key != "" {
		return key
	}
	key, err := getApiKey()
	if err != nil {
		return ""
	}
	return key
}
