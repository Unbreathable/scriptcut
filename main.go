package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/joho/godotenv"
	"google.golang.org/genai"
)

var client *genai.Client

const cutsDir = "cuts"

func ensureCutsDir() error {
	info, err := os.Stat(cutsDir)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s exists and is not a directory", cutsDir)
		}

		var deleteExisting bool
		confirmForm := huh.NewForm(
			huh.NewGroup(
				huh.NewConfirm().
					Title("Cuts folder already exists. Delete it?").
					Value(&deleteExisting),
			),
		)
		if err := confirmForm.Run(); err != nil {
			return err
		}
		if deleteExisting {
			if err := os.RemoveAll(cutsDir); err != nil {
				return err
			}
			return os.MkdirAll(cutsDir, 0755)
		}

		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}

	return os.MkdirAll(cutsDir, 0755)
}

func main() {
	inputFlag := flag.String("i", "", "Input video file (required)")
	scriptFlag := flag.String("s", "", "Script file to use for cutting (required without -c)")
	promptFlag := flag.String("p", "", "Prompt to pass to ScriptCut")
	cutsFlag := flag.String("c", "", "Cuts to apply in LLM format: 00:00:01-00:00:05,00:00:10-00:00:20")
	flag.Parse()

	// Collect any leftover args as an inline prompt if -p was not used
	prompt := *promptFlag
	if prompt == "" && flag.NArg() > 0 {
		prompt = strings.Join(flag.Args(), " ")
	}

	// Validate required flags
	if *inputFlag == "" {
		fmt.Fprintln(os.Stderr, "error: input file is required (-i)")
		flag.Usage()
		os.Exit(1)
	}

	videoFile := *inputFlag

	// If -c is provided, skip the LLM entirely and just apply the cuts
	if *cutsFlag != "" {
		if err := ensureCutsDir(); err != nil {
			log.Fatal(err)
		}
		cuts := parseStamps(*cutsFlag)
		applyCuts(cuts, videoFile)
		return
	}

	if *scriptFlag == "" {
		fmt.Fprintln(os.Stderr, "error: script file is required (-s) unless cuts are provided directly (-c)")
		flag.Usage()
		os.Exit(1)
	}

	// Read script file
	scriptBytes, err := os.ReadFile(*scriptFlag)
	if err != nil {
		log.Fatalf("failed to read script file: %v", err)
	}
	script := strings.TrimSpace(string(scriptBytes))

	// Build the user message content that will be sent to Gemini
	userMessage := fmt.Sprintf("<SCRIPT>%s</SCRIPT>\n<PROMPT>%s</PROMPT>", script, prompt)

	godotenv.Load()

	// Create a new client for the gemini api
	ctx := context.Background()
	client, err = genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  os.Getenv("GEMINI_KEY"),
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		log.Fatal(err)
	}

	// List available models and let user select one
	modelList, err := client.Models.List(ctx, nil)
	if err != nil {
		log.Fatal(err)
	}

	var modelOptions []huh.Option[string]
	for _, m := range modelList.Items {
		modelOptions = append(modelOptions, huh.NewOption(m.Name, m.Name))
	}

	var selectedModel string
	modelForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a Gemini model").
				Options(modelOptions...).
				Value(&selectedModel),
		),
	)
	if err := modelForm.Run(); err != nil {
		log.Fatal(err)
	}

	if err := ensureCutsDir(); err != nil {
		log.Fatal(err)
	}

	audioFile := filepath.Join(cutsDir, "audio.mp3")
	cmd := exec.Command("ffmpeg", "-i", videoFile, "-q:a", "0", "-map", "a", audioFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Fatalf("ffmpeg conversion failed: %v", err)
	}
	fmt.Printf("Audio written to %s\n", audioFile)
	defer os.Remove(audioFile)

	file, err := client.Files.UploadFromPath(
		ctx,
		audioFile,
		&genai.UploadFileConfig{
			MIMEType: "audio/mp3",
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Files.Delete(ctx, file.Name, nil)

	// Poll until the audio file is completely processed (state becomes ACTIVE).
	for file.State == genai.FileStateUnspecified || file.State != genai.FileStateActive {
		fmt.Println("Processing audio...")
		fmt.Println("File state:", file.State)
		time.Sleep(5 * time.Second)

		file, err = client.Files.Get(ctx, file.Name, nil)
		if err != nil {
			log.Fatal(err)
		}
	}

	// The system prompt
	systemPrompt := `
	Your name is ScriptCut, you are an AI helping to cut videos using the user's script. The user may talk to you (in the prompt or in the provided audio) by referring to you as Scriptcut.

	The script of the user will be provided to you in <SCRIPT></SCRIPT> tags. Additionally, a prompt MAY be provided in <PROMPT></PROMPT> tags. This way you should be able to clearly tell them apart. The audio will be attached as a file.

	Your job is to cut a video into multiple parts just using the audio of the video according to the user's script. They might be saying things a little off script sometimes, so at appropriate times you might also use that in order for the whole video to make sense according to the script. The goal is to, at the end, have a list of cuts for the original video that, when combined, are the person reading the script.

	Provide the result in the following format (this is an example):
	00:01:00-00:02:00,00:02:00-00:03:00

	You can also use more accurate timestamps (with a max of two numbers behind the dot):
	00:00:13.20-00:00:15.30

	The format means the following:
	HH:MM:SS.xx

	Explanation for the format:
	- xx is fractional seconds, up to two decimals.
	- SS, MM, and HH CAN NOT BE BIGGER THAN 60. ALWAYS PROVIDE HH AS WELL.

	Separate the timestamps with a comma, as in the examples above. DO NOT PUT IT IN A CODE BLOCK. NO MARKDOWN. NO PUTTING THEM IN SEPARATE LINES.

	Don't cut out little breaks when they aren't huge: You shouldn't cut out less than a second of a break between different clips. Cut clips together seamlessly, do not make exact cuts, always leave 2 seconds before and after your cut there even when there is no speech, but do not merge. Do not edit in things with the same meaning, strictly do it by looking at the script. DO NOT cut off sentences.
	`

	// Generate the actual response
	parts := []*genai.Part{
		genai.NewPartFromText(userMessage),
		genai.NewPartFromURI(file.URI, file.MIMEType),
	}
	contents := []*genai.Content{
		genai.NewContentFromParts(parts, "user"),
	}

	countTokensResponse, err := client.Models.CountTokens(
		ctx,
		selectedModel,
		contents,
		nil,
	)
	if err != nil {
		log.Fatal(err)
	}

	var proceed bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf(
					"Input tokens=%d. Continue?",
					countTokensResponse.TotalTokens,
				)).
				Value(&proceed),
		),
	)
	if err := confirmForm.Run(); err != nil {
		log.Fatal(err)
	}
	if !proceed {
		fmt.Println("Canceled by user.")
		return
	}

	// Actually prompt gemini to cut the video
	response, err := client.Models.GenerateContent(
		ctx,
		selectedModel,
		contents,
		&genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(systemPrompt, genai.RoleUser),
		},
	)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Cutting using:", response.Text())

	cuts := parseStamps(response.Text())
	applyCuts(cuts, videoFile)
}
