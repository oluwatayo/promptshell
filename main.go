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
	fmt.Println("initializing...")
	ctx := context.Background()
	client, err := genai.NewClient(ctx, option.WithAPIKey("AIzaSyCSakG5HG00_jzta6xJATWlHuEZi9QiXXE"))
	if err != nil {
		fmt.Println("fatal error occurred", err)
	}

	defer client.Close()

	model := client.GenerativeModel("gemini-pro")
	fmt.Println("generating response to prompt...")
	resp, err := model.GenerateContent(ctx, genai.Text("generate a shell script for this task: create a folder called work and create two html files inside the folder"))
	if err != nil {
		fmt.Println("fatal error occurred", err)
	}
	candidate := resp.Candidates[0]
	text := fmt.Sprintf("%vn", candidate.Content.Parts[0])
	text = text[:len(text)-1]
	text = strings.Replace(text, "```sh\n", "", 1)
	text = strings.Replace(text, "```", "", 1)
	writeToFile("exec.sh", text)
	permissionCommand := "chmod a+x exec.sh"
	exec.Command(permissionCommand)
	app := "./exec.sh"
	cmd := exec.Command(app)
	stdout, err := cmd.Output()

	if err != nil {
		fmt.Println(err.Error())
		return
	} else {
		fmt.Print("executed", stdout)
	}
}
