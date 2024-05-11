package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

func main() {
	arg1 := "bash"
	arg2 := "-c"

	fmt.Println("initializing...")
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey("REDACTED-API-KEY"))
	if err != nil {
		fmt.Println("fatal error occurred", err)
	}

	defer client.Close()

	model := client.GenerativeModel("gemini-pro")
	fmt.Println("generating response to prompt...")
	resp, err := model.GenerateContent(ctx, genai.Text("generate a shell script for this task: create an android project called blank"))
	if err != nil {
		fmt.Println("fatal error occurred", err)
	}
	candidate := resp.Candidates[0]
	text := fmt.Sprintf("%vn", candidate.Content.Parts[0])
	text = text[:len(text)-1]
	text = strings.Replace(text, "```sh\n", "", 1)
	text = strings.Replace(text, "```", "", 1)
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
